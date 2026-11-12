package herdr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
)

// Status is what a program reported about itself through OSC 7501.
type Status struct {
	State string `json:"state"` // idle, working, done, blocked, error, ...
	App   string `json:"app"`
	Kind  string `json:"kind"`
	Msg   string `json:"msg"`
	Title string `json:"title"`
}

// Foreground is the program running in a terminal now.
type Foreground struct {
	PID  int      `json:"pid"`
	Name string   `json:"name"`
	Argv []string `json:"argv"`
	Cwd  string   `json:"cwd"`
}

// Terminal is a core terminal record. Address is "t7" here, or "machine/t7"
// on another machine.
type Terminal struct {
	Address    string            `json:"address"`
	Name       string            `json:"name"`
	Argv       []string          `json:"argv"`
	Cwd        string            `json:"cwd"`
	State      string            `json:"state"` // live or finished
	PID        int               `json:"pid"`
	Cols       int               `json:"cols"`
	Rows       int               `json:"rows"`
	Title      string            `json:"title"`
	Labels     map[string]string `json:"labels"`
	Parent     string            `json:"parent"`
	Keep       bool              `json:"keep"`
	Status     *Status           `json:"status"`
	Records    map[string]Status `json:"records"`
	Reported   bool              `json:"reported"`
	Foreground *Foreground       `json:"foreground"`
	Reason     string            `json:"reason"`
	// Turn counts the turns its program finished.
	Turn      int64 `json:"turn"`
	CreatedMS int64 `json:"created_ms"`
}

// Live reports whether the terminal has a program.
func (t Terminal) Live() bool { return t.State == "live" && t.PID != 0 }

// Local reports whether the terminal runs on this machine.
func (t Terminal) Local() bool { return !bytes.ContainsRune([]byte(t.Address), '/') }

// RunParams starts a program in a new terminal.
type RunParams struct {
	Argv   []string          `json:"argv"`
	Cwd    string            `json:"cwd,omitempty"`
	Env    map[string]string `json:"env,omitempty"`
	Labels map[string]string `json:"labels,omitempty"`
	Name   string            `json:"name,omitempty"`
	Keep   bool              `json:"keep,omitempty"`
	Size   *Size             `json:"size,omitempty"`
	// NoParent keeps the terminal from being listed as a helper of the caller.
	NoParent bool `json:"no_parent,omitempty"`
}

// Run starts a terminal and returns its record.
func (c *Client) Run(ctx context.Context, p RunParams) (Terminal, error) {
	var t Terminal
	return t, c.Call(ctx, "terminal.run", p, &t)
}

// List returns the live terminals.
func (c *Client) List(ctx context.Context) ([]Terminal, error) {
	var r struct {
		Terminals []Terminal `json:"terminals"`
	}
	return r.Terminals, c.Call(ctx, "terminal.list", nil, &r)
}

// Get returns one terminal.
func (c *Client) Get(ctx context.Context, terminal string) (Terminal, error) {
	var t Terminal
	return t, c.Call(ctx, "terminal.get", map[string]any{"terminal": terminal}, &t)
}

// Label sets labels on a terminal; an empty value removes the label.
func (c *Client) Label(ctx context.Context, terminal string, labels map[string]string) error {
	return c.Call(ctx, "terminal.label", map[string]any{"terminal": terminal, "labels": labels}, nil)
}

// SendResult is what the program was doing when text went in.
type SendResult struct {
	Status  string `json:"status"`
	Turn    int64  `json:"turn"`
	Started *bool  `json:"started"`
}

// Send pastes text and presses Enter. The core first waits for anything the
// person is typing in that terminal, so the context should allow about 70
// seconds. force types into a program that reports blocked.
func (c *Client) Send(ctx context.Context, terminal, text string, force bool) (SendResult, error) {
	var r SendResult
	return r, c.Call(ctx, "terminal.send", map[string]any{"terminal": terminal, "text": text, "force": force}, &r)
}

// Keys presses named keys: enter, esc, tab, up, ctrl+c, shift+tab, f5, a.
func (c *Client) Keys(ctx context.Context, terminal string, keys []string) error {
	return c.Call(ctx, "terminal.keys", map[string]any{"terminal": terminal, "keys": keys}, nil)
}

