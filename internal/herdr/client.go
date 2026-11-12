// Package herdr is a client of herdr-core, the terminal server behind Gwen
// Terminal (plan.md 3.14.5). It speaks JSON-RPC 2.0 on the core's Unix socket
// in framed mode, which carries calls, the event stream, and attach streams
// side by side on one connection (herdr-core schema/README.md).
package herdr

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Generation is the protocol generation this client speaks.
const Generation = 1

// Frame kinds of framed mode.
const (
	kindData   = 0
	kindCredit = 1
	kindClose  = 2
	kindResize = 3
)

// maxFrame is the largest payload the core sends.
const maxFrame = 16 << 20

// callTimeout bounds a call whose context has no deadline.
const callTimeout = 10 * time.Second

// ErrClosed is a call on a connection that is gone.
var ErrClosed = errors.New("herdr-core: connection closed")

// Error is a JSON-RPC error from the core. Kind is stable: not_found,
// conflict, unavailable, usage, blocked, ...
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Kind string `json:"kind"`
		Hint string `json:"hint"`
	} `json:"data"`
}

func (e *Error) Error() string { return "herdr-core: " + e.Message }

// IsKind reports whether err is a core error of the given kind.
func IsKind(err error, kind string) bool {
	var e *Error
	return errors.As(err, &e) && e.Data.Kind == kind
}

// Session is the name of Gwen's own herdr-core (HC_SESSION). A named session
// is an isolated core with its own socket, terminals, and documents, so
// Gwen never shares a core with a Herdr the person runs for themselves.
const Session = "gwen"

// DefaultSocket is the socket of Gwen's own core:
// $XDG_RUNTIME_DIR/herdr-core/gwen.sock.
func DefaultSocket() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(os.TempDir(), "herdr-core-"+strconv.Itoa(os.Getuid()))
		return filepath.Join(dir, Session+".sock")
	}
	return filepath.Join(dir, "herdr-core", Session+".sock")
}

// Hello is what the core says about itself when a client connects.
type Hello struct {
	Generation int      `json:"generation"`
	Version    string   `json:"version"`
	Features   []string `json:"features"`
	Methods    []string `json:"methods"`
}

// Client is one connection to the core. It is safe for concurrent use.
type Client struct {
	conn  net.Conn
	hello Hello

	wmu sync.Mutex // serializes frames written to conn

	mu      sync.Mutex
	nextID  uint64
	replies map[uint64]chan reply
	streams map[uint32]*Stream
	early   map[uint32][]Frame // frames that arrived before their stream registered
	closed  bool
	done    chan struct{}
}

type reply struct {
	result json.RawMessage
	err    error
}

// Dial connects to the core at socket, says hello, and switches the
// connection to framed mode.
func Dial(ctx context.Context, socket string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", socket)
	if err != nil {
		return nil, fmt.Errorf("herdr-core: connect %s: %w", socket, err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(callTimeout)
	}
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write([]byte(`{"jsonrpc":"2.0","id":0,"method":"hello","params":{"framed":true,"features":["resize_notifications"]}}` + "\n")); err != nil {
		conn.Close()
		return nil, fmt.Errorf("herdr-core: hello: %w", err)
	}
	// The reply is one line; everything after it is frames, so read it byte
	// by byte and leave the rest in the socket.
	line, err := readLine(conn, 1<<20)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("herdr-core: hello: %w", err)
	}
	var msg struct {
		Result Hello  `json:"result"`
		Error  *Error `json:"error"`
	}
	if err := json.Unmarshal(line, &msg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("herdr-core: hello: %w", err)
	}
	if msg.Error != nil {
		conn.Close()
		return nil, msg.Error
	}
	_ = conn.SetDeadline(time.Time{})
	c := &Client{conn: conn, hello: msg.Result, replies: map[uint64]chan reply{}, streams: map[uint32]*Stream{},
		early: map[uint32][]Frame{}, done: make(chan struct{})}
	go c.read()
	return c, nil
}

