package herdrtest

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"slices"
	"sync"

	"github.com/kzark/gwen/internal/herdr"
)

// Frame kinds of framed mode (herdr-core schema/README.md).
const (
	kindData   = 0
	kindCredit = 1
	kindClose  = 2
	kindResize = 3
)

// maxFrame is the largest payload either side may send.
const maxFrame = 16 << 20

// conn is one client connection. Everything but nc, wake and done is
// guarded by core.mu.
type conn struct {
	core *Core
	id   int
	nc   net.Conn
	wake chan struct{} // tells the writer there is something to send
	done chan struct{} // closed once the core has let go of the connection

	goneOnce sync.Once
	dead     bool // the core let go of it: nothing more opens on it
	framed   bool
	// plainSub is a plain connection after events.subscribe: it carries only
	// notifications now. Until its reply is queued they wait in held.
	plainSub  bool
	plainHold bool
	held      [][]byte

	// The outbox, as in herdr-core's wire.rs: control frames (replies and
	// CLOSE answers) go first, then streams in turn, each within its credit.
	control [][]byte
	streams map[uint32]*Stream // streams still sending, by channel
	all     map[uint32]*Stream // every stream this connection opened
	turn    uint32
	nextCh  uint32
}

func encode(channel uint32, kind byte, payload []byte) []byte {
	f := make([]byte, 9+len(payload))
	binary.LittleEndian.PutUint32(f[0:4], channel)
	f[4] = kind
	binary.LittleEndian.PutUint32(f[5:9], uint32(len(payload)))
	copy(f[9:], payload)
	return f
}

// kick wakes the writer; it never blocks, so it is safe under core.mu.
func (cn *conn) kick() {
	select {
	case cn.wake <- struct{}{}:
	default:
	}
}

func (cn *conn) newChannelLocked() uint32 {
	cn.nextCh++
	return cn.nextCh
}

func (cn *conn) writeLoop() {
	defer cn.core.wg.Done()
	for {
		select {
		case <-cn.wake:
		case <-cn.done:
			return
		}
		for {
			cn.core.mu.Lock()
			f := cn.nextFrameLocked()
			if f != nil {
				cn.core.notifyLocked()
			}
			cn.core.mu.Unlock()
			if f == nil {
				break
			}
			if _, err := cn.nc.Write(f); err != nil {
				cn.nc.Close()
				return
			}
		}
	}
}

// nextFrameLocked is herdr-core's Outbox::next_frame: what to write next,
// or nil when nothing may go now.
func (cn *conn) nextFrameLocked() []byte {
	if len(cn.control) > 0 {
		f := cn.control[0]
		cn.control = cn.control[1:]
		return f
	}
	channels := slices.Sorted(func(yield func(uint32) bool) {
		for ch := range cn.streams {
			if !yield(ch) {
				return
			}
		}
	})
	// A stream whose program ended closes once its output is out.
	for _, ch := range channels {
		s := cn.streams[ch]
		if !s.paused && s.ending && len(s.pending) == 0 {
			s.endLocked()
			return encode(ch, kindClose, s.reason)
		}
	}
	var ready []uint32
	for _, ch := range channels {
		if s := cn.streams[ch]; !s.paused && s.credit > 0 && len(s.pending) > 0 {
			ready = append(ready, ch)
		}
	}
	if len(ready) == 0 {
		return nil
	}
	ch := ready[0]
	for _, r := range ready {
		if r > cn.turn {
			ch = r
			break
		}
	}
	cn.turn = ch
	s := cn.streams[ch]
	if len(s.resizes) > 0 && s.resizes[0].at == 0 {
		size := s.resizes[0].size
		s.resizes = s.resizes[1:]
		return encode(ch, kindResize, size[:])
	}
	n := min(len(s.pending), s.credit, chunk)
	if len(s.resizes) > 0 {
		n = min(n, s.resizes[0].at)
	}
	for i := range s.resizes {
		s.resizes[i].at -= n
	}
	s.credit -= n
	s.sent += n
	f := encode(ch, kindData, s.pending[:n])
	s.pending = s.pending[n:]
	return f
}

// queueLocked puts one control item in the outbox: a frame in framed mode,
// a line in plain mode.
func (cn *conn) queueLocked(item []byte) {
	cn.control = append(cn.control, item)
	cn.kick()
}