// Read returns the last lines of a terminal's screen as text.
func (c *Client) Read(ctx context.Context, terminal string, lines int) (string, error) {
	var r struct {
		Text string `json:"text"`
	}
	return r.Text, c.Call(ctx, "terminal.read", map[string]any{"terminal": terminal, "lines": lines}, &r)
}

// Interrupt stops what the program is doing: Escape for one that reports
// status, else Ctrl+C. It returns the outcome: stopped, exited,
// still_working, or unknown.
func (c *Client) Interrupt(ctx context.Context, terminal string) (string, error) {
	var r struct {
		Outcome string `json:"outcome"`
	}
	return r.Outcome, c.Call(ctx, "terminal.interrupt", map[string]any{"terminal": terminal, "timeout": 5}, &r)
}

// MaxPut is the largest file Put accepts.
const MaxPut = 8 << 20

// Put saves a file beside a terminal and returns where it landed.
func (c *Client) Put(ctx context.Context, terminal, name string, data []byte) (string, error) {
	var r struct {
		Path string `json:"path"`
	}
	err := c.Call(ctx, "terminal.put", map[string]any{"terminal": terminal, "name": name,
		"data": base64.StdEncoding.EncodeToString(data)}, &r)
	return r.Path, err
}

// Doc reads a core document; value is nil when it was never written.
func (c *Client) Doc(ctx context.Context, name string) (json.RawMessage, error) {
	var r struct {
		Value json.RawMessage `json:"value"`
	}
	if err := c.Call(ctx, "doc.get", map[string]any{"name": name}, &r); err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(r.Value), []byte("null")) {
		return nil, nil
	}
	return r.Value, nil
}

// AttachParams opens a view of a terminal. Observe never types or resizes.
type AttachParams struct {
	Terminal string `json:"terminal"`
	Observe  bool   `json:"observe"`
	// ResizeNotifications asks for the terminal's grid before the first
	// paint and at every change; only observers may ask.
	ResizeNotifications bool `json:"resize_notifications,omitempty"`
	History             int  `json:"history"`
	Cols                int  `json:"cols"`
	Rows                int  `json:"rows"`
}

// Attached is an open view.
type Attached struct {
	*Stream
	Terminal Terminal
	// Resizes says the stream carries Resize frames.
	Resizes bool
}

// Attach opens a view. The stream carries the first paint, then the
// program's output; return credit for what has been passed on.
func (c *Client) Attach(ctx context.Context, p AttachParams) (*Attached, error) {
	var r struct {
		Channel  uint32   `json:"channel"`
		Terminal Terminal `json:"terminal"`
		Resizes  bool     `json:"resize_notifications"`
	}
	if err := c.Call(ctx, "terminal.attach", p, &r); err != nil {
		return nil, err
	}
	return &Attached{Stream: c.open(r.Channel), Terminal: r.Terminal, Resizes: r.Resizes}, nil
}

// Event is one change from the event stream. Type is created, lost,
// restarted, keep, finished, removed, status, turn, title, labels, size,
// foreground, or doc; Missed carries a fresh snapshot instead.
type Event struct {
	Seq      uint64          `json:"seq"`
	Type     string          `json:"type"`
	Terminal string          `json:"terminal"`
	Data     json.RawMessage `json:"data"`
	// Missed is set when the subscriber fell behind: it replaces everything
	// known, and documents must be read again.
	Missed []Terminal `json:"-"`
}

// Events is the event stream of one subscription.
type Events struct {
	s       *Stream
	pending []byte
	seq     uint64
}

// Subscribe follows every terminal and document. It returns the terminals as
// they are now, and the stream of changes after that.
func (c *Client) Subscribe(ctx context.Context) (*Events, []Terminal, error) {
	var r struct {
		Seq       uint64     `json:"seq"`
		Terminals []Terminal `json:"terminals"`
		Channel   *uint32    `json:"channel"`
	}
	if err := c.Call(ctx, "events.subscribe", nil, &r); err != nil {
		return nil, nil, err
	}
	if r.Channel == nil {
		return nil, nil, errors.New("herdr-core: the subscription has no channel")
	}
	s := c.open(*r.Channel)
	s.Credit(1 << 20)
	return &Events{s: s, seq: r.Seq}, r.Terminals, nil
}

