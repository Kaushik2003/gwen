package herdrtest

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kzark/gwen/internal/herdr"
)

// Paint is what the fake sends as a terminal's paint, first on attach and
// again when a viewer falls too far behind or its grid changes: CAN (which
// cancels a half-sent escape sequence, as herdr-core's paints start), then
// the screen set with SetScreen.
func Paint(screen string) []byte { return append([]byte{0x18}, screen...) }

type resizeAt struct {
	at   int // pending bytes to send before it
	size [4]byte
}

func sizeBytes(cols, rows int) [4]byte {
	var b [4]byte
	binary.LittleEndian.PutUint16(b[0:2], uint16(cols))
	binary.LittleEndian.PutUint16(b[2:4], uint16(rows))
	return b
}

// Stream is the core's end of one attach view, or of one framed event
// subscription (Terminal is "" then). Its methods are safe for concurrent
// use; the Wait methods must run on the test's goroutine.
type Stream struct {
	core     *Core
	cn       *conn
	channel  uint32
	terminal string
	params   herdr.AttachParams
	notify   bool // resize notifications confirmed

	// Outbox state, as in herdr-core's wire.rs.
	pending []byte
	resizes []resizeAt
	paused  bool // until the attach reply is queued
	credit  int
	ending  bool // close with reason once pending is out
	reason  []byte

	open     bool // still in the outbox
	attached bool // still a viewer: until it closes either way
	closed   bool // over, either way
	lost     bool // its connection went away
	detached bool // the client sent CLOSE

	activeAt int64 // when it last attached, typed or resized; 0 for observers
	size     herdr.Size

	input    []byte
	credited int
	sent     int
	resized  []herdr.Size
}

// Terminal is the address the stream views; "" for an event subscription.
func (s *Stream) Terminal() string { return s.terminal }

// Params are the attach params the client sent.
func (s *Stream) Params() herdr.AttachParams { return s.params }

// Notified reports whether the stream confirmed resize notifications.
func (s *Stream) Notified() bool { return s.notify }

// Channel is the stream's channel on its connection.
func (s *Stream) Channel() uint32 { return s.channel }

// Conn is the connection the stream belongs to (see Call.Conn).
func (s *Stream) Conn() int { return s.cn.id }

// Push writes bytes to this viewer alone, within its credit. A viewer that
// would fall more than MaxPending behind gets a fresh Paint of its terminal
// instead of the backlog; an event subscriber gets `missed`.
func (s *Stream) Push(data []byte) {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	s.pushLocked(data)
}

func (s *Stream) pushLocked(data []byte) {
	if !s.open || s.ending {
		return
	}
	c := s.core
	if s.terminal == "" {
		if !s.queueLines(data) {
			s.queueData(c.missedLineLocked())
		}
	} else if !s.queueData(data) {
		s.queueData(Paint(c.screenLocked(s.terminal)))
	}
	s.cn.kick()
}

// Finish closes the stream with r once its queued output is out, as when the
// program ends: its last screen is what the viewer most wants to see.
func (s *Stream) Finish(r herdr.CloseReason) {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	s.finishLocked(reasonPayload(r))
}

func (s *Stream) finishLocked(reason []byte) {
	switch {
	case s.open:
		s.ending, s.reason = true, reason
		s.cn.kick()
	case !s.closed:
		s.endLocked()
		s.cn.queueLocked(encode(s.channel, kindClose, reason))
	}
}

// Close closes the stream at once with r, dropping its queued output. A zero
// r sends an empty CLOSE.
func (s *Stream) Close(r herdr.CloseReason) {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	if s.closed {
		return
	}
	s.endLocked()
	s.cn.queueLocked(encode(s.channel, kindClose, reasonPayload(r)))
}

// endLocked takes the stream out of the outbox for good.
func (s *Stream) endLocked() {
	delete(s.cn.streams, s.channel)
	wasTyping := s.attached && s.terminal != "" && !s.params.Observe
	s.open, s.attached, s.closed, s.pending = false, false, true, nil
	if wasTyping {
		s.core.applySizeLocked(s.terminal)
	}
	s.core.notifyLocked()
}

func reasonPayload(r herdr.CloseReason) []byte {
	if r == (herdr.CloseReason{}) {
		return nil
	}
	b, _ := json.Marshal(r)
	return b
}

// queueData is herdr-core's Outbox::data: false, with the backlog dropped,
// when the viewer is too far behind and needs a repaint instead.
func (s *Stream) queueData(b []byte) bool {
	if !s.open {
		return true
	}
	if len(s.pending)+len(b) > s.core.opts.MaxPending {
		s.pending = nil
		if n := len(s.resizes); n > 0 {
			s.resizes = []resizeAt{{at: 0, size: s.resizes[n-1].size}}
		}
		return false
	}
	s.pending = append(s.pending, b...)
	return true
}

// queueLines is Outbox::data_lines for a stream of whole lines: too far
// behind, it keeps only the rest of a line whose start may be out already.
func (s *Stream) queueLines(b []byte) bool {
	if !s.open {
		return true
	}
	if len(s.pending)+len(b) > s.core.opts.MaxPending {
		rest := len(s.pending)
		if i := bytes.IndexByte(s.pending, '\n'); i >= 0 {
			rest = i + 1
		}
		s.pending = s.pending[:rest]
		return false
	}
	s.pending = append(s.pending, b...)
	return true
}

