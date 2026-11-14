package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
)

func TestCanMove(t *testing.T) {
	statuses := []string{store.RunDraft, store.RunRunning, store.RunWaiting, store.RunDone, store.RunFailed, store.RunCancelled, "", "bogus"}
	allowed := map[[2]string]bool{
		{store.RunDraft, store.RunRunning}:     true,
		{store.RunDraft, store.RunCancelled}:   true,
		{store.RunRunning, store.RunWaiting}:   true,
		{store.RunRunning, store.RunDone}:      true,
		{store.RunRunning, store.RunFailed}:    true,
		{store.RunRunning, store.RunCancelled}: true,
		{store.RunWaiting, store.RunRunning}:   true,
		{store.RunWaiting, store.RunCancelled}: true,
	}
	for _, from := range statuses {
		for _, to := range statuses {
			t.Run(fmt.Sprintf("%q to %q", from, to), func(t *testing.T) {
				if got, want := CanMove(from, to), allowed[[2]string{from, to}]; got != want {
					t.Fatalf("CanMove(%q, %q) = %t, want %t", from, to, got, want)
				}
			})
		}
	}
}

type engineGate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *engineGate) unblock() { g.once.Do(func() { close(g.release) }) }

type engineKind struct {
	mu    sync.Mutex
	gates map[string]*engineGate
}

var _ Kind = (*engineKind)(nil)

func (*engineKind) Name() string { return "engine_test" }
func (*engineKind) Begin(_ context.Context, r store.Run) ([]StepSpec, error) {
	return []StepSpec{{Title: "block"}, {Title: r.Brief}}, nil
}
func (k *engineKind) Step(ctx context.Context, r store.Run, s store.RunStep, _ Exec) (StepResult, error) {
	switch s.Title {
	case "block":
		k.mu.Lock()
		g := k.gates[r.ID]
		k.mu.Unlock()
		select {
		case g.entered <- struct{}{}:
		case <-ctx.Done():
			return StepResult{}, ctx.Err()
		}
		select {
		case <-g.release:
		case <-ctx.Done():
			return StepResult{}, ctx.Err()
		}
	case "fail":
		return StepResult{Output: json.RawMessage(`{"partial":true}`)}, errors.New("controlled failure")
	}
	return StepResult{Output: json.RawMessage(`{"ok":true}`)}, nil
}

type engineMessenger struct{ *engineKind }

func (*engineMessenger) Name() string { return "engine_messenger_test" }
func (*engineMessenger) Message(context.Context, store.Run, string, Exec) error {
	return errors.New("finished run reached Messenger")
}

var _ Messenger = (*engineMessenger)(nil)

type engineFixture struct {
	repo   store.RunRepo
	clk    *clock.Fake
	engine *Engine
	kind   *engineKind
	mu     sync.Mutex
	queued map[string]int
}

func newEngineFixture(t *testing.T) *engineFixture {
	t.Helper()
	f := &engineFixture{clk: clock.NewFake(testutil.T0), kind: &engineKind{gates: map[string]*engineGate{}}, queued: map[string]int{}}
	f.repo = store.NewRunRepo(testutil.NewDBWithClock(t, f.clk))
	f.newEngine(t)
	t.Cleanup(func() {
		f.kind.mu.Lock()
		for _, g := range f.kind.gates {
			g.unblock()
		}
		f.kind.mu.Unlock()
	})
	return f
}

func (f *engineFixture) newEngine(t *testing.T) {
	t.Helper()
	f.engine = New(Options{Runs: f.repo, Clock: f.clk, Registry: connect.NewRegistry(), Publish: func(name string, data any) {
		if name == wire.EventRunChanged {
			r := data.(wire.RunChanged)
			if r.Reason == ReasonQueued {
				f.mu.Lock()
				f.queued[r.ID]++
				f.mu.Unlock()
			}
		}
	}}, f.kind, &engineMessenger{f.kind})
	t.Cleanup(f.engine.Close)
}

