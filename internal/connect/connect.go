// Package connect is how Gwen touches the world: connectors (the desktop,
// the calendar, Claude Code) offer capabilities, each with a risk, and the
// harness calls them only through the policy in policy.go (plan.md 3.8).
// Run kinds ask for a capability by name and never import a connector.
package connect

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Risk is how much a capability can change, which decides whether it needs
// your approval (policy.go).
type Risk string

// Risks, from least to most.
const (
	Read        Risk = "read"        // list emails, read a calendar
	Local       Risk = "local"       // open a URL or folder, start tracking, speak
	Write       Risk = "write"       // create a draft, edit files in a run's worktree, start a Claude session
	External    Risk = "external"    // send, push, post anywhere
	Destructive Risk = "destructive" // delete mail or files: not offered in this phase
)

// Risks are every risk, least first.
var Risks = []Risk{Read, Local, Write, External, Destructive}

// Valid reports whether r is a known risk.
func (r Risk) Valid() bool { return slices.Contains(Risks, r) }

// Connection states.
const (
	Connected  = "connected"
	NeedsSetup = "needs_setup"
	Failed     = "error"
)

// Status is how a connector stands, with a message for the dashboard.
type Status struct {
	State   string
	Message string
}

// Capability is one thing a connector can do.
type Capability struct {
	// Name is "<area>.<verb>", such as "browser.open".
	Name string
	// Title says what it does, in a few words: "Open links in your browser".
	Title string
	Risk  Risk
	// Input describes the JSON input, for the assistant and the dashboard:
	// `{"urls": ["https://..."]}`.
	Input string
	// Check decodes and checks an input, returning it as it will be used.
	// It must not touch the world.
	Check func(input json.RawMessage) (json.RawMessage, error)
	// Summary says in a sentence what this call will do, for an approval
	// card and the run log. It must never include a secret.
	Summary func(input json.RawMessage) string
	// Scope is the folder this call works in, for grants; nil when none.
	Scope func(input json.RawMessage) string
	// Invoke does it and says what happened. An error is an honest failure:
	// "Couldn't open the browser".
	Invoke func(ctx context.Context, input json.RawMessage) (json.RawMessage, error)
}

// Connector offers capabilities.
type Connector interface {
	// ID is "desktop", "calendar", "claude_code", and later "gmail" or
	// "mcp:<name>".
	ID() string
	// Name is how the dashboard names it.
	Name() string
	Status(ctx context.Context) Status
	Capabilities() []Capability
}

// Linker is a connector that can be connected and disconnected from the
// dashboard (Gmail in W22). The desktop and the calendar are not.
type Linker interface {
	Connect(ctx context.Context) error
	Disconnect(ctx context.Context) error
}

// Registry holds the connectors of this device.
type Registry struct {
	mu    sync.RWMutex
	conns []Connector
	caps  map[string]entry
}

type entry struct {
	cap  Capability
	conn string
}

// NewRegistry builds a registry of these connectors. Two capabilities of
// one name are a programming error.
func NewRegistry(conns ...Connector) *Registry {
	r := &Registry{caps: map[string]entry{}}
	for _, c := range conns {
		r.Add(c)
	}
	return r
}

// Add adds a connector and its capabilities.
func (r *Registry) Add(c Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cp := range c.Capabilities() {
		if _, dup := r.caps[cp.Name]; dup {
			panic(fmt.Sprintf("connect: capability %s offered twice", cp.Name))
		}
		if !cp.Risk.Valid() || cp.Invoke == nil || cp.Check == nil || cp.Summary == nil {
			panic(fmt.Sprintf("connect: capability %s is incomplete", cp.Name))
		}
		r.caps[cp.Name] = entry{cap: cp, conn: c.ID()}
	}
	r.conns = append(r.conns, c)
}

// Capability returns a capability by name and the id of its connector.
func (r *Registry) Capability(name string) (Capability, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.caps[name]
	return e.cap, e.conn, ok
}

// Connectors are the connectors, in the order added.
func (r *Registry) Connectors() []Connector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.conns)
}

// Connector returns one connector by id.
func (r *Registry) Connector(id string) (Connector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, c := range r.conns {
		if c.ID() == id {
			return c, true
		}
	}
	return nil, false
}

// Capabilities are every capability offered, by name, without the
// destructive ones, which are not offered in this phase.
func (r *Registry) Capabilities() []Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Capability, 0, len(r.caps))
	for _, e := range r.caps {
		if e.cap.Risk != Destructive {
			out = append(out, e.cap)
		}
	}
	slices.SortFunc(out, func(a, b Capability) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Simple is a connector made of a name, a status, and capabilities.
type Simple struct {
	IDValue, NameValue string
	StatusFunc         func(ctx context.Context) Status
	Caps               []Capability
}

func (s Simple) ID() string                 { return s.IDValue }
func (s Simple) Name() string               { return s.NameValue }
func (s Simple) Capabilities() []Capability { return s.Caps }

// Status is StatusFunc's, or connected when there is none.
func (s Simple) Status(ctx context.Context) Status {
	if s.StatusFunc == nil {
		return Status{State: Connected}
	}
	return s.StatusFunc(ctx)
}

// Decode reads an input strictly into v: unknown fields fail, so a typo
// never silently drops a field.
func Decode(input json.RawMessage, v any) error {
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}
	dec := json.NewDecoder(strings.NewReader(string(input)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("the input doesn't fit: %w", err)
	}
	return nil
}

// Checked builds a Check that decodes into T, runs valid, and returns the
// input as decoded.
func Checked[T any](valid func(*T) error) func(json.RawMessage) (json.RawMessage, error) {
	return func(input json.RawMessage) (json.RawMessage, error) {
		var v T
		if err := Decode(input, &v); err != nil {
			return nil, err
		}
		if valid != nil {
			if err := valid(&v); err != nil {
				return nil, err
			}
		}
		return json.Marshal(v)
	}
}
