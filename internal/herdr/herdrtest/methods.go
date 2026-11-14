package herdrtest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/kzark/gwen/internal/herdr"
)

// methods are the built-in methods, as hello lists them.
var methods = []string{"hello", "events.subscribe", "terminal.attach", "terminal.list", "terminal.get", "terminal.run",
	"terminal.label", "terminal.send", "terminal.keys", "terminal.read", "terminal.interrupt", "terminal.put",
	"doc.get", "doc.set"}

// decode reads a call's params, or returns the invalid_params error.
func decode(call Call, v any) error {
	if err := call.Decode(v); err != nil {
		return protoErr(-32602, "invalid_params", fmt.Sprintf("%s: %v", call.Method, err))
	}
	return nil
}

func notFound(ref string) error { return Err("not_found", "no terminal "+ref) }

// builtin is the fake's own answer to a call.
func (c *Core) builtin(call Call) (any, error) {
	switch call.Method {
	case "hello":
		return c.hello(call)
	case "events.subscribe":
		return c.subscribe(call)
	case "terminal.attach":
		return c.attach(call)
	case "terminal.list":
		return c.list(call)
	case "terminal.get":
		return c.withTerminal(call, func(t *terminal) (any, error) { return clone(t.rec), nil })
	case "terminal.run":
		return c.runTerminal(call)
	case "terminal.label":
		var p struct {
			Labels map[string]string `json:"labels"`
		}
		if err := decode(call, &p); err != nil {
			return nil, err
		}
		return c.withTerminal(call, func(t *terminal) (any, error) {
			c.labelLocked(t, p.Labels)
			return clone(t.rec), nil
		})
	case "terminal.send":
		var p struct {
			Text  string `json:"text"`
			Force bool   `json:"force"`
		}
		if err := decode(call, &p); err != nil {
			return nil, err
		}
		return c.withProgram(call, func(t *terminal) (any, error) {
			if t.rec.Status != nil && t.rec.Status.State == "blocked" && !p.Force {
				return nil, Err("blocked", t.rec.Address+" is blocked: it may be asking something")
			}
			var started any
			if t.rec.Reported {
				started = true
			}
			return map[string]any{"status": statusWord(t), "turn": t.rec.Turn, "started": started}, nil
		})
	case "terminal.keys":
		var p struct {
			Keys []string `json:"keys"`
		}
		if err := decode(call, &p); err != nil {
			return nil, err
		}
		if len(p.Keys) == 0 {
			return nil, protoErr(-32602, "invalid_params", "no keys")
		}
		return c.withProgram(call, func(t *terminal) (any, error) {
			return map[string]any{"turn": t.rec.Turn, "status": statusWord(t)}, nil
		})
	case "terminal.read":
		var p struct {
			Lines int `json:"lines"`
		}
		if err := decode(call, &p); err != nil {
			return nil, err
		}
		return c.withTerminal(call, func(t *terminal) (any, error) {
			text := t.screen
			if lines := strings.Split(text, "\n"); p.Lines > 0 && len(lines) > p.Lines {
				text = strings.Join(lines[len(lines)-p.Lines:], "\n")
			}
			return map[string]any{"text": text, "complete": true, "how": "screen", "why": nil}, nil
		})
	case "terminal.interrupt":
		return c.withProgram(call, func(t *terminal) (any, error) {
			if t.rec.Reported {
				return map[string]any{"outcome": "stopped", "key": "esc"}, nil
			}
			return map[string]any{"outcome": "unknown", "key": "ctrl+c"}, nil
		})
	case "terminal.put":
		var p struct {
			Name string `json:"name"`
			Data string `json:"data"`
		}
		if err := decode(call, &p); err != nil {
			return nil, err
		}
		data, err := base64.StdEncoding.DecodeString(p.Data)
		if err != nil || p.Name == "" {
			return nil, protoErr(-32602, "invalid_params", "put needs a name and base64 data")
		}
		if len(data) > herdr.MaxPut {
			return nil, protoErr(-32602, "invalid_params", "the file is larger than 8 MB")
		}
		return c.withTerminal(call, func(t *terminal) (any, error) {
			name := path.Base(p.Name)
			t.files[name] = data
			return map[string]any{"path": path.Join("/tmp/herdrtest", t.rec.Address, name), "bytes": len(data)}, nil
		})
	case "doc.get":
		var p struct {
			Name string `json:"name"`
		}
		if err := decode(call, &p); err != nil {
			return nil, err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		value, version := json.RawMessage("null"), uint64(0)
		if d := c.docs[p.Name]; d != nil {
			version = d.version
			if d.value != nil {
				value = append(json.RawMessage(nil), d.value...)
			}
		}
		return map[string]any{"value": value, "version": version}, nil
	case "doc.set":
		var p struct {
			Name    string          `json:"name"`
			Value   json.RawMessage `json:"value"`
			Version uint64          `json:"version"`
		}
		if err := decode(call, &p); err != nil {
			return nil, err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		var current uint64
		if d := c.docs[p.Name]; d != nil {
			current = d.version
		}
		if p.Version != current {
			return nil, Err("conflict", fmt.Sprintf("%s changed: its version is %d", p.Name, current))
		}
		return map[string]any{"version": c.setDocLocked(p.Name, p.Value)}, nil
	}
	return nil, protoErr(-32601, "unknown_method", fmt.Sprintf("unknown method %q", call.Method))
}

func statusWord(t *terminal) any {
	if t.rec.Status == nil {
		return nil
	}
	return t.rec.Status.State
}

// withTerminal runs f with the call's terminal, under the lock.
func (c *Core) withTerminal(call Call, f func(t *terminal) (any, error)) (any, error) {
	var p struct {
		Terminal string `json:"terminal"`
	}
	if err := decode(call, &p); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t := c.findLocked(p.Terminal)
	if t == nil {
		return nil, notFound(p.Terminal)
	}
	return f(t)
}

// withProgram is withTerminal for calls that need its program running.
func (c *Core) withProgram(call Call, f func(t *terminal) (any, error)) (any, error) {
	return c.withTerminal(call, func(t *terminal) (any, error) {
		if t.rec.State == "finished" {
			return nil, Err("finished", t.rec.Address+" has finished")
		}
		if t.rec.PID == 0 {
			return nil, Err("unavailable", t.rec.Address+" lost its program, waiting for its new shell")
		}
		return f(t)
	})
}

func (c *Core) hello(call Call) (any, error) {
	var p struct {
		Framed     bool `json:"framed"`
		Generation *int `json:"generation"`
	}
	if err := decode(call, &p); err != nil {
		return nil, err
	}
	if p.Generation != nil && *p.Generation != c.opts.Generation {
		e := Err("unsupported_generation", fmt.Sprintf("protocol generation %d not supported", *p.Generation))
		e.Data.Hint = fmt.Sprintf("this core speaks generation %d", c.opts.Generation)
		return nil, e
	}
	return map[string]any{"generation": c.opts.Generation, "version": c.opts.Version,
		"core": map[string]any{"id": "herdrtest", "name": "herdrtest"}, "framed": p.Framed,
		"methods": methods, "features": c.opts.Features}, nil
}

// subscribe takes the snapshot and starts the subscription in one step, so
// no event falls between them.
func (c *Core) subscribe(call Call) (any, error) {
	cn := call.cn
	c.mu.Lock()
	defer c.mu.Unlock()
	if cn.dead {
		return nil, Err("unavailable", "the connection is gone")
	}
	terms := c.snapshotLocked()
	if !cn.framed {
		// A plain connection carries only notifications from now on.
		cn.plainSub, cn.plainHold = true, true
		return map[string]any{"seq": c.seq, "terminals": terms}, nil
	}
	ch := cn.newChannelLocked()
	s := &Stream{core: c, cn: cn, channel: ch, credit: c.opts.Window, open: true, attached: true}
	cn.streams[ch], cn.all[ch] = s, s
	c.subs = append(c.subs, s)
	c.notifyLocked()
	return map[string]any{"seq": c.seq, "terminals": terms, "channel": ch}, nil
}

// attach opens a view: paused until its reply is queued, then (for an
// observer that asked) the grid, then the first paint.
func (c *Core) attach(call Call) (any, error) {
	var p herdr.AttachParams
	if err := decode(call, &p); err != nil {
		return nil, err
	}
	cn := call.cn
	c.mu.Lock()
	defer c.mu.Unlock()
	if cn.dead {
		return nil, Err("unavailable", "the connection is gone")
	}
	if !cn.framed {
		return nil, Err("needs_framed", "attach needs framed mode: send hello with framed true")
	}
	t := c.findLocked(p.Terminal)
	if t == nil {
		return nil, notFound(p.Terminal)
	}
	if t.rec.State == "finished" {
		return nil, Err("finished", t.rec.Address+" has finished")
	}
	if t.rec.PID == 0 {
		return nil, Err("unavailable", t.rec.Address+" lost its program, waiting for its new shell")
	}
	ch := cn.newChannelLocked()
	s := &Stream{core: c, cn: cn, channel: ch, terminal: t.rec.Address, params: p, notify: p.Observe && p.ResizeNotifications,
		credit: c.opts.Window, paused: true, attached: true, size: herdr.Size{Cols: p.Cols, Rows: p.Rows}}
	if !p.Observe {
		c.activity++
		s.activeAt = c.activity
	}
	c.streams = append(c.streams, s)
	cn.all[ch] = s
	// A typing viewer with another grid resizes the terminal before its own
	// paint, which then has the new grid.
	c.applySizeLocked(t.rec.Address)
	s.open = true
	cn.streams[ch] = s
	if s.notify {
		s.resizes = []resizeAt{{at: 0, size: sizeBytes(t.rec.Cols, t.rec.Rows)}}
	}
	s.queueData(Paint(t.screen))
	c.notifyLocked()
	return map[string]any{"channel": ch, "terminal": clone(t.rec), "resize_notifications": s.notify, "automatic_input": false}, nil
}

func (c *Core) list(call Call) (any, error) {
	var p struct {
		All bool `json:"all"`
	}
	if err := decode(call, &p); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	terms, hidden := []herdr.Terminal{}, 0
	for _, t := range c.snapshotLocked() {
		if t.State == "finished" && !p.All {
			hidden++
			continue
		}
		terms = append(terms, t)
	}
	return map[string]any{"terminals": terms, "hidden_finished": hidden, "machines": []any{}}, nil
}

func (c *Core) runTerminal(call Call) (any, error) {
	var p herdr.RunParams
	if err := decode(call, &p); err != nil {
		return nil, err
	}
	if len(p.Argv) == 0 {
		return nil, protoErr(-32602, "invalid_params", "argv is empty")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if p.Name != "" {
		if t := c.findLocked(p.Name); t != nil && t.rec.State != "finished" {
			return nil, Err("name_taken", "a terminal is already called "+p.Name)
		}
	}
	rec := herdr.Terminal{Name: p.Name, Argv: p.Argv, Cwd: p.Cwd, Labels: p.Labels, Keep: p.Keep}
	if p.Size != nil {
		rec.Cols, rec.Rows = p.Size.Cols, p.Size.Rows
	}
	return clone(c.addLocked(clone(rec)).rec), nil
}
