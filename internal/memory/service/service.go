// Package service puts Gwen's memory together (harness section 4): the write
// path (learn), the read path (recall), the lanes that hold copies of what
// she knows (vec, mem0, supermemory, the memory folder), and the worker that
// learns, projects, and consolidates in the background. Service implements
// memory.Memory, which the API serves, and the API's resolver of the refs
// chats show.
//
// The configuration is live: every use reads it, a lane is rebuilt when its
// settings change, and with memory switched off every method answers
// memory.ErrDisabled while the worker idles.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/memory/learn"
	"github.com/kzark/gwen/internal/memory/recall"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// Options are what the service works with.
type Options struct {
	DB    *store.DB
	Repos store.Repos
	Clock clock.Clock // nil is the wall clock
	Loc   *time.Location
	// DataDir holds index.db; CredDir the optional lanes' API keys.
	DataDir, CredDir string
	// Config is the configuration in effect, read at every use.
	Config func() config.Config
	// LLM makes the model of a configuration (llm.NewMemory with the
	// daemon's options); nil is none, and learning is paused.
	LLM        func(cfg config.LLM) (llm.Memory, error)
	HTTPClient *http.Client
	// Publish sends an event to the API's subscribers; nil sends none.
	Publish func(name string, data any)
	// Day is the local day of an instant, aware of the day rollover; nil is
	// the date in Loc.
	Day func(time.Time) string
	// Idle reports whether the user is not working right now; nil is never.
	Idle func(ctx context.Context) bool
	// RecallScale stretches the read path's deadlines (recall.Options.Scale);
	// tests under the race detector set it.
	RecallScale float64
}

// laneGwen names Gwen's own store in receipts and the status: the claims,
// episodes, and ledger in gwen.db.
const laneGwen memory.LaneName = "gwen"

// Service is Gwen's memory. Its methods are safe to call at once.
type Service struct {
	o       Options
	loc     *time.Location
	learner *learn.Learner
	router  *recall.Router
	data    *dataLane

	// modelMu lets one model call run at a time across the service: the
	// user's laptop is small.
	modelMu sync.Mutex

	lanesMu sync.Mutex
	lanes   laneSet
	make    factories

	tickMu  sync.Mutex // one worker tick at a time
	sleepMu sync.Mutex // one consolidation at a time
	filesMu sync.Mutex // one scan and projection of the folder at a time

	wake  chan struct{}
	force atomic.Bool // the next extraction runs even when nothing is due

	// profileDirty is set when claims changed since the newest card.
	profileDirty atomic.Bool

	stateMu    sync.Mutex
	extract    backoffState // extraction after model errors
	idleSince  time.Time    // when Idle last became true; zero while working
	sleepAfter time.Time    // a consolidation that failed is not tried again before
	health     map[memory.LaneName]healthEntry
	ledger     ledgerEntry
	filesErr   string // why the last projection of the folder failed, "" when it did not

	closed atomic.Bool
}

// backoffState is how a failing step waits before trying again.
type backoffState struct {
	fails   int
	until   time.Time
	lastErr string
}

// New returns the service. Nothing runs until Run.
func New(o Options) (*Service, error) {
	if o.DB == nil || o.Config == nil {
		return nil, errors.New("memory service: a database and a configuration are needed")
	}
	if o.Clock == nil {
		o.Clock = clock.Real()
	}
	loc := o.Loc
	if loc == nil {
		loc = time.Local
	}
	o.Loc = loc
	if o.Day == nil {
		o.Day = func(t time.Time) string { return t.In(loc).Format(time.DateOnly) }
	}
	s := &Service{o: o, loc: loc, wake: make(chan struct{}, 1), make: defaultFactories(o),
		health: map[memory.LaneName]healthEntry{}}
	s.profileDirty.Store(true)
	s.data = &dataLane{s: s}
	s.router = recall.New(recall.Options{Repo: o.Repos.Memory, State: o.Repos.MemoryState, Chats: o.Repos.Chats,
		Data: s.data, Clock: o.Clock, Loc: loc, Day: o.Day, Scale: o.RecallScale})
	s.learner = learn.New(learn.Options{Repo: o.Repos.Memory, State: o.Repos.MemoryState, DB: o.DB,
		Model: s.model, Clock: o.Clock, Loc: loc, Day: o.Day,
		Verify: func() bool { return s.cfg().Verify },
		Data:   s.data.summaries})
	return s, nil
}

// Close closes the lanes. The worker stops with its context.
func (s *Service) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.lanesMu.Lock()
	defer s.lanesMu.Unlock()
	return s.lanes.close()
}

// cfg is the [memory] section in effect.
func (s *Service) cfg() config.Memory { return s.o.Config().Memory }

// enabled reports whether memory is switched on.
func (s *Service) enabled() bool { return s.cfg().Enabled }

// on returns memory.ErrDisabled when memory is switched off.
func (s *Service) on() error {
	if !s.enabled() {
		return memory.ErrDisabled
	}
	return nil
}