func (f *engineFixture) create(t *testing.T, fail bool) (store.Run, *engineGate) {
	t.Helper()
	// Distinct creation times make FIFO independent of random IDs.
	f.clk.Advance(time.Second)
	brief := "finish"
	if fail {
		brief = "fail"
	}
	r, err := f.engine.Create(context.Background(), f.kind.Name(), "", "Engine test", brief, "")
	if err != nil {
		t.Fatal(err)
	}
	g := &engineGate{entered: make(chan struct{}, 4), release: make(chan struct{})}
	f.kind.mu.Lock()
	f.kind.gates[r.ID] = g
	f.kind.mu.Unlock()
	f.limit(t)
	return r, g
}

func (f *engineFixture) start(t *testing.T, id string) store.Run {
	t.Helper()
	r, err := f.engine.Start(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	f.limit(t)
	return r
}
func (f *engineFixture) cancel(t *testing.T, id string) {
	t.Helper()
	r, err := f.engine.Cancel(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != store.RunCancelled {
		t.Fatalf("Cancel = %+v", r)
	}
	f.limit(t)
}
func (f *engineFixture) limit(t *testing.T) {
	t.Helper()
	runs, err := f.repo.Runs(context.Background(), []string{store.RunRunning}, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) > MaxRunning {
		t.Fatalf("%d running runs, limit %d: %+v", len(runs), MaxRunning, runs)
	}
}
func (f *engineFixture) row(t *testing.T, id string) store.Run {
	t.Helper()
	r, err := f.repo.Run(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f *engineFixture) status(t *testing.T, id, status, reason string) {
	t.Helper()
	if r := f.row(t, id); r.Status != status || r.Reason != reason {
		t.Fatalf("run = %+v, want %s/%s", r, status, reason)
	}
	f.limit(t)
}
func (f *engineFixture) steps(t *testing.T, id string) []store.RunStep {
	t.Helper()
	steps, err := f.repo.Steps(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return steps
}
func (f *engineFixture) wait(t *testing.T, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.engine.Wait(ctx, id)
	if ctx.Err() != nil {
		t.Fatal("engine did not stop:", ctx.Err())
	}
	f.limit(t)
}
func engineEntered(t *testing.T, g *engineGate) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("step did not enter")
	}
}
func (f *engineFixture) interrupted(t *testing.T) (store.Run, *engineGate) {
	t.Helper()
	r, g := f.create(t, false)
	f.start(t, r.ID)
	engineEntered(t, g)
	f.engine.Close()
	f.status(t, r.ID, store.RunRunning, "")
	f.newEngine(t)
	if err := f.engine.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.status(t, r.ID, store.RunWaiting, ReasonInterrupted)
	steps := f.steps(t, r.ID)
	if len(steps) != 2 || steps[0].Status != store.StepWaiting || steps[1].Status != store.StepPending {
		t.Fatalf("interrupted steps = %+v", steps)
	}
	return r, g
}

type engineSnapshot struct {
	run    store.Run
	steps  []store.RunStep
	events []store.RunEvent
}

func (f *engineFixture) snapshot(t *testing.T, id string) engineSnapshot {
	t.Helper()
	events, err := f.repo.Events(context.Background(), id, 1000)
	if err != nil {
		t.Fatal(err)
	}
	return engineSnapshot{f.row(t, id), f.steps(t, id), events}
}
func (f *engineFixture) refused(t *testing.T, id string, action func() (store.Run, error)) {
	t.Helper()
	before := f.snapshot(t, id)
	// A rejected operation must not update timestamps either.
	f.clk.Advance(time.Second)
	if _, err := action(); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("error = %v, want conflict", err)
	}
	if after := f.snapshot(t, id); !reflect.DeepEqual(before, after) {
		t.Fatalf("refused move changed state:\nbefore: %+v\nafter: %+v", before, after)
	}
	f.limit(t)
}

func TestEngineRefusedMoves(t *testing.T) {
	tests := []struct{ action, status string }{
		{"start", store.RunRunning}, {"start", store.RunWaiting}, {"start", store.RunDone}, {"start", store.RunFailed}, {"start", store.RunCancelled},
		{"cancel", store.RunDone}, {"cancel", store.RunFailed}, {"cancel", store.RunCancelled},
		{"resume", store.RunDraft}, {"resume", store.RunRunning}, {"resume", store.RunDone}, {"resume", store.RunFailed}, {"resume", store.RunCancelled},
		{"message without replies", store.RunRunning}, {"message finished", store.RunDone},
	}
	for _, tt := range tests {
		t.Run(tt.action+"/"+tt.status, func(t *testing.T) {
			f := newEngineFixture(t)
			r, g := f.create(t, tt.status == store.RunFailed)
			if tt.action == "message finished" {
				r.Kind = "engine_messenger_test"
				// Kind is immutable in UpdateRun; seed a messenger run through CreateWith.
				var err error
				r, err = f.engine.CreateWith(context.Background(), r.Kind, "", "Messenger", "", "", []StepSpec{{Title: "finish"}})
				if err != nil {
					t.Fatal(err)
				}
			}
			switch tt.status {
			case store.RunRunning:
				f.start(t, r.ID)
				engineEntered(t, g)
			case store.RunWaiting:
				r, _ = f.interrupted(t)
			case store.RunDone, store.RunFailed:
				g.unblock()
				f.start(t, r.ID)
				f.wait(t, r.ID)
			case store.RunCancelled:
				f.cancel(t, r.ID)
			}
			if got := f.row(t, r.ID).Status; got != tt.status {
				t.Fatalf("setup status = %s, want %s", got, tt.status)
			}
			f.refused(t, r.ID, func() (store.Run, error) {
				switch tt.action {
				case "start":
					return f.engine.Start(context.Background(), r.ID)
				case "cancel":
					return f.engine.Cancel(context.Background(), r.ID)
				case "resume":
					return f.engine.Resume(context.Background(), r.ID)
				default:
					return f.engine.Message(context.Background(), r.ID, "reply")
				}
			})
		})
	}
	t.Run("start twice", func(t *testing.T) {
		f := newEngineFixture(t)
		r, g := f.create(t, false)
		f.start(t, r.ID)
		engineEntered(t, g)
		f.refused(t, r.ID, func() (store.Run, error) { return f.engine.Start(context.Background(), r.ID) })
	})
}

func TestEngineAllowedMoves(t *testing.T) {
	for _, action := range []string{"cancel draft", "cancel waiting", "resume waiting", "start draft", "failing step", "cancel running"} {
		t.Run(action, func(t *testing.T) {
			f := newEngineFixture(t)
			var r store.Run
			var g *engineGate
			if action == "cancel waiting" || action == "resume waiting" {
				r, g = f.interrupted(t)
			} else {
				r, g = f.create(t, action == "failing step")
			}
			switch action {
			case "cancel draft", "cancel waiting":
				before := f.steps(t, r.ID)
				f.cancel(t, r.ID)
				if after := f.steps(t, r.ID); !reflect.DeepEqual(before, after) {
					t.Fatalf("Cancel changed inactive steps: %+v", after)
				}
				return
			case "resume waiting":
				started := f.row(t, r.ID).StartedAt
				resumed, err := f.engine.Resume(context.Background(), r.ID)
				if err != nil {
					t.Fatal(err)
				}
				if resumed.Status != store.RunRunning || !reflect.DeepEqual(resumed.StartedAt, started) {
					t.Fatalf("Resume = %+v", resumed)
				}
			default:
				if started := f.start(t, r.ID); started.Status != store.RunRunning {
					t.Fatalf("Start = %+v", started)
				}
			}
			engineEntered(t, g)
			f.limit(t)
			if action == "cancel running" {
				f.cancel(t, r.ID)
				f.wait(t, r.ID)
				steps := f.steps(t, r.ID)
				if steps[0].Status != store.StepFailed || steps[0].Error != "cancelled" || steps[1].Status != store.StepPending {
					t.Fatalf("cancelled steps = %+v", steps)
				}
				return
			}
			g.unblock()
			f.wait(t, r.ID)
			wantRun, wantStep := store.RunDone, store.StepDone
			if action == "failing step" {
				wantRun, wantStep = store.RunFailed, store.StepFailed
			}
			row, steps := f.row(t, r.ID), f.steps(t, r.ID)
			if row.Status != wantRun || row.StartedAt == nil || row.EndedAt == nil {
				t.Fatalf("finished run = %+v", row)
			}
			if len(steps) != 2 || steps[0].Status != store.StepDone || steps[1].Status != wantStep {
				t.Fatalf("finished steps = %+v", steps)
			}
			if action == "failing step" && (steps[1].Error != "controlled failure" || string(steps[1].Output) != `{"partial":true}` || row.Reason != "1 of 2 steps failed") {
				t.Fatalf("failure lost: run=%+v steps=%+v", row, steps)
			}
		})
	}
}

func TestMaxRunning(t *testing.T) {
	if MaxRunning != 2 {
		t.Fatalf("MaxRunning = %d, want 2", MaxRunning)
	}
	for _, cancelQueued := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel queued=%t", cancelQueued), func(t *testing.T) {
			f := newEngineFixture(t)
			runs := make([]store.Run, 4)
			gates := make([]*engineGate, 4)
			for i := range runs {
				runs[i], gates[i] = f.create(t, false)
				started := f.start(t, runs[i].ID)
				if i < 2 {
					if started.Status != store.RunRunning {
						t.Fatalf("Start = %+v", started)
					}
					engineEntered(t, gates[i])
				} else {
					f.status(t, runs[i].ID, store.RunDraft, ReasonQueued)
				}
				f.limit(t)
			}
			for _, r := range runs[2:] {
				before := f.snapshot(t, r.ID)
				f.start(t, r.ID)
				if after := f.snapshot(t, r.ID); !reflect.DeepEqual(before, after) {
					t.Fatalf("queued Start changed state: %+v", after)
				}
				f.mu.Lock()
				count := f.queued[r.ID]
				f.mu.Unlock()
				if count != 1 {
					t.Fatalf("queued publications = %d, want 1", count)
				}
			}
			if cancelQueued {
				f.cancel(t, runs[3].ID)
			}
			gates[0].unblock()
			f.wait(t, runs[0].ID)
			engineEntered(t, gates[2])
			f.status(t, runs[0].ID, store.RunDone, "")
			f.status(t, runs[2].ID, store.RunRunning, "")
			fourthStatus, fourthReason := store.RunDraft, ReasonQueued
			if cancelQueued {
				fourthStatus, fourthReason = store.RunCancelled, ""
			}
			f.status(t, runs[3].ID, fourthStatus, fourthReason)
			f.cancel(t, runs[1].ID)
			if !cancelQueued {
				engineEntered(t, gates[3])
				f.status(t, runs[3].ID, store.RunRunning, "")
			}
			for i, g := range gates {
				g.unblock()
				f.wait(t, runs[i].ID)
				want := store.RunDone
				if i == 1 || (cancelQueued && i == 3) {
					want = store.RunCancelled
				}
				f.status(t, runs[i].ID, want, "")
			}
			if cancelQueued {
				select {
				case <-gates[3].entered:
					t.Fatal("cancelled queued run executed")
				default:
				}
				if steps := f.steps(t, runs[3].ID); len(steps) != 0 {
					t.Fatalf("cancelled queued run gained steps: %+v", steps)
				}
			}
			remaining, err := f.repo.Runs(context.Background(), []string{store.RunDraft, store.RunRunning, store.RunWaiting}, 500)
			if err != nil {
				t.Fatal(err)
			}
			if len(remaining) != 0 {
				t.Fatalf("unfinished runs: %+v", remaining)
			}
		})
	}
}

