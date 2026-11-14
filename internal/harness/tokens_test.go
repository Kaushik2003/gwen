package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/connect"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
)

var tokenPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/-]+=*`),
	regexp.MustCompile(`(?i)\bAuthorization["']?\s*[:=]\s*["']?[^\s"',}]+(?:[ \t]+[^\s"',}]+)?`),
	regexp.MustCompile(`\bya29\.[A-Za-z0-9._-]+`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
	regexp.MustCompile(`\bgh[pos]_[A-Za-z0-9_]+`),
	regexp.MustCompile(`\bxox[abp]-[A-Za-z0-9-]+`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+`),
	regexp.MustCompile(`\bAIza[A-Za-z0-9_-]+`),
	regexp.MustCompile(`(?i)\b(?:access_token|refresh_token|api_key)["']?\s*[:=]\s*["']?[^\s"'&,}]+`),
}

var genericToken = regexp.MustCompile(`[A-Za-z0-9+/_-]{32,}`)
var tokenLetter = regexp.MustCompile(`[A-Za-z]`)
var tokenDigit = regexp.MustCompile(`[0-9]`)

func findTokens(text string) []string {
	var found []string
	for _, pattern := range tokenPatterns {
		found = append(found, pattern.FindAllString(text, -1)...)
	}
	for _, candidate := range genericToken.FindAllString(text, -1) {
		if tokenLetter.MatchString(candidate) && tokenDigit.MatchString(candidate) {
			found = append(found, candidate)
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// Only values read from generated store fields are exempt, never ID/time shapes.
func withoutStoreValues(text string, values []string) string {
	values = slices.Clone(values)
	slices.SortFunc(values, func(a, b string) int { return len(b) - len(a) })
	for _, value := range values {
		if value != "" {
			text = strings.ReplaceAll(text, value, " ")
		}
	}
	return text
}

func TestFindTokensMatcher(t *testing.T) {
	tests := []struct{ name, text string }{
		{"bearer", "Bearer abcdefghijklmnop1234"},
		{"authorization", "Authorization: Basic YWJjOjEyMw=="},
		{"authorization JSON", `{"Authorization":"Bearer short123"}`},
		{"google oauth", "ya29.a0AfH6SMA-example"},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl"},
		{"github personal", "ghp_abcdef123456"},
		{"github oauth", "gho_abcdef123456"},
		{"github server", "ghs_abcdef123456"},
		{"slack app", "xoxa-1234-abcd"},
		{"slack bot", "xoxb-1234-abcd"},
		{"slack user", "xoxp-1234-abcd"},
		{"api secret", "sk-proj-abcdef123456"},
		{"google api", "AIzaSyAbc123456"},
		{"access assignment", "access_token=short123"},
		{"refresh assignment", "refresh_token=short123"},
		{"api assignment", "api_key=short123"},
		{"query", "https://example.com/a?x=1&access_token=short123&y=2"},
		{"JSON", `{"refresh_token":"short123","api_key":"short456"}`},
		{"generic exactly 32", "abcdefghijklmnopqrstuvwxyz123456"},
		{"generic alphabet", "ABcd12+/_-ABcd12+/_-ABcd12+/_-ABcd12+/_-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := findTokens(tt.text); len(got) == 0 {
				t.Fatalf("missed planted token in %q", tt.text)
			}
		})
	}
	for _, text := range []string{
		"https://leetcode.com/problems/two-sum/", "https://example.com/a/b?x=1",
		"/home/user/projects/gwen/main.go", "~/notes/today.txt", "Open https://example.com",
		strings.Repeat("a", 40), strings.Repeat("1", 40), "abcdefghijklmnopqrstuvwxyz12345",
	} {
		t.Run(text, func(t *testing.T) {
			if got := findTokens(text); len(got) != 0 {
				t.Fatalf("ordinary text flagged: %q", got)
			}
		})
	}
	t.Run("store identifiers and times by value", func(t *testing.T) {
		repo := store.NewRunRepo(testutil.NewDB(t))
		r, err := repo.CreateRun(context.Background(), store.Run{Kind: KindRoutine, Title: "Example"})
		if err != nil {
			t.Fatal(err)
		}
		// The strict generic rule must still catch a UUID until its exact value is removed.
		if len(findTokens(r.ID)) == 0 {
			t.Fatal("generic matcher did not flag raw store ID")
		}
		text := r.ID + " " + r.CreatedAt.Format(time.RFC3339Nano)
		if got := findTokens(withoutStoreValues(text, []string{r.ID, r.CreatedAt.Format(time.RFC3339Nano)})); len(got) != 0 {
			t.Fatal(got)
		}
		if got := findTokens(collect(t, repo, &tokenPublished{})); len(got) != 0 {
			t.Fatal(got)
		}
	})
}

type tokenPublished struct {
	mu        sync.Mutex
	lines     []string
	err       error
	requested chan json.RawMessage
}

func (p *tokenPublished) publish(name string, payload any) {
	raw, err := json.Marshal(payload)
	p.mu.Lock()
	p.lines = append(p.lines, name+" "+string(raw))
	if err != nil {
		p.err = err
	}
	p.mu.Unlock()
	if name == wire.EventApprovalRequested && p.requested != nil {
		p.requested <- raw
	}
}

// A fresh scenario DB contains only that scenario's runs, approvals and grants.
// Limits are the largest the repository accepts; fail instead of silently truncating.
func collect(t *testing.T, repo store.RunRepo, published *tokenPublished) string {
	t.Helper()
	ctx := context.Background()
	var text strings.Builder
	var generated []string
	add := func(label string, value any) {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&text, "%s %s\n", label, raw)
	}
	times := func(values ...*time.Time) {
		for _, value := range values {
			if value != nil {
				generated = append(generated, value.Format(time.RFC3339), value.Format(time.RFC3339Nano), value.String())
			}
		}
	}
	runs, err := repo.Runs(ctx, nil, 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) == 500 {
		t.Fatal("collector run limit reached")
	}
	for _, listed := range runs {
		r, err := repo.Run(ctx, listed.ID)
		if err != nil {
			t.Fatal(err)
		}
		add("run", r)
		generated = append(generated, r.ID)
		times(&r.CreatedAt, &r.UpdatedAt, r.StartedAt, r.EndedAt)
		steps, err := repo.Steps(ctx, r.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range steps {
			add("step", step)
			generated = append(generated, step.ID)
			times(step.StartedAt, step.EndedAt)
		}
		events, err := repo.Events(ctx, r.ID, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 1000 {
			t.Fatal("collector event limit reached")
		}
		for _, event := range events {
			add("event", event)
			// Events have integer sequence numbers rather than UUIDs.
			generated = append(generated, fmt.Sprintf(`"Seq":%d,`, event.Seq))
			times(&event.At)
		}
	}
	approvals, err := repo.Approvals(ctx, "", 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(approvals) == 500 {
		t.Fatal("collector approval limit reached")
	}
	for _, approval := range approvals {
		add("approval", approval)
		generated = append(generated, approval.ID)
		times(&approval.CreatedAt, approval.DecidedAt)
	}
	grants, err := repo.Grants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, grant := range grants {
		add("grant", grant)
		generated = append(generated, grant.ID)
		times(&grant.CreatedAt, grant.ExpiresAt)
	}
	published.mu.Lock()
	defer published.mu.Unlock()
	if published.err != nil {
		t.Fatal(published.err)
	}
	for _, line := range published.lines {
		text.WriteString(line)
		text.WriteByte('\n')
	}
	return withoutStoreValues(text.String(), generated)
}

func TestCollectorSeesEvents(t *testing.T) {
	repo := store.NewRunRepo(testutil.NewDB(t))
	ctx := context.Background()
	run, err := repo.CreateRun(ctx, store.Run{Kind: KindRoutine, Title: "Collector coverage"})
	if err != nil {
		t.Fatal(err)
	}
	const planted = "Bearer abcdefghijklmnop1234"
	if _, err := repo.AddEvent(ctx, run.ID, "", "note", map[string]string{"note": planted}); err != nil {
		t.Fatal(err)
	}
	if got := findTokens(collect(t, repo, &tokenPublished{})); !slices.Contains(got, planted) {
		t.Fatalf("collector missed stored event: %q", got)
	}
}

type tokenOpener struct {
	open    func(context.Context, string) error
	urls    []string
	folders []string
}

func (o *tokenOpener) OpenURL(ctx context.Context, url string) error {
	o.urls = append(o.urls, url)
	if o.open != nil {
		return o.open(ctx, url)
	}
	return nil
}
func (o *tokenOpener) OpenFolder(_ context.Context, path string) error {
	o.folders = append(o.folders, path)
	return nil
}

type tokenWriteConnector struct{ calls int }

func (*tokenWriteConnector) ID() string   { return "test_write" }
func (*tokenWriteConnector) Name() string { return "Test writer" }
func (*tokenWriteConnector) Status(context.Context) connect.Status {
	return connect.Status{State: connect.Connected}
}
func (c *tokenWriteConnector) Capabilities() []connect.Capability {
	return []connect.Capability{{
		Name: "draft.write", Title: "Write draft", Risk: connect.Write,
		Check: connect.Checked[struct {
			Text string `json:"text"`
		}](nil),
		Summary: func(json.RawMessage) string { return "Write a local draft" },
		Scope:   func(json.RawMessage) string { return "draft-folder" },
		Invoke: func(context.Context, json.RawMessage) (json.RawMessage, error) {
			c.calls++
			return json.RawMessage(`{"written":true}`), nil
		},
	}}
}

type tokenFixture struct {
	repo      store.RunRepo
	engine    *Engine
	registry  *connect.Registry
	clk       *clock.Fake
	published *tokenPublished
	opener    *tokenOpener
	home      string
	tracked   []string
}

func newTokenFixture(t *testing.T, open func(context.Context, string) error, steps []store.RoutineStep) *tokenFixture {
	t.Helper()
	// Keep a short real folder path, so the generic matcher can inspect it unchanged.
	home, err := os.MkdirTemp("", "tok-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(home); err != nil {
			t.Error(err)
		}
	})
	f := &tokenFixture{repo: store.NewRunRepo(testutil.NewDB(t)), clk: clock.NewFake(testutil.T0), home: home,
		published: &tokenPublished{requested: make(chan json.RawMessage, 8)}, opener: &tokenOpener{open: open}}
	f.registry = connect.NewRegistry(connect.Desktop(f.opener, func(_ context.Context, project, task string) (string, error) {
		f.tracked = append(f.tracked, project+":"+task)
		return "active", nil
	}, home))
	if steps == nil {
		folder, err := json.Marshal(map[string]string{"path": home})
		if err != nil {
			t.Fatal(err)
		}
		steps = []store.RoutineStep{
			{Capability: "browser.open", Input: json.RawMessage(`{"urls":["https://leetcode.com/problems/two-sum/","https://example.com/a/b?x=1"]}`)},
			{Capability: "folder.open", Input: folder},
			{Capability: "tracking.start", Input: json.RawMessage(`{"project":"practice"}`)},
		}
	}
	f.engine = New(Options{Runs: f.repo, Registry: f.registry, Clock: f.clk, Publish: f.published.publish}, Routine(f.registry, func(context.Context, string) (store.Routine, bool, error) {
		return store.Routine{Steps: steps}, true, nil
	}))
	t.Cleanup(f.engine.Close)
	return f
}

func (f *tokenFixture) start(t *testing.T) store.Run {
	t.Helper()
	ctx := context.Background()
	r, err := f.engine.Create(ctx, KindRoutine, "event:local/practice", "Practice routine", "Open the workspace", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.engine.Start(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *tokenFixture) wait(t *testing.T, id, status string) []store.RunStep {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.engine.Wait(ctx, id)
	if ctx.Err() != nil {
		t.Fatal("engine did not stop:", ctx.Err())
	}
	r, err := f.repo.Run(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != status {
		t.Fatalf("run = %+v, want status %s", r, status)
	}
	steps, err := f.repo.Steps(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

func TestRunOutputTokensRoutine(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "completion"
		if fail {
			name = "partial open failure"
		}
		t.Run(name, func(t *testing.T) {
			const openError = "exec: xdg-open: executable file not found in $PATH"
			f := newTokenFixture(t, func(_ context.Context, url string) error {
				if fail && strings.Contains(url, "leetcode.com") {
					return errors.New(openError)
				}
				return nil
			}, nil)
			r := f.start(t)
			status := store.RunDone
			if fail {
				status = store.RunFailed
			}
			steps := f.wait(t, r.ID, status)
			if len(steps) != 3 || len(f.opener.urls) != 2 || len(f.opener.folders) != 1 || !slices.Equal(f.tracked, []string{"practice:"}) {
				t.Fatalf("routine did not execute all calls: steps=%+v urls=%v folders=%v tracked=%v", steps, f.opener.urls, f.opener.folders, f.tracked)
			}
			for i, step := range steps {
				want := store.StepDone
				if fail && i == 0 {
					want = store.StepFailed
				}
				if step.Status != want {
					t.Fatalf("step %d = %+v", i, step)
				}
			}
			if fail && (!strings.Contains(string(steps[0].Output), openError) || steps[0].Error == "") {
				t.Fatalf("partial failure missing: %+v", steps[0])
			}
			if got := findTokens(collect(t, f.repo, f.published)); len(got) != 0 {
				t.Fatalf("token-like strings in run output: %q", got)
			}
		})
	}
}

func TestRunOutputTokensCancellationAndRecovery(t *testing.T) {
	for _, recoverRun := range []bool{false, true} {
		name := "cancel running step"
		if recoverRun {
			name = "restart recovery"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			f := newTokenFixture(t, func(ctx context.Context, _ string) error {
				close(entered)
				<-ctx.Done()
				return ctx.Err()
			}, []store.RoutineStep{{Capability: "browser.open", Input: json.RawMessage(`{"urls":["https://example.com"]}`)}})
			r := f.start(t)
			select {
			case <-entered:
			case <-time.After(10 * time.Second):
				t.Fatal("opener did not start")
			}
			if recoverRun {
				f.engine.Close()
				f.wait(t, r.ID, store.RunRunning)
				second := New(Options{Runs: f.repo, Registry: f.registry, Clock: f.clk, Publish: f.published.publish}, Routine(f.registry, nil))
				t.Cleanup(second.Close)
				if err := second.Recover(context.Background()); err != nil {
					t.Fatal(err)
				}
				row, err := f.repo.Run(context.Background(), r.ID)
				if err != nil {
					t.Fatal(err)
				}
				if row.Status != store.RunWaiting || row.Reason != ReasonInterrupted {
					t.Fatalf("recovery = %+v", row)
				}
				steps, err := f.repo.Steps(context.Background(), r.ID)
				if err != nil {
					t.Fatal(err)
				}
				if len(steps) != 1 || steps[0].Status != store.StepWaiting {
					t.Fatalf("recovered steps = %+v", steps)
				}
			} else {
				if _, err := f.engine.Cancel(context.Background(), r.ID); err != nil {
					t.Fatal(err)
				}
				steps := f.wait(t, r.ID, store.RunCancelled)
				if len(steps) != 1 || steps[0].Error != "cancelled" {
					t.Fatalf("cancelled steps = %+v", steps)
				}
			}
			if got := findTokens(collect(t, f.repo, f.published)); len(got) != 0 {
				t.Fatalf("token-like strings in run output: %q", got)
			}
		})
	}
}

func TestRunOutputTokensApproval(t *testing.T) {
	for _, approve := range []bool{true, false} {
		name := "approved and remembered"
		if !approve {
			name = "denied"
		}
		t.Run(name, func(t *testing.T) {
			f := newTokenFixture(t, nil, []store.RoutineStep{{Capability: "draft.write", Input: json.RawMessage(`{"text":"Practice notes"}`)}})
			writer := &tokenWriteConnector{}
			f.registry.Add(writer)
			r := f.start(t)
			var raw json.RawMessage
			select {
			case raw = <-f.published.requested:
			case <-time.After(10 * time.Second):
				t.Fatal("approval was not published")
			}
			var request wire.Approval
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			row, err := f.repo.Run(context.Background(), r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if row.Status != store.RunWaiting || row.Reason != ReasonApproval {
				t.Fatalf("approval did not wait: %+v", row)
			}
			remember := ""
			if approve {
				remember = "draft-folder"
			}
			decided, err := f.engine.Decide(context.Background(), request.ID, approve, nil, remember)
			if err != nil {
				t.Fatal(err)
			}
			wantApproval, wantStep, wantCalls := store.ApprovalRejected, store.StepSkipped, 0
			if approve {
				wantApproval, wantStep, wantCalls = store.ApprovalApproved, store.StepDone, 1
			}
			if decided.Status != wantApproval {
				t.Fatalf("approval = %+v", decided)
			}
			steps := f.wait(t, r.ID, store.RunDone)
			if len(steps) != 1 || steps[0].Status != wantStep || writer.calls != wantCalls {
				t.Fatalf("steps=%+v calls=%d", steps, writer.calls)
			}
			grants, err := f.repo.Grants(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if approve {
				if len(grants) != 1 || grants[0].Scope != remember || grants[0].Decision != string(connect.Allow) {
					t.Fatalf("remembered grants=%+v", grants)
				}
			} else if len(grants) != 0 {
				t.Fatalf("denial created grants: %+v", grants)
			}
			if got := findTokens(collect(t, f.repo, f.published)); len(got) != 0 {
				t.Fatalf("token-like strings in run output: %q", got)
			}
		})
	}
}