func readLine(r io.Reader, limit int) ([]byte, error) {
	var line []byte
	b := make([]byte, 1)
	for len(line) < limit {
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, err
		}
		if b[0] == '\n' {
			return line, nil
		}
		line = append(line, b[0])
	}
	return nil, errors.New("reply too long")
}

// Hello is the core's hello reply.
func (c *Client) Hello() Hello { return c.hello }

// Done is closed when the connection is gone.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close drops the connection; every stream ends and waiting calls fail.
func (c *Client) Close() { c.conn.Close() }

func (c *Client) read() {
	r := bufio.NewReaderSize(c.conn, 64<<10)
	var header [9]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			break
		}
		channel := binary.LittleEndian.Uint32(header[0:4])
		kind := header[4]
		n := binary.LittleEndian.Uint32(header[5:9])
		if n > maxFrame {
			break
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			break
		}
		if channel == 0 {
			c.reply(payload)
			continue
		}
		var f Frame
		switch kind {
		case kindData:
			f = Frame{Data: payload}
		case kindResize:
			if len(payload) < 4 {
				continue
			}
			f = Frame{Resize: &Size{Cols: int(binary.LittleEndian.Uint16(payload[0:2])), Rows: int(binary.LittleEndian.Uint16(payload[2:4]))}}
		case kindClose:
			reason := CloseReason{Reason: "closed"}
			_ = json.Unmarshal(payload, &reason)
			f = Frame{Closed: &reason}
		default:
			continue
		}
		c.mu.Lock()
		s := c.streams[channel]
		if s == nil {
			c.early[channel] = append(c.early[channel], f)
		} else if f.Closed != nil {
			delete(c.streams, channel)
		}
		c.mu.Unlock()
		if s != nil {
			s.push(f)
		}
	}
	c.conn.Close()
	c.mu.Lock()
	c.closed = true
	replies, streams := c.replies, c.streams
	c.replies, c.streams = map[uint64]chan reply{}, map[uint32]*Stream{}
	c.mu.Unlock()
	for _, ch := range replies {
		ch <- reply{err: ErrClosed}
	}
	for _, s := range streams {
		s.push(Frame{Closed: &CloseReason{Reason: "lost"}})
	}
	close(c.done)
}

func (c *Client) reply(payload []byte) {
	var msg struct {
		ID     *uint64         `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *Error          `json:"error"`
	}
	if json.Unmarshal(payload, &msg) != nil || msg.ID == nil {
		return
	}
	c.mu.Lock()
	ch := c.replies[*msg.ID]
	delete(c.replies, *msg.ID)
	c.mu.Unlock()
	if ch == nil {
		return
	}
	if msg.Error != nil {
		ch <- reply{err: msg.Error}
		return
	}
	ch <- reply{result: msg.Result}
}

func (c *Client) write(channel uint32, kind byte, payload []byte) error {
	frame := make([]byte, 9+len(payload))
	binary.LittleEndian.PutUint32(frame[0:4], channel)
	frame[4] = kind
	binary.LittleEndian.PutUint32(frame[5:9], uint32(len(payload)))
	copy(frame[9:], payload)
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(callTimeout))
	_, err := c.conn.Write(frame)
	return err
}

// Call runs one method. params is marshalled as the params object; result,
// when not nil, receives the result. A context without a deadline waits 10
// seconds.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	if params == nil {
		params = struct{}{}
	}
	p, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("herdr-core: %s: %w", method, err)
	}
	ch := make(chan reply, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClosed
	}
	c.nextID++
	id := c.nextID
	c.replies[id] = ch
	c.mu.Unlock()
	req := fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":%s}`, id, method, p)
	if err := c.write(0, kindData, req); err != nil {
		c.forget(id)
		return ErrClosed
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, callTimeout)
		defer cancel()
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if result == nil {
			return nil
		}
		if err := json.Unmarshal(r.result, result); err != nil {
			return fmt.Errorf("herdr-core: %s: bad reply: %w", method, err)
		}
		return nil
	case <-ctx.Done():
		c.forget(id)
		return fmt.Errorf("herdr-core: %s: %w", method, ctx.Err())
	}
}

