package herdrtest

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/kzark/gwen/internal/herdr"
)

// terminal is one fake terminal.
type terminal struct {
	rec    herdr.Terminal
	screen string
	input  []byte            // what typing viewers sent
	files  map[string][]byte // what terminal.put saved, by name
}

type document struct {
	value   json.RawMessage // nil once deleted
	version uint64
}

// clone deep-copies a record, so callers and JSON encoding outside the lock
// never share its maps and pointers.
func clone(t herdr.Terminal) herdr.Terminal {
	b, _ := json.Marshal(t)
	var out herdr.Terminal
	_ = json.Unmarshal(b, &out)
	return out
}

// AddTerminal adds a terminal and sends `created`. Empty fields get
// defaults: the next free address ("t1", "t2", ...), state live, a pid for
// a live terminal, an 80x24 grid, and empty labels. It returns the record.
func (c *Core) AddTerminal(t herdr.Terminal) herdr.Terminal {
	c.mu.Lock()
	defer c.mu.Unlock()
	return clone(c.addLocked(clone(t)).rec)
}

func (c *Core) addLocked(t herdr.Terminal) *terminal {
	c.nextTerm++
	if t.Address == "" {
		for c.terms["t"+strconv.Itoa(c.nextTerm)] != nil {
			c.nextTerm++
		}
		t.Address = "t" + strconv.Itoa(c.nextTerm)
	}
	if t.State == "" {
		t.State = "live"
	}
	if t.State == "live" && t.PID == 0 {
		t.PID = 10000 + c.nextTerm
	}
	if t.Cols == 0 || t.Rows == 0 {
		t.Cols, t.Rows = 80, 24
	}
	if t.Labels == nil {
		t.Labels = map[string]string{}
	}
	if t.CreatedMS == 0 {
		t.CreatedMS = int64(c.nextTerm) * 1000
	}
	tm := &terminal{rec: t, files: map[string][]byte{}}
	if _, ok := c.terms[t.Address]; !ok {
		c.order = append(c.order, t.Address)
	}
	c.terms[t.Address] = tm
	c.eventLocked("created", t.Address, t)
	return tm
}

// Terminal returns a copy of a terminal's record.
func (c *Core) Terminal(addr string) (herdr.Terminal, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.terms[addr]
	if t == nil {
		return herdr.Terminal{}, false
	}
	return clone(t.rec), true
}

// Terminals returns copies of every record, finished ones included, in the
// order they were added.
func (c *Core) Terminals() []herdr.Terminal {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshotLocked()
}

func (c *Core) snapshotLocked() []herdr.Terminal {
	out := []herdr.Terminal{}
	for _, addr := range c.order {
		if t := c.terms[addr]; t != nil {
			out = append(out, clone(t.rec))
		}
	}
	return out
}

// findLocked resolves an address or a name.
func (c *Core) findLocked(ref string) *terminal {
	if t := c.terms[ref]; t != nil {
		return t
	}
	for _, addr := range c.order {
		if t := c.terms[addr]; t != nil && t.rec.Name != "" && t.rec.Name == ref {
			return t
		}
	}
	return nil
}

// mustLocked finds a terminal for a test helper, or reports the mistake.
func (c *Core) mustLocked(addr string) *terminal {
	t := c.terms[addr]
	if t == nil {
		c.t.Errorf("herdrtest: no terminal %s", addr)
	}
	return t
}

// Update changes a terminal's record with change (which may be nil; it runs
// with the core locked, so it must not call the core) and, when typ is not
// "", sends an event of that type with data. A nil data is the whole record
// for created and restarted, else {}. Use it for status records, keep,
// restarted and the like.
func (c *Core) Update(addr, typ string, data any, change func(t *herdr.Terminal)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.mustLocked(addr)
	if t == nil {
		return
	}
	if change != nil {
		change(&t.rec)
	}
	if typ == "" {
		return
	}
	if data == nil {
		data = map[string]any{}
		if typ == "created" || typ == "restarted" {
			data = t.rec
		}
	}
	c.eventLocked(typ, addr, data)
}

// SetStatus replaces a terminal's root status (nil clears it) and sends
// `status` with the status, its records, and whether it ever reported.
func (c *Core) SetStatus(addr string, st *herdr.Status) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.mustLocked(addr)
	if t == nil {
		return
	}
	if st != nil {
		cp := *st
		st = &cp
		t.rec.Reported = true
	}
	t.rec.Status = st
	c.statusEventLocked(t)
}

func (c *Core) statusEventLocked(t *terminal) {
	c.eventLocked("status", t.rec.Address, map[string]any{"status": t.rec.Status, "records": t.rec.Records, "reported": t.rec.Reported})
}

// SetForeground sets what runs in a terminal now (nil: nothing) and sends
// `foreground`.
func (c *Core) SetForeground(addr string, fg *herdr.Foreground) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.mustLocked(addr)
	if t == nil {
		return
	}
	if fg != nil {
		cp := *fg
		cp.Argv = slices.Clone(fg.Argv)
		fg = &cp
	}
	t.rec.Foreground = fg
	c.eventLocked("foreground", addr, fg)
}