// A run resumed while the engine is full waits for a slot as queued, once, and starts when one frees up.
func TestResumeWhileFull(t *testing.T) {
	f := newEngineFixture(t)
	interrupted, gate := f.interrupted(t)
	first, firstGate := f.create(t, false)
	second, secondGate := f.create(t, false)
	f.start(t, first.ID)
	engineEntered(t, firstGate)
	f.start(t, second.ID)
	engineEntered(t, secondGate)
	resumed, err := f.engine.Resume(context.Background(), interrupted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != store.RunWaiting || resumed.Reason != ReasonQueued {
		t.Fatalf("Resume while full = %s/%s, want waiting/queued", resumed.Status, resumed.Reason)
	}
	f.status(t, interrupted.ID, store.RunWaiting, ReasonQueued)
	if steps := f.steps(t, interrupted.ID); len(steps) != 2 || steps[0].Status != store.StepPending || steps[1].Status != store.StepPending {
		t.Fatalf("steps after Resume = %+v, want both pending", steps)
	}
	// Pressing Resume again changes nothing: no second event, no new timestamps.
	before := f.snapshot(t, interrupted.ID)
	if _, err := f.engine.Resume(context.Background(), interrupted.ID); err != nil {
		t.Fatal(err)
	}
	if after := f.snapshot(t, interrupted.ID); !reflect.DeepEqual(before, after) {
		t.Fatalf("a second Resume changed the queued run: %+v", after)
	}
	f.limit(t)
	firstGate.unblock()
	f.wait(t, first.ID) // execute's next() has returned before Wait completes.
	engineEntered(t, gate)
	f.status(t, interrupted.ID, store.RunRunning, "")
	gate.unblock()
	f.wait(t, interrupted.ID)
	f.status(t, interrupted.ID, store.RunDone, "")
	secondGate.unblock()
	f.wait(t, second.ID)
	f.status(t, second.ID, store.RunDone, "")
}

// Cancelling a resumed run that waits for a slot ends it, and it never starts.
func TestCancelResumedQueuedRun(t *testing.T) {
	f := newEngineFixture(t)
	interrupted, gate := f.interrupted(t)
	first, firstGate := f.create(t, false)
	second, secondGate := f.create(t, false)
	f.start(t, first.ID)
	engineEntered(t, firstGate)
	f.start(t, second.ID)
	engineEntered(t, secondGate)
	if _, err := f.engine.Resume(context.Background(), interrupted.ID); err != nil {
		t.Fatal(err)
	}
	f.status(t, interrupted.ID, store.RunWaiting, ReasonQueued)
	f.cancel(t, interrupted.ID)
	f.status(t, interrupted.ID, store.RunCancelled, "")
	firstGate.unblock()
	f.wait(t, first.ID)
	select {
	case <-gate.entered:
		t.Fatal("a cancelled run executed")
	default:
	}
	f.status(t, interrupted.ID, store.RunCancelled, "")
	secondGate.unblock()
	f.wait(t, second.ID)
}

// After a restart the runs that were running are interrupted, and the runs that were queued for a slot start,
// the oldest first, instead of waiting for some other run to end.
func TestRecoverStartsQueuedRuns(t *testing.T) {
	f := newEngineFixture(t)
	resumed, resumedGate := f.interrupted(t)
	first, firstGate := f.create(t, false)
	second, secondGate := f.create(t, false)
	queued, queuedGate := f.create(t, false)
	f.start(t, first.ID)
	engineEntered(t, firstGate)
	f.start(t, second.ID)
	engineEntered(t, secondGate)
	f.start(t, queued.ID)
	f.status(t, queued.ID, store.RunDraft, ReasonQueued)
	if _, err := f.engine.Resume(context.Background(), resumed.ID); err != nil {
		t.Fatal(err)
	}
	f.status(t, resumed.ID, store.RunWaiting, ReasonQueued)

	f.engine.Close() // Gwen stops: the two running runs stay running, as a restart finds them.
	f.status(t, first.ID, store.RunRunning, "")
	f.status(t, second.ID, store.RunRunning, "")
	f.newEngine(t)
	if err := f.engine.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.status(t, first.ID, store.RunWaiting, ReasonInterrupted)
	f.status(t, second.ID, store.RunWaiting, ReasonInterrupted)
	engineEntered(t, resumedGate)
	engineEntered(t, queuedGate)
	f.status(t, resumed.ID, store.RunRunning, "")
	f.status(t, queued.ID, store.RunRunning, "")
	resumedGate.unblock()
	queuedGate.unblock()
	f.wait(t, resumed.ID)
	f.wait(t, queued.ID)
	f.status(t, resumed.ID, store.RunDone, "")
	f.status(t, queued.ID, store.RunDone, "")
}
