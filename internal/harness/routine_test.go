package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

// A routine run (plan.md 3.7, 3.11) is the subject's "when it starts" steps,
// each one capability call, run in order. These tests run it with a fake
// opener and read back what the run says about every step: the step, and the
// log line by line. The DSA workflow is the example: open the problem links,
// open the folder, start tracking.

// logLine is one line of a run's log: its kind, the position of the step it
// belongs to (-1 for the run's own lines), and its data.
type logLine struct {
	Kind string
	Step int
	Data map[string]string
}

const (
	routineSubject = "event:local/dsa"
	problemOne     = "https://leetcode.com/problems/two-sum/"
	problemTwo     = "https://example.com/a/b?x=1"
	dsaLinks       = `{"urls":["` + problemOne + `","` + problemTwo + `"]}`
)

type routineFixture struct {
	repo   store.RunRepo
	engine *Engine
	opener *tokenOpener
	folder string // an existing folder in the fake home
	// steps are the routine every subject has; nil means none.
	steps   []store.RoutineStep
	tracked []string
}

// newRoutineFixture is an engine with the desktop connector over a fake
// opener, whose open is what opening a link does (nil: it works), and the
// routine kind, which finds f.steps.
func newRoutineFixture(t *testing.T, open func(context.Context, string) error) *routineFixture {
	t.Helper()
	home := t.TempDir()
	f := &routineFixture{repo: store.NewRunRepo(testutil.NewDB(t)), opener: &tokenOpener{open: open}, folder: filepath.Join(home, "dsa")}
	require.NoError(t, os.Mkdir(f.folder, 0o755))
	reg := connect.NewRegistry(connect.Desktop(f.opener, func(_ context.Context, project, task string) (string, error) {
		f.tracked = append(f.tracked, project+":"+task)
		return "practice", nil
	}, home))
	f.engine = New(Options{Runs: f.repo, Registry: reg, Clock: clock.NewFake(testutil.T0)},
		Routine(reg, func(context.Context, string) (store.Routine, bool, error) {
			return store.Routine{Steps: f.steps}, f.steps != nil, nil
		}))
	t.Cleanup(f.engine.Close)
	return f
}

// dsaSteps is the DSA routine: the links, the folder (with a title of its
// own), tracking.
func (f *routineFixture) dsaSteps() []store.RoutineStep {
	return []store.RoutineStep{
		{Capability: "browser.open", Input: json.RawMessage(dsaLinks)},
		{Title: "Open the DSA folder", Capability: "folder.open", Input: json.RawMessage(`{"path":` + quote(f.folder) + `}`)},
		{Capability: "tracking.start", Input: json.RawMessage(`{"project":"practice"}`)},
	}
}