// queuePlainLocked sends a notification line to a plain subscriber.
func (cn *conn) queuePlainLocked(line []byte) {
	if cn.plainHold {
		cn.held = append(cn.held, line)
		return
	}
	cn.queueLocked(line)
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// parse reads one JSON-RPC request, or queues the parse error.
func (cn *conn) parse(payload []byte) (Call, bool) {
	var req request
	if err := json.Unmarshal(payload, &req); err != nil || req.Method == "" {
		cn.reply(Call{id: json.RawMessage("null")}, nil, protoErr(-32700, "parse_error", "not a JSON-RPC request"))
		return Call{}, false
	}
	call := Call{Conn: cn.id, Method: req.Method, Params: req.Params, id: req.ID, core: cn.core, cn: cn}
	c := cn.core
	c.mu.Lock()
	c.calls = append(c.calls, call)
	c.notifyLocked()
	c.mu.Unlock()
	return call, true
}

// protoErr is a JSON-RPC protocol error with its own code.
func protoErr(code int, kind, message string) *herdr.Error {
	e := Err(kind, message)
	e.Code = code
	return e
}

func errorJSON(err error) map[string]any {
	var he *herdr.Error
	if errors.As(err, &he) {
		code := he.Code
		if code == 0 {
			code = -32000
		}
		return map[string]any{"code": code, "message": he.Message, "data": map[string]any{"kind": he.Data.Kind, "hint": he.Data.Hint}}
	}
	return map[string]any{"code": -32603, "message": err.Error(), "data": map[string]any{"kind": "internal", "hint": ""}}
}

// reply queues the answer to a call. In framed mode a result that names a
// channel lets that stream start, so its frames follow the reply.
func (cn *conn) reply(call Call, result any, err error) {
	if len(call.id) == 0 {
		return // a notification
	}
	msg := map[string]any{"jsonrpc": "2.0", "id": call.id}
	if err != nil {
		msg["error"] = errorJSON(err)
	} else {
		if result == nil {
			result = struct{}{}
		}
		msg["result"] = result
	}
	b, merr := json.Marshal(msg)
	if merr != nil {
		b, _ = json.Marshal(map[string]any{"jsonrpc": "2.0", "id": call.id, "error": errorJSON(merr)})
	}
	c := cn.core
	c.mu.Lock()
	defer c.mu.Unlock()
	if !cn.framed {
		cn.queueLocked(append(b, '\n'))
		if cn.plainHold {
			cn.plainHold = false
			for _, line := range cn.held {
				cn.queueLocked(line)
			}
			cn.held = nil
		}
		return
	}
	cn.queueLocked(encode(0, kindData, b))
	var r struct {
		Result struct {
			Channel *uint32 `json:"channel"`
		} `json:"result"`
	}
	if json.Unmarshal(b, &r) == nil && r.Result.Channel != nil {
		if s := cn.streams[*r.Result.Channel]; s != nil {
			s.paused = false
		}
	}
}

func (cn *conn) readLoop() {
	defer cn.core.wg.Done()
	defer cn.gone()
	r := bufio.NewReaderSize(cn.nc, 64<<10)
	// Plain mode: one message per line, answered in order.
	for {
		cn.core.mu.Lock()
		framed := cn.framed
		cn.core.mu.Unlock()
		if framed {
			break
		}
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		line = bytes.TrimSpace(line)
		cn.core.mu.Lock()
		sub := cn.plainSub
		cn.core.mu.Unlock()
		if len(line) == 0 || sub {
			continue
		}
		call, ok := cn.parse(line)
		if !ok {
			continue
		}
		// The handler may block; a dropped connection stops waiting for it.
		type answer struct {
			result any
			err    error
		}
		answered := make(chan answer, 1)
		go func() {
			result, err := cn.core.run(call)
			answered <- answer{result, err}
		}()
		var a answer
		select {
		case a = <-answered:
		case <-cn.done:
			return
		}
		cn.reply(call, a.result, a.err)
		if call.Method == "hello" && a.err == nil {
			var p struct {
				Framed bool `json:"framed"`
			}
			if call.Decode(&p) == nil && p.Framed {
				cn.core.mu.Lock()
				cn.framed = true
				cn.core.mu.Unlock()
			}
		}
	}
	// Framed mode: calls on channel 0 run side by side.
	var header [9]byte
	for {
		if _, err := io.ReadFull(r, header[:]); err != nil {
			return
		}
		channel := binary.LittleEndian.Uint32(header[0:4])
		kind := header[4]
		n := binary.LittleEndian.Uint32(header[5:9])
		if n > maxFrame {
			return
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			return
		}
		switch {
		case channel == 0 && kind == kindData:
			call, ok := cn.parse(payload)
			if !ok {
				continue
			}
			go func() {
				result, err := cn.core.run(call)
				cn.reply(call, result, err)
			}()
		case channel == 0:
		case kind == kindData:
			cn.core.viewerInput(cn, channel, payload)
		case kind == kindCredit && n == 4:
			cn.core.viewerCredit(cn, channel, int(binary.LittleEndian.Uint32(payload)))
		case kind == kindResize && n == 4:
			cn.core.viewerResize(cn, channel, int(binary.LittleEndian.Uint16(payload[0:2])), int(binary.LittleEndian.Uint16(payload[2:4])))
		case kind == kindClose:
			cn.core.viewerClose(cn, channel)
		}
	}
}

// gone lets go of a connection that ended: its streams and subscriptions
// end with it. It runs once, from the reader or from DropConnections.
func (cn *conn) gone() { cn.goneOnce.Do(cn.let) }

func (cn *conn) let() {
	cn.nc.Close()
	c := cn.core
	c.mu.Lock()
	cn.dead = true
	delete(c.conns, cn)
	var touched []string
	for _, s := range cn.all {
		if s.attached && s.terminal != "" && !s.params.Observe {
			touched = append(touched, s.terminal)
		}
		s.attached, s.open, s.closed, s.lost = false, false, true, true
		s.pending = nil
	}
	cn.streams = map[uint32]*Stream{}
	cn.control, cn.held = nil, nil
	for _, addr := range touched {
		c.applySizeLocked(addr)
	}
	c.notifyLocked()
	c.mu.Unlock()
	close(cn.done)
}

// run answers a call with its handler or the built-in.
func (c *Core) run(call Call) (any, error) {
	c.mu.Lock()
	h := c.handlers[call.Method]
	c.mu.Unlock()
	if h != nil {
		return h(call)
	}
	return call.Builtin()
}