func (c *Client) forget(id uint64) {
	c.mu.Lock()
	delete(c.replies, id)
	c.mu.Unlock()
}

// open registers the stream of a channel the core just named, with any
// frames that arrived before the reply was read.
func (c *Client) open(channel uint32) *Stream {
	s := &Stream{c: c, channel: channel, wake: make(chan struct{}, 1)}
	c.mu.Lock()
	early := c.early[channel]
	delete(c.early, channel)
	if c.closed {
		early = append(early, Frame{Closed: &CloseReason{Reason: "lost"}})
	} else {
		c.streams[channel] = s
		for _, f := range early {
			if f.Closed != nil {
				delete(c.streams, channel)
			}
		}
	}
	c.mu.Unlock()
	for _, f := range early {
		s.push(f)
	}
	return s
}

// Size is a terminal grid.
type Size struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// CloseReason is why a stream ended: exited, killed, server_stop, lost (the
// connection went away), closed (this side detached), ...
type CloseReason struct {
	Reason   string `json:"reason"`
	ExitCode *int   `json:"exit_code"`
	Signal   *int   `json:"signal"`
}

// Frame is one item of a stream: exactly one field is set.
type Frame struct {
	Data   []byte
	Resize *Size
	Closed *CloseReason
}

// Stream is the receiving end of an attach or event channel. The core stops
// sending when its credit runs out, so the queue stays small as long as the
// reader returns credit only for what it has passed on.
type Stream struct {
	c       *Client
	channel uint32

	mu     sync.Mutex
	queue  []Frame
	closed bool
	wake   chan struct{}
}

func (s *Stream) push(f Frame) {
	s.mu.Lock()
	if !s.closed {
		s.queue = append(s.queue, f)
		s.closed = f.Closed != nil
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Next returns the next frame. After a Closed frame it returns io.EOF.
func (s *Stream) Next(ctx context.Context) (Frame, error) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			f := s.queue[0]
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return f, nil
		}
		closed := s.closed
		s.mu.Unlock()
		if closed {
			return Frame{}, io.EOF
		}
		select {
		case <-s.wake:
		case <-ctx.Done():
			return Frame{}, ctx.Err()
		}
	}
}

// Credit tells the core that n more bytes of this stream were passed on.
func (s *Stream) Credit(n int) {
	if n <= 0 {
		return
	}
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(n))
	_ = s.c.write(s.channel, kindCredit, b[:])
}

// Write sends bytes as typed input; only a viewer that attached without
// observe may type.
func (s *Stream) Write(p []byte) error { return s.c.write(s.channel, kindData, p) }

// Resize announces this viewer's grid; it makes the viewer the active one.
func (s *Stream) Resize(cols, rows int) error {
	var b [4]byte
	binary.LittleEndian.PutUint16(b[0:2], uint16(cols))
	binary.LittleEndian.PutUint16(b[2:4], uint16(rows))
	return s.c.write(s.channel, kindResize, b[:])
}

// Close detaches the stream.
func (s *Stream) Close() {
	s.c.mu.Lock()
	delete(s.c.streams, s.channel)
	s.c.mu.Unlock()
	_ = s.c.write(s.channel, kindClose, nil)
	s.push(Frame{Closed: &CloseReason{Reason: "closed"}})
}

// VersionAtLeast reports whether version have is at least want; both are
// dotted numbers, with or without a leading "v". Anything unparseable counts
// as 0.
func VersionAtLeast(have, want string) bool {
	h, w := versionParts(have), versionParts(want)
	for i := range 3 {
		if h[i] != w[i] {
			return h[i] > w[i]
		}
	}
	return true
}

func versionParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	for i, part := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(part)
	}
	return out
}