// run makes a routine run for subject, starts it, and returns it once it has
// ended and written its last log line, with its steps and its log.
func (f *routineFixture) run(t *testing.T, subject string) (store.Run, []store.RunStep, []logLine) {
	t.Helper()
	ctx := context.Background()
	r, err := f.engine.Create(ctx, KindRoutine, subject, "DSA", "", "")
	require.NoError(t, err)
	_, err = f.engine.Start(ctx, r.ID)
	require.NoError(t, err)
	// finish is the only writer of a log line with a reason, and it writes it last.
	deadline := time.Now().Add(10 * time.Second)
	var events []store.RunEvent
	for {
		events, err = f.repo.Events(ctx, r.ID, 1000)
		require.NoError(t, err)
		if n := len(events); n > 0 && events[n-1].StepID == "" && strings.Contains(string(events[n-1].Data), `"reason"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the run never ended; its log: %+v", events)
		}
		time.Sleep(5 * time.Millisecond)
	}
	row, err := f.repo.Run(ctx, r.ID)
	require.NoError(t, err)
	steps, err := f.repo.Steps(ctx, r.ID)
	require.NoError(t, err)
	position := map[string]int{}
	for _, s := range steps {
		position[s.ID] = s.Position
	}
	lines := make([]logLine, len(events))
	for i, ev := range events {
		lines[i] = logLine{Kind: ev.Kind, Step: -1, Data: map[string]string{}}
		if ev.StepID != "" {
			p, found := position[ev.StepID]
			require.True(t, found, "log line %q belongs to no step of the run", ev.Kind)
			lines[i].Step = p
		}
		require.NoError(t, json.Unmarshal(ev.Data, &lines[i].Data), "log line %q", ev.Kind)
	}
	return row, steps, lines
}

func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// stepDone is the log of a step whose call went through.
func stepDone(step int, capability, summary, title string) []logLine {
	return []logLine{
		{"call", step, map[string]string{"capability": capability, "summary": summary}},
		{"call_done", step, map[string]string{"capability": capability}},
		{"step_done", step, map[string]string{"title": title, "error": ""}},
	}
}

func joined(parts ...[]logLine) []logLine {
	var out []logLine
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// Every step of a routine that goes well is done, with the output of its
// call, and the log tells it in order: what each call will do, that it was
// done, and that the step is done; then the run is done.
func TestRoutineRunLogsEachStep(t *testing.T) {
	f := newRoutineFixture(t, nil)
	f.steps = f.dsaSteps()

	run, steps, lines := f.run(t, routineSubject)

	require.Equal(t, store.RunDone, run.Status)
	require.Empty(t, run.Reason)
	require.NotNil(t, run.StartedAt)
	require.NotNil(t, run.EndedAt)
	require.Equal(t, []string{problemOne, problemTwo}, f.opener.urls, "one open per link, in order")
	require.Equal(t, []string{f.folder}, f.opener.folders)
	require.Equal(t, []string{"practice:"}, f.tracked)

	require.Len(t, steps, 3)
	for i, want := range []struct{ title, capability string }{
		{"Open 2 links in your browser", "browser.open"},
		{"Open the DSA folder", "folder.open"},
		{"Start tracking time", "tracking.start"},
	} {
		s := steps[i]
		require.Equal(t, i, s.Position)
		require.Equal(t, want.title, s.Title, "a step is named by its title, else by what its call will do")
		require.Equal(t, want.capability, s.Capability)
		require.Equal(t, string(connect.Local), s.Risk)
		require.Equal(t, store.StepDone, s.Status, "step %d: %+v", i, s)
		require.Empty(t, s.Error)
		require.NotNil(t, s.StartedAt)
		require.NotNil(t, s.EndedAt)
	}
	require.JSONEq(t, `{"links":[{"url":"`+problemOne+`","opened":true},{"url":"`+problemTwo+`","opened":true}]}`, string(steps[0].Output))
	require.JSONEq(t, `{"opened":`+quote(f.folder)+`}`, string(steps[1].Output))
	require.JSONEq(t, `{"tracking":"practice"}`, string(steps[2].Output))

	require.Equal(t, joined(
		stepDone(0, "browser.open", "Open 2 links in your browser", "Open 2 links in your browser"),
		stepDone(1, "folder.open", "Open ~/dsa in your file manager", "Open the DSA folder"),
		stepDone(2, "tracking.start", "Start tracking time", "Start tracking time"),
		[]logLine{{"done", -1, map[string]string{"reason": ""}}},
	), lines)
}

// A link that doesn't open is reported, not hidden: the step fails with what
// went wrong and keeps which links did open, the log says so, and the steps
// after it still run.
func TestRoutineRunReportsAFailedOpen(t *testing.T) {
	const why = "exec: xdg-open: executable file not found in $PATH"
	tests := []struct {
		name      string
		fails     func(url string) bool
		stepError string
		links     string
	}{
		{"one link", func(u string) bool { return u == problemTwo }, "couldn't open 1 of the 2 links",
			`{"links":[{"url":"` + problemOne + `","opened":true},{"url":"` + problemTwo + `","opened":false,"error":"` + why + `"}]}`},
		{"every link", func(string) bool { return true }, "couldn't open the browser",
			`{"links":[{"url":"` + problemOne + `","opened":false,"error":"` + why + `"},{"url":"` + problemTwo + `","opened":false,"error":"` + why + `"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRoutineFixture(t, func(_ context.Context, u string) error {
				if tt.fails(u) {
					return errors.New(why)
				}
				return nil
			})
			f.steps = f.dsaSteps()

			run, steps, lines := f.run(t, routineSubject)

			require.Equal(t, store.RunFailed, run.Status)
			require.Equal(t, "1 of 3 steps failed", run.Reason)
			require.Equal(t, []string{problemOne, problemTwo}, f.opener.urls, "every link was tried, even after one failed")
			require.Len(t, steps, 3)
			require.Equal(t, store.StepFailed, steps[0].Status)
			require.Equal(t, tt.stepError, steps[0].Error)
			require.JSONEq(t, tt.links, string(steps[0].Output), "the step keeps which links opened and why the others didn't")
			require.Equal(t, store.StepDone, steps[1].Status, "the steps after a failed one still run")
			require.Equal(t, store.StepDone, steps[2].Status)
			require.Equal(t, []string{f.folder}, f.opener.folders)
			require.Equal(t, []string{"practice:"}, f.tracked)

			require.Equal(t, joined(
				[]logLine{
					{"call", 0, map[string]string{"capability": "browser.open", "summary": "Open 2 links in your browser"}},
					{"call_failed", 0, map[string]string{"capability": "browser.open", "error": tt.stepError}},
					{"step_failed", 0, map[string]string{"title": "Open 2 links in your browser", "error": tt.stepError}},
				},
				stepDone(1, "folder.open", "Open ~/dsa in your file manager", "Open the DSA folder"),
				stepDone(2, "tracking.start", "Start tracking time", "Start tracking time"),
				[]logLine{{"failed", -1, map[string]string{"reason": "1 of 3 steps failed"}}},
			), lines)
		})
	}
}

// A step whose call can't be made at all fails before anything is opened and
// without a call line: a capability that is gone since the routine was saved,
// and an input that no longer checks.
func TestRoutineRunRefusesCallsThatDontCheck(t *testing.T) {
	tests := []struct {
		name  string
		step  store.RoutineStep
		fails string
	}{
		{"no such capability", store.RoutineStep{Capability: "mail.send", Input: json.RawMessage(`{}`)}, "there is no capability mail.send"},
		{"not a web link", store.RoutineStep{Capability: "browser.open", Input: json.RawMessage(`{"urls":["ftp://example.com/file"]}`)},
			`"ftp://example.com/file" is not an http or https link`},
		{"no links", store.RoutineStep{Capability: "browser.open", Input: json.RawMessage(`{"urls":[]}`)}, "give 1 to 10 links"},
		{"a typo in the input", store.RoutineStep{Capability: "browser.open", Input: json.RawMessage(`{"url":["https://example.com"]}`)},
			"the input doesn't fit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRoutineFixture(t, nil)
			f.steps = []store.RoutineStep{tt.step, f.dsaSteps()[2]}

			run, steps, lines := f.run(t, routineSubject)

			require.Equal(t, store.RunFailed, run.Status)
			require.Equal(t, "1 of 2 steps failed", run.Reason)
			require.Empty(t, f.opener.urls)
			require.Len(t, steps, 2)
			require.Equal(t, store.StepFailed, steps[0].Status)
			require.Contains(t, steps[0].Error, tt.fails)
			require.Equal(t, store.StepDone, steps[1].Status, "the next step still runs")
			require.Equal(t, []string{"practice:"}, f.tracked)

			require.Len(t, lines, 5, "%+v", lines)
			require.Equal(t, "step_failed", lines[0].Kind)
			require.Equal(t, 0, lines[0].Step)
			require.Equal(t, steps[0].Error, lines[0].Data["error"])
			require.Equal(t, stepDone(1, "tracking.start", "Start tracking time", "Start tracking time"), lines[1:4])
			require.Equal(t, logLine{"failed", -1, map[string]string{"reason": "1 of 2 steps failed"}}, lines[4])
		})
	}
}

// A routine run that can't find its steps fails at once and says why, and
// opens nothing.
func TestRoutineRunWithoutSteps(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		steps   []store.RoutineStep
		reason  string
	}{
		{"no routine", routineSubject, nil, "there are no steps for when it starts"},
		{"an empty routine", routineSubject, []store.RoutineStep{}, "there are no steps for when it starts"},
		{"no subject", "", []store.RoutineStep{{Capability: "tracking.start", Input: json.RawMessage(`{"project":"practice"}`)}}, "a routine run needs a subject"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRoutineFixture(t, nil)
			f.steps = tt.steps

			run, steps, lines := f.run(t, tt.subject)

			require.Equal(t, store.RunFailed, run.Status)
			require.Equal(t, tt.reason, run.Reason)
			require.Empty(t, steps)
			require.Empty(t, f.opener.urls)
			require.Empty(t, f.opener.folders)
			require.Empty(t, f.tracked)
			require.NotEmpty(t, lines)
			require.Equal(t, logLine{"failed", -1, map[string]string{"reason": tt.reason}}, lines[len(lines)-1])
		})
	}
}