// queueResize is Outbox::resize: a new grid supersedes pending output; the
// caller queues the repaint.
func (s *Stream) queueResize(cols, rows int) {
	s.pending = nil
	s.resizes = []resizeAt{{at: 0, size: sizeBytes(cols, rows)}}
}

// Input is every byte the client sent as DATA on this stream. herdr-core
// passes on only a typing viewer's (see Core.Input).
func (s *Stream) Input() []byte {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	return slices.Clone(s.input)
}

// WaitInput waits until the client has sent at least n bytes on the stream
// and returns them all.
func (s *Stream) WaitInput(n int) []byte {
	s.core.t.Helper()
	var out []byte
	s.core.waitUntil(fmt.Sprintf("%d bytes typed on channel %d", n, s.channel), func() bool {
		out = slices.Clone(s.input)
		return len(out) >= n
	})
	return out
}

// Credited is the credit the client has returned in total.
func (s *Stream) Credited() int {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	return s.credited
}

// WaitCredited waits until the client has returned at least n bytes of
// credit in total.
func (s *Stream) WaitCredited(n int) {
	s.core.t.Helper()
	s.core.waitUntil(fmt.Sprintf("%d bytes of credit on channel %d", n, s.channel), func() bool { return s.credited >= n })
}

// Sent is how many DATA bytes the core has put on the wire for the stream.
func (s *Stream) Sent() int {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	return s.sent
}

// WaitSent waits until the core has sent at least n DATA bytes.
func (s *Stream) WaitSent(n int) {
	s.core.t.Helper()
	s.core.waitUntil(fmt.Sprintf("%d bytes sent on channel %d", n, s.channel), func() bool { return s.sent >= n })
}

// Queued is how many bytes wait for credit.
func (s *Stream) Queued() int {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	return len(s.pending)
}

// Resizes are the RESIZE frames the client sent, observers' included
// (herdr-core ignores those).
func (s *Stream) Resizes() []herdr.Size {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	return slices.Clone(s.resized)
}

// WaitResizes waits until the client has sent n RESIZE frames and returns
// them all.
func (s *Stream) WaitResizes(n int) []herdr.Size {
	s.core.t.Helper()
	var out []herdr.Size
	s.core.waitUntil(fmt.Sprintf("%d resizes on channel %d", n, s.channel), func() bool {
		out = slices.Clone(s.resized)
		return len(out) >= n
	})
	return out
}

// Detached reports whether the client sent CLOSE.
func (s *Stream) Detached() bool {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	return s.detached
}

// WaitDetached waits until the client sends CLOSE.
func (s *Stream) WaitDetached() {
	s.core.t.Helper()
	s.core.waitUntil(fmt.Sprintf("the client to close channel %d", s.channel), func() bool { return s.detached })
}

// Closed reports whether the stream is over: the core sent CLOSE, the client
// did, or the connection went away.
func (s *Stream) Closed() bool {
	s.core.mu.Lock()
	defer s.core.mu.Unlock()
	return s.closed
}

// WaitClosed waits until the stream is over.
func (s *Stream) WaitClosed() {
	s.core.t.Helper()
	s.core.waitUntil(fmt.Sprintf("channel %d to close", s.channel), func() bool { return s.closed })
}

// viewerInput is DATA from the client: typing from a typing viewer makes it
// the active one and reaches the program; an observer's is dropped.
func (c *Core) viewerInput(cn *conn, channel uint32, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := cn.all[channel]
	if s == nil || !s.attached {
		return
	}
	s.input = append(s.input, data...)
	if t := c.terms[s.terminal]; t != nil && !s.params.Observe {
		t.input = append(t.input, data...)
		c.activity++
		s.activeAt = c.activity
		c.applySizeLocked(s.terminal)
	}
	c.notifyLocked()
}

func (c *Core) viewerCredit(cn *conn, channel uint32, n int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := cn.all[channel]
	if s == nil {
		return
	}
	s.credited += n
	if s.credit < 1<<40 {
		s.credit += n
	}
	cn.kick()
	c.notifyLocked()
}

// viewerResize is RESIZE from the client: a typing viewer's new grid, which
// it becomes the active viewer with. An observer's changes nothing.
func (c *Core) viewerResize(cn *conn, channel uint32, cols, rows int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := cn.all[channel]
	if s == nil {
		return
	}
	s.resized = append(s.resized, herdr.Size{Cols: cols, Rows: rows})
	if s.attached && s.terminal != "" && !s.params.Observe {
		s.size = herdr.Size{Cols: cols, Rows: rows}
		c.activity++
		s.activeAt = c.activity
		c.applySizeLocked(s.terminal)
	}
	c.notifyLocked()
}

// viewerClose is CLOSE from the client: the viewer detaches, its queued
// output is dropped, and the core answers with an empty CLOSE.
func (c *Core) viewerClose(cn *conn, channel uint32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if s := cn.all[channel]; s != nil {
		s.detached = true
		if !s.closed {
			s.endLocked()
		}
	}
	cn.queueLocked(encode(channel, kindClose, nil))
	c.notifyLocked()
}