// Next returns the next event, skipping heartbeats and events a snapshot
// already covers. It returns io.EOF when the connection is gone.
func (e *Events) Next(ctx context.Context) (Event, error) {
	for {
		if i := bytes.IndexByte(e.pending, '\n'); i >= 0 {
			line := e.pending[:i]
			e.pending = e.pending[i+1:]
			if ev, ok := e.parse(line); ok {
				return ev, nil
			}
			continue
		}
		f, err := e.s.Next(ctx)
		if err != nil {
			return Event{}, err
		}
		if f.Closed != nil {
			return Event{}, io.EOF
		}
		e.pending = append(e.pending, f.Data...)
		e.s.Credit(len(f.Data))
	}
}

func (e *Events) parse(line []byte) (Event, bool) {
	var msg struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if json.Unmarshal(line, &msg) != nil {
		return Event{}, false
	}
	switch msg.Method {
	case "missed":
		var p struct {
			Seq       uint64     `json:"seq"`
			Terminals []Terminal `json:"terminals"`
		}
		if json.Unmarshal(msg.Params, &p) != nil {
			return Event{}, false
		}
		e.seq = p.Seq
		if p.Terminals == nil {
			p.Terminals = []Terminal{}
		}
		return Event{Seq: p.Seq, Type: "missed", Missed: p.Terminals}, true
	case "event":
		var ev Event
		if json.Unmarshal(msg.Params, &ev) != nil || ev.Seq <= e.seq {
			return Event{}, false
		}
		e.seq = ev.Seq
		return ev, true
	}
	return Event{}, false
}

// Apply folds one terminal event into a set of terminals keyed by address,
// as herdr-core's schema/README.md describes. It reports whether the event
// named a terminal.
func Apply(terms map[string]Terminal, ev Event) bool {
	if ev.Terminal == "" {
		return false
	}
	t, known := terms[ev.Terminal]
	switch ev.Type {
	case "created", "restarted":
		var rec Terminal
		if json.Unmarshal(ev.Data, &rec) == nil && rec.Address != "" {
			terms[ev.Terminal] = rec
		}
		return true
	case "removed":
		delete(terms, ev.Terminal)
		return true
	case "labels":
		var d struct {
			Labels map[string]string `json:"labels"`
			Record *Terminal         `json:"record"`
		}
		if json.Unmarshal(ev.Data, &d) != nil {
			return true
		}
		if d.Record != nil && d.Record.Address != "" {
			terms[ev.Terminal] = *d.Record
			return true
		}
		if known {
			t.Labels = d.Labels
		}
	case "status":
		var d struct {
			Status   *Status           `json:"status"`
			Records  map[string]Status `json:"records"`
			Reported bool              `json:"reported"`
		}
		if json.Unmarshal(ev.Data, &d) == nil && known {
			t.Status, t.Records, t.Reported = d.Status, d.Records, d.Reported
		}
	case "foreground":
		var fg *Foreground
		if json.Unmarshal(ev.Data, &fg) == nil && known {
			t.Foreground = fg
		}
	case "finished":
		if known {
			t.State, t.Foreground, t.PID = "finished", nil, 0
		}
	case "lost":
		if known {
			t.Foreground, t.PID = nil, 0
		}
	case "title":
		var d struct {
			Title string `json:"title"`
		}
		if json.Unmarshal(ev.Data, &d) == nil && known {
			t.Title = d.Title
		}
	case "size":
		var d Size
		if json.Unmarshal(ev.Data, &d) == nil && known {
			t.Cols, t.Rows = d.Cols, d.Rows
		}
	case "turn":
		var d struct {
			Turn int64 `json:"turn"`
		}
		if json.Unmarshal(ev.Data, &d) == nil && known {
			t.Turn = d.Turn
		}
	case "keep":
		var d struct {
			Keep bool `json:"keep"`
		}
		if json.Unmarshal(ev.Data, &d) == nil && known {
			t.Keep = d.Keep
		}
	}
	if known {
		terms[ev.Terminal] = t
	}
	return true
}