// SetLabels merges labels into a terminal's, an empty value removing one,
// and sends `labels` with all of them, as terminal.label does.
func (c *Core) SetLabels(addr string, labels map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.mustLocked(addr); t != nil {
		c.labelLocked(t, labels)
	}
}

func (c *Core) labelLocked(t *terminal, labels map[string]string) {
	merged := maps.Clone(t.rec.Labels)
	if merged == nil {
		merged = map[string]string{}
	}
	for k, v := range labels {
		if v == "" {
			delete(merged, k)
		} else {
			merged[k] = v
		}
	}
	t.rec.Labels = merged
	c.eventLocked("labels", t.rec.Address, map[string]any{"labels": merged})
}

// SetTitle sets a terminal's title and sends `title`.
func (c *Core) SetTitle(addr, title string) {
	c.Update(addr, "title", map[string]any{"title": title}, func(t *herdr.Terminal) { t.Title = title })
}

// SetTurn sets a terminal's turn counter and sends `turn` with the status
// word the turn ended on.
func (c *Core) SetTurn(addr string, turn int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.mustLocked(addr)
	if t == nil {
		return
	}
	t.rec.Turn = turn
	var state any
	if t.rec.Status != nil {
		state = t.rec.Status.State
	}
	c.eventLocked("turn", addr, map[string]any{"turn": turn, "state": state})
}

// SetScreen sets a terminal's screen: what terminal.read returns and what a
// Paint shows. It sends nothing.
func (c *Core) SetScreen(addr, screen string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.mustLocked(addr); t != nil {
		t.screen = screen
	}
}

func (c *Core) screenLocked(addr string) string {
	if t := c.terms[addr]; t != nil {
		return t.screen
	}
	return ""
}

// Input is what typing viewers typed into a terminal, in order. Observers'
// bytes never reach it.
func (c *Core) Input(addr string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.terms[addr]; t != nil {
		return slices.Clone(t.input)
	}
	return nil
}

// File returns what terminal.put saved beside a terminal under name.
func (c *Core) File(addr, name string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.terms[addr]; t != nil {
		b, ok := t.files[name]
		return slices.Clone(b), ok
	}
	return nil, false
}

// Output is program output in a terminal: it goes to every viewer within
// its credit, and one too far behind gets a Paint instead of the backlog.
func (c *Core) Output(addr string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range c.streamsLocked(addr) {
		s.pushLocked(data)
	}
}

// Resize changes a terminal's grid, as when the viewer whose size counts
// changes it: `size`, and a RESIZE and a fresh Paint for each observer that
// asked for resize notifications, superseding its pending output.
func (c *Core) Resize(addr string, cols, rows int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t := c.mustLocked(addr); t != nil {
		c.resizeLocked(t, cols, rows)
	}
}

func (c *Core) resizeLocked(t *terminal, cols, rows int) {
	if (t.rec.Cols == cols && t.rec.Rows == rows) || cols == 0 || rows == 0 || t.rec.State == "finished" {
		return
	}
	t.rec.Cols, t.rec.Rows = cols, rows
	for _, s := range c.streamsLocked(t.rec.Address) {
		if s.open && s.notify && !s.ending {
			s.queueResize(cols, rows)
			s.queueData(Paint(t.screen))
			s.cn.kick()
		}
	}
	c.eventLocked("size", t.rec.Address, map[string]any{"cols": cols, "rows": rows})
}

// applySizeLocked gives a terminal the grid of its last active typing
// viewer. With none it keeps its grid (herdr-core would run it headless).
func (c *Core) applySizeLocked(addr string) {
	t := c.terms[addr]
	if t == nil {
		return
	}
	var leader *Stream
	for _, s := range c.streamsLocked(addr) {
		if s.attached && !s.params.Observe && (leader == nil || s.activeAt > leader.activeAt) {
			leader = s
		}
	}
	if leader != nil {
		c.resizeLocked(t, leader.size.Cols, leader.size.Rows)
	}
}

// Finish ends a terminal's program: a status event clearing its status if it
// had one, then `finished`; it is marked finished, and each stream on it
// closes with r once its queued output is out.
func (c *Core) Finish(addr string, r herdr.CloseReason) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.mustLocked(addr)
	if t == nil {
		return
	}
	if t.rec.Status != nil {
		t.rec.Status = nil
		c.statusEventLocked(t)
	}
	t.rec.State, t.rec.PID, t.rec.Foreground, t.rec.Reason = "finished", 0, nil, r.Reason
	c.eventLocked("finished", addr, map[string]any{"reason": r.Reason, "exit_code": r.ExitCode, "signal": r.Signal,
		"finished_ms": t.rec.CreatedMS + 1})
	c.closeStreamsLocked(addr, r)
}

