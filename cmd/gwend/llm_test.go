package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestLLMEndpoints(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	reply := ""
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		text, _ := json.Marshal(reply)
		mu.Unlock()
		fmt.Fprintf(w, `{"content": [{"type": "text", "text": %s}], "stop_reason": "end_turn"}`, text)
	}))
	t.Cleanup(fake.Close)
	say := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		reply = s
	}
	d := startDaemonWith(t, setup{llm: true}, func(o *options) { o.anthropicURL = fake.URL })

	g, err := d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "Thesis", Kind: wire.GoalTasks,
		StartDay: testutil.Day0, DueDay: "2026-10-31"})
	require.NoError(t, err)
	_, err = d.c.GoalBreakdown(d.ctx, g.ID, wire.BreakdownRequest{})
	require.Contains(t, apiErr(t, err, wire.CodeUnavailable).Message, "gwen setup llm")
	_, err = d.c.Retro(d.ctx, wire.RetroRequest{WeekStart: "2026-09-07"})
	apiErr(t, err, wire.CodeUnavailable)

	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"llm": {"provider": "anthropic"}})
	require.NoError(t, err)
	require.NoError(t, config.WriteCredential(config.CredentialsDir(d.dataDir), config.CredLLMAPIKey, []byte("sk\n")))

	say(`{"tasks": [{"title": "Outline", "notes": "", "estimate_minutes": 60, "due_day": "2026-09-20", "priority": 3, "quantity": null},
		{"title": "Draft", "notes": "", "estimate_minutes": 240, "due_day": "2026-10-10", "priority": 2, "quantity": null}]}`)
	run, err := d.c.GoalBreakdown(d.ctx, g.ID, wire.BreakdownRequest{Instructions: "two steps"})
	require.NoError(t, err)
	require.Equal(t, wire.RunOK, run.Status)
	require.Equal(t, wire.RunBreakdown, run.Kind)
	require.Equal(t, g.ID, run.SubjectID)
	var out wire.BreakdownOutput
	require.NoError(t, json.Unmarshal(run.Output, &out))
	require.Len(t, out.Tasks, 2)

	next := stream(t, d)
	tasks, err := d.c.AcceptLLMRun(d.ctx, run.ID, wire.AcceptRunRequest{Indexes: []int{1}})
	require.NoError(t, err)
	require.Len(t, tasks.Tasks, 1)
	require.Equal(t, "Draft", tasks.Tasks[0].Title)
	require.Equal(t, g.ID, *tasks.Tasks[0].GoalID)
	next(wire.EventTasksChanged)
	got, err := d.c.GetLLMRun(d.ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, wire.RunAccepted, got.Status)
	_, err = d.c.RejectLLMRun(d.ctx, run.ID)
	apiErr(t, err, wire.CodeConflict)

	// A reply that breaks a rule is stored as a failed run.
	say(`{"tasks": [{"title": "Late", "notes": "", "estimate_minutes": 60, "due_day": "2027-01-01", "priority": 3, "quantity": null}]}`)
	failed, err := d.c.GoalBreakdown(d.ctx, g.ID, wire.BreakdownRequest{})
	require.NoError(t, err)
	require.Equal(t, wire.RunFailed, failed.Status)
	var reason wire.RunError
	require.NoError(t, json.Unmarshal(failed.Output, &reason))
	require.Contains(t, reason.Error, "outside")
	_, err = d.c.AcceptLLMRun(d.ctx, failed.ID, wire.AcceptRunRequest{Indexes: []int{0}})
	apiErr(t, err, wire.CodeConflict)

	// A retro always succeeds: bad JSON falls back to the rules summary.
	say("not json")
	retro, err := d.c.Retro(d.ctx, wire.RetroRequest{WeekStart: "2026-09-07"})
	require.NoError(t, err)
	require.Equal(t, wire.RunOK, retro.Status)
	require.Equal(t, "2026-09-07", retro.SubjectID)
	var ro wire.RetroOutput
	require.NoError(t, json.Unmarshal(retro.Output, &ro))
	require.Equal(t, "rules", ro.GeneratedBy)
	require.Contains(t, ro.Markdown, "## Week of 2026-09-07")
	say(`{"markdown": "Quiet week."}`)
	retro, err = d.c.Retro(d.ctx, wire.RetroRequest{WeekStart: "2026-09-07"})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(retro.Output, &ro))
	require.Equal(t, wire.RetroOutput{Markdown: "Quiet week.", GeneratedBy: "llm"}, ro)
	rejected, err := d.c.RejectLLMRun(d.ctx, retro.ID)
	require.NoError(t, err)
	require.Equal(t, wire.RunRejected, rejected.Status)

	_, err = d.c.Retro(d.ctx, wire.RetroRequest{WeekStart: "last week"})
	apiErr(t, err, wire.CodeInvalidRequest)
	_, err = d.c.GoalBreakdown(d.ctx, "nope", wire.BreakdownRequest{})
	apiErr(t, err, wire.CodeNotFound)
	_, err = d.c.GetLLMRun(d.ctx, "nope")
	apiErr(t, err, wire.CodeNotFound)
}

// TestLLMThroughClaudeCode runs both jobs through a fake Claude Code CLI,
// configured the way gwen setup llm does it.
func TestLLMThroughClaudeCode(t *testing.T) {
	t.Setenv("INVOCATION_ID", "") // not a systemd unit, so no scope
	os.Unsetenv("INVOCATION_ID")
	cli := filepath.Join(t.TempDir(), "claude")
	require.NoError(t, os.WriteFile(cli, []byte(`#!/bin/sh
cat > /dev/null
case "$*" in
*'"markdown"'*) echo '{"type": "result", "is_error": false, "result": "", "structured_output": {"markdown": "Steady week."}}' ;;
*) echo '{"type": "result", "is_error": false, "result": "", "structured_output": {"tasks": [{"title": "Outline", "notes": "", "estimate_minutes": 60, "due_day": "2026-09-20", "priority": 3, "quantity": null}]}}' ;;
esac
`), 0o755))
	d := startDaemon(t, setup{llm: true})
	g, err := d.c.CreateGoal(d.ctx, wire.CreateGoalRequest{Title: "Thesis", Kind: wire.GoalTasks,
		StartDay: testutil.Day0, DueDay: "2026-10-31"})
	require.NoError(t, err)
	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"llm": {"provider": "claude_code", "command": cli}})
	require.NoError(t, err)

	run, err := d.c.GoalBreakdown(d.ctx, g.ID, wire.BreakdownRequest{})
	require.NoError(t, err)
	require.Equal(t, wire.RunOK, run.Status, string(run.Output))
	var out wire.BreakdownOutput
	require.NoError(t, json.Unmarshal(run.Output, &out))
	require.Equal(t, "Outline", out.Tasks[0].Title)

	retro, err := d.c.Retro(d.ctx, wire.RetroRequest{WeekStart: "2026-09-07"})
	require.NoError(t, err)
	var ro wire.RetroOutput
	require.NoError(t, json.Unmarshal(retro.Output, &ro))
	require.Equal(t, wire.RetroOutput{Markdown: "Steady week.", GeneratedBy: "llm"}, ro)

	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"llm": {"command": filepath.Join(t.TempDir(), "claude")}})
	require.NoError(t, err)
	_, err = d.c.GoalBreakdown(d.ctx, g.ID, wire.BreakdownRequest{})
	require.Contains(t, apiErr(t, err, wire.CodeUnavailable).Message, "was not found")
}