// now is the current instant, to the millisecond the store keeps.
func (s *Service) now() time.Time { return s.o.Clock.Now().Truncate(time.Millisecond) }

// today is the local day now.
func (s *Service) today() string { return s.o.Day(s.now()) }

// model returns the model memory learns with now, or why there is none:
// learning is off, or the model is not set up. Its calls run one at a time.
func (s *Service) model() (llm.Memory, error) {
	all := s.o.Config()
	m := all.Memory
	if !m.Enabled {
		return nil, errors.New("memory is off")
	}
	if s.o.LLM == nil {
		return nil, errors.New("no model is set up")
	}
	var (
		mm  llm.Memory
		err error
	)
	switch m.Learn {
	case config.LearnOff:
		return nil, errors.New("learning from chats is off")
	case config.LearnLocal:
		mm, err = s.o.LLM(config.LLM{Provider: config.ProviderOpenAICompatible, Endpoint: m.LocalEndpoint,
			Model: m.LocalModel, Timeout: all.LLM.Timeout})
	default:
		mm, err = s.o.LLM(all.LLM)
	}
	if err != nil {
		return nil, err
	}
	if mm == nil {
		return nil, errors.New("no model is set up")
	}
	return &lockedModel{m: mm, mu: &s.modelMu}, nil
}

// learning reports whether a model is set up to learn, and in words with
// what, or why not.
func (s *Service) learning() (bool, string) {
	m, err := s.model()
	if err != nil {
		return false, "learning paused: " + err.Error()
	}
	detail := "learning with " + m.Name()
	s.stateMu.Lock()
	if e := s.extract; e.lastErr != "" {
		detail += "; the last try failed: " + e.lastErr
		if e.until.After(s.now()) {
			detail += fmt.Sprintf(" (trying again after %s)", e.until.In(s.loc).Format("15:04"))
		}
	}
	s.stateMu.Unlock()
	return true, detail
}

// Wake runs a worker tick soon; force makes it extract even when nothing is
// due yet.
func (s *Service) Wake(force bool) {
	if force {
		s.force.Store(true)
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// changed records that claims or conflicts changed: the profile card is
// compiled again, and memory_changed tells the dashboard. Nil ids and no
// conflicts is no change; an empty slice is claims too many to name.
func (s *Service) changed(ctx context.Context, ids []string, conflicts bool) {
	if ids == nil && !conflicts {
		return
	}
	s.profileDirty.Store(true)
	err := s.o.Repos.MemoryState.Enqueue(ctx, store.NewJob{Kind: model.JobProfile, Subject: s.today(), RunAfter: s.now()})
	if err != nil {
		slog.Warn("memory: profile job not queued", "err", err)
	}
	s.publish(ids, conflicts)
}

// publish sends memory_changed.
func (s *Service) publish(ids []string, conflicts bool) {
	if s.o.Publish == nil {
		return
	}
	if ids == nil {
		ids = []string{}
	}
	s.o.Publish(wire.EventMemoryChanged, wire.MemoryChanged{ClaimIDs: ids, Conflicts: conflicts})
}

// lockedModel runs one model call at a time across the service.
type lockedModel struct {
	m  llm.Memory
	mu *sync.Mutex
}

func (l *lockedModel) Name() string { return l.m.Name() }

func (l *lockedModel) Extract(ctx context.Context, req llm.ExtractRequest) (llm.ExtractOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Extract(ctx, req)
}

func (l *lockedModel) Verify(ctx context.Context, req llm.VerifyRequest) (llm.VerifyOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Verify(ctx, req)
}

func (l *lockedModel) Observe(ctx context.Context, req llm.ObserveRequest) (llm.ObserveOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Observe(ctx, req)
}

func (l *lockedModel) Reflect(ctx context.Context, req llm.ReflectRequest) (llm.ReflectOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Reflect(ctx, req)
}

func (l *lockedModel) Contradict(ctx context.Context, req llm.ContradictRequest) (llm.ContradictOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Contradict(ctx, req)
}

func (l *lockedModel) Insight(ctx context.Context, req llm.InsightRequest) (llm.InsightOutput, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.m.Insight(ctx, req)
}

// rememberCues are words in a user message that make it worth learning from
// at once; the learner checks the same.
var rememberCues = []string{"remember", "don't forget", "note that", "keep in mind", "for future reference"}

// hasCue reports whether text asks to be remembered.
func hasCue(text string) bool {
	text = strings.ToLower(memory.Normalize(text))
	for _, c := range rememberCues {
		if strings.Contains(text, c) {
			return true
		}
	}
	return false
}

// isNotFound reports whether err is the store's not found.
func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }

// bareClaimID strips claim: from a ref, leaving an id as it is.
func bareClaimID(s string) string {
	id, _ := strings.CutPrefix(s, memory.RefClaim+":")
	return id
}