// Lose is a kept terminal losing its program: `lost`; it stays live without
// a pid or foreground until a new shell starts, and each stream on it closes
// with r once its queued output is out.
func (c *Core) Lose(addr string, r herdr.CloseReason) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.mustLocked(addr)
	if t == nil {
		return
	}
	t.rec.PID, t.rec.Foreground, t.rec.Reason = 0, nil, r.Reason
	c.eventLocked("lost", addr, map[string]any{"reason": r.Reason, "exit_code": r.ExitCode, "signal": r.Signal, "finished_ms": nil})
	c.closeStreamsLocked(addr, r)
}

func (c *Core) closeStreamsLocked(addr string, r herdr.CloseReason) {
	payload := reasonPayload(r)
	for _, s := range c.streamsLocked(addr) {
		s.finishLocked(payload)
	}
}

// Remove drops a terminal's record and sends `removed`. Its streams are left
// alone: Finish it first to end them.
func (c *Core) Remove(addr string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.mustLocked(addr) == nil {
		return
	}
	c.eventLocked("removed", addr, map[string]any{})
	delete(c.terms, addr)
	c.order = slices.DeleteFunc(c.order, func(a string) bool { return a == addr })
}

// SetDoc writes a document (nil deletes its value but keeps its version)
// and sends its doc event. It returns the new version.
func (c *Core) SetDoc(name string, value any) uint64 {
	var raw json.RawMessage
	if value != nil {
		b, err := json.Marshal(value)
		if err != nil {
			c.t.Errorf("herdrtest: document %s: %v", name, err)
			return 0
		}
		raw = b
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.setDocLocked(name, raw)
}

func (c *Core) setDocLocked(name string, value json.RawMessage) uint64 {
	d := c.docs[name]
	if d == nil {
		d = &document{}
		c.docs[name] = d
	}
	if string(value) == "null" {
		value = nil
	}
	d.value = value
	d.version++
	c.deliverLocked(map[string]any{"type": "doc", "data": map[string]any{"name": name, "version": d.version}})
	return d.version
}

// Doc returns a document's value (nil when unwritten or deleted) and version.
func (c *Core) Doc(name string) (json.RawMessage, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := c.docs[name]; d != nil {
		return slices.Clone(d.value), d.version
	}
	return nil, 0
}

// Seq is the sequence number of the last event.
func (c *Core) Seq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seq
}

// Emit sends an event of any type to every subscriber, with the next
// sequence number, and returns that number. terminal "" leaves the field
// out, as for doc events. The fake's records do not change.
func (c *Core) Emit(typ, terminal string, data any) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	params := map[string]any{"type": typ, "data": data}
	if terminal != "" {
		params["terminal"] = terminal
	}
	return c.deliverLocked(params)
}

// EmitLine sends one raw line, a newline added if missing, to every
// subscriber: a stale event, junk, half a line.
func (c *Core) EmitLine(line string) {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastLocked([]byte(line), false)
}

// Missed sends every subscriber `missed` with a fresh snapshot at the
// current sequence number, as herdr-core does after a machine reconnects.
func (c *Core) Missed() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastLocked(c.missedLineLocked(), false)
}

// Heartbeat sends a heartbeat with the current sequence number to every
// subscriber with nothing queued, as herdr-core does every 15 seconds.
func (c *Core) Heartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.broadcastLocked(notification("heartbeat", map[string]any{"seq": c.seq}), true)
}

func notification(method string, params any) []byte {
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		panic(fmt.Sprintf("herdrtest: %s: %v", method, err))
	}
	return append(b, '\n')
}

func (c *Core) missedLineLocked() []byte {
	return notification("missed", map[string]any{"seq": c.seq, "terminals": c.snapshotLocked()})
}

func (c *Core) eventLocked(typ, addr string, data any) {
	c.deliverLocked(map[string]any{"type": typ, "terminal": addr, "data": data})
}

// deliverLocked numbers an event and queues it for every subscriber; one
// too far behind gets `missed` instead of the backlog.
func (c *Core) deliverLocked(params map[string]any) uint64 {
	c.seq++
	params["seq"] = c.seq
	line := notification("event", params)
	var missed []byte
	for _, s := range c.subs {
		if !s.open {
			continue
		}
		if !s.queueLines(line) {
			if missed == nil {
				missed = c.missedLineLocked()
			}
			s.queueData(missed)
		}
		s.cn.kick()
	}
	for cn := range c.conns {
		if cn.plainSub {
			cn.queuePlainLocked(line)
		}
	}
	c.notifyLocked()
	return c.seq
}

// broadcastLocked queues a line for every subscriber as it is; idle limits
// it to subscribers with nothing queued.
func (c *Core) broadcastLocked(line []byte, idle bool) {
	for _, s := range c.subs {
		if !s.open || (idle && len(s.pending) > 0) {
			continue
		}
		if !s.queueLines(line) {
			s.queueData(line)
		}
		s.cn.kick()
	}
	for cn := range c.conns {
		if cn.plainSub {
			cn.queuePlainLocked(line)
		}
	}
	c.notifyLocked()
}
