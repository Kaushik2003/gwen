// Package learn is the write path of Gwen's memory: it reads chats and notes
// with a model, checks what the model proposes against the words it came
// from, and commits claims, with their history, conflicts, and ledger rows,
// through store.MemoryRepo.Apply. It also carries out the user's own changes
// (remember, edit, confirm, reject, suppress, forget, resolve) and the
// consolidation done while the user is away: observation logs, the
// contradiction scan, merging duplicates, insights, and the profile card.
package learn

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// Options are what a Learner works with.
type Options struct {
	Repo  store.MemoryRepo
	State store.MemoryStateRepo
	DB    *store.DB // local_state cursors, DeviceID
	// Model returns the model now, or an error when there is none (learning
	// is paused). Nil is never a model.
	Model func() (llm.Memory, error)
	Clock clock.Clock // nil is the wall clock
	Loc   *time.Location
	// Day is the local day of an instant, aware of the day rollover; nil is
	// the date in Loc.
	Day func(time.Time) string
	// Verify reports whether the entailment check is on; nil is off.
	Verify func() bool
	// Data returns summaries of the user's tracked data, for insights; nil
	// is none.
	Data func(ctx context.Context) ([]string, error)
}

// Learner is the write path. Its methods are safe to call at once: writes
// decide and commit one at a time, and model calls happen outside that.
type Learner struct {
	o     Options
	loc   *time.Location
	chats store.ChatRepo

	mu        sync.Mutex // held while a write reads what it changes and applies it
	extractMu sync.Mutex // one extraction at a time, so cursors move in order
	observeMu sync.Mutex // one observation log fold at a time
	insightMu sync.Mutex // one insight run at a time
}

// New returns a Learner.
func New(o Options) *Learner {
	if o.Clock == nil {
		o.Clock = clock.Real()
	}
	loc := o.Loc
	if loc == nil {
		loc = time.Local
	}
	return &Learner{o: o, loc: loc, chats: store.NewChatRepo(o.DB)}
}

// Result is what a write changed, for memory_changed events: the claims that
// are new or changed, whether a conflict was opened, and how many claims were
// learned.
type Result struct {
	Claims    []string
	Conflicts bool
	Learned   int
}

// merge adds r's changes to res.
func (res *Result) merge(r Result) {
	for _, id := range r.Claims {
		if !slices.Contains(res.Claims, id) {
			res.Claims = append(res.Claims, id)
		}
	}
	res.Conflicts = res.Conflicts || r.Conflicts
	res.Learned += r.Learned
}

// Channel of claims inferred from the user's tracked data. Channels are an
// open list; the others are model.Channel*.
const channelData = "data"

// Extractor of claims the user made or changed by hand.
const extractorUser = "user"

// now is the current instant, to the millisecond the store keeps.
func (l *Learner) now() time.Time { return l.o.Clock.Now().Truncate(time.Millisecond) }

// day is the local day of t.
func (l *Learner) day(t time.Time) string {
	if l.o.Day != nil {
		return l.o.Day(t)
	}
	return t.In(l.loc).Format(time.DateOnly)
}

// model returns the model, or an error matching memory.ErrDisabled.
func (l *Learner) model() (llm.Memory, error) {
	if l.o.Model == nil {
		return nil, memory.ErrDisabled
	}
	m, err := l.o.Model()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", memory.ErrDisabled, err)
	}
	if m == nil {
		return nil, memory.ErrDisabled
	}
	return m, nil
}

// verifyOn reports whether the entailment check is on.
func (l *Learner) verifyOn() bool { return l.o.Verify != nil && l.o.Verify() }

// extractorOf is the extractor recorded with the claims a model proposed.
func extractorOf(m llm.Memory) string { return m.Name() + "#" + llm.MemoryPromptVersion }

// localStamp is how an instant reads to a model: local "2006-01-02 15:04".
func (l *Learner) localStamp(t time.Time) string { return t.In(l.loc).Format("2006-01-02 15:04") }

// parseDay reads a model's or a user's date: YYYY-MM-DD is local midnight in
// Loc, and RFC 3339 is accepted too. When end is set, a date is the last day
// of a span, which is stored exclusive: midnight of the day after. Anything
// else is nil.
func (l *Learner) parseDay(s string, end bool) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if d, err := time.ParseInLocation(time.DateOnly, s, l.loc); err == nil {
		if end {
			d = d.AddDate(0, 0, 1)
		}
		return &d
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		t = t.Truncate(time.Millisecond)
		return &t
	}
	return nil
}

// dayOf is the local date of t, "" for nil; for an exclusive end it is the
// last day the span holds.
func (l *Learner) dayOf(t *time.Time, end bool) string {
	if t == nil {
		return ""
	}
	at := *t
	if end {
		at = at.Add(-time.Millisecond)
	}
	return at.In(l.loc).Format(time.DateOnly)
}

// getCursor reads a rowid cursor of local_state; absent is 0.
func (l *Learner) getCursor(ctx context.Context, key string) (int64, error) {
	v, err := store.GetLocal(ctx, l.o.DB.SQL(), key)
	if errors.Is(err, store.ErrNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var n int64
	if _, err := fmt.Sscan(v, &n); err != nil {
		return 0, fmt.Errorf("local state %s: %q is not a rowid", key, v)
	}
	return n, nil
}

// setCursor stores a rowid cursor.
func (l *Learner) setCursor(ctx context.Context, key string, rowid int64) error {
	return store.SetLocal(ctx, l.o.DB.SQL(), key, fmt.Sprint(rowid), l.now())
}

// jsonString is the JSON of a string value.
func jsonString(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

// stringValue is a claim's value when it is a JSON string.
func stringValue(v []byte) (string, bool) {
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}

// statusFor is the status a new claim of trust gets (spec section 1).
func statusFor(trust string) string {
	switch trust {
	case model.TrustInferred:
		return model.ClaimCandidate
	case model.TrustExternal:
		return model.ClaimQuarantined
	}
	return model.ClaimActive
}

// live reports whether a claim counts as held or awaiting the user: active,
// candidate, or quarantined, and not forgotten.
func live(c model.Claim) bool {
	if c.DeletedAt != nil {
		return false
	}
	switch c.Status {
	case model.ClaimActive, model.ClaimCandidate, model.ClaimQuarantined:
		return true
	}
	return false
}

// linkKind is the memory_links to_kind of a claim source kind.
func linkKind(sourceKind string) string {
	switch sourceKind {
	case model.SourceMessage:
		return "message"
	case model.SourceEpisode:
		return "episode"
	case model.SourceClaim:
		return "claim"
	}
	return sourceKind
}

// projectJob brings a claim up to date in every lane.
func projectJob(id string) store.NewJob {
	return store.NewJob{Kind: model.JobProject, Subject: memory.Ref(memory.RefClaim, id)}
}

// filesJob writes the memory folder again.
func filesJob() store.NewJob { return store.NewJob{Kind: model.JobFiles} }

// ledgerRow is a ledger row of a claim's transition.
func ledgerRow(action string, c *model.Claim, extra ...any) store.LedgerItem {
	data := map[string]any{"action": action, "claim": c.ID, "hash": c.ContentHash, "status": c.Status,
		"trust": c.Trust}
	for i := 0; i+1 < len(extra); i += 2 {
		data[fmt.Sprint(extra[i])] = extra[i+1]
	}
	return store.LedgerItem{Data: data}
}
