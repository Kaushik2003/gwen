package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

const runID = "01926d2e-7a4b-7c3d-8e9f-00000000abab"

func breakdownRun(status string) *wire.LlmRun {
	out, _ := json.Marshal(wire.BreakdownOutput{Tasks: []wire.ProposedTask{
		{Title: "Outline", EstimateMinutes: 60, DueDay: "2026-09-20", Priority: 3},
		{Title: "Drills", EstimateMinutes: 90, DueDay: "2026-10-01", Priority: 2, Quantity: testutil.Ptr(5)},
	}})
	return &wire.LlmRun{ID: runID, Kind: wire.RunBreakdown, SubjectID: gID, Status: status, Output: out}
}

func dayPlanRun(status string) *wire.LlmRun {
	out, _ := json.Marshal(wire.DayPlanOutput{
		Messages: []wire.ChatMessage{{Role: "user", Text: "Start at 14:00"}, {Role: "assistant", Text: "Pointers from 14:00."}},
		Items: []wire.ProposedBlock{
			{TaskID: tID, Title: "Two pointers", StartAt: wire.Millis(testutil.At("14:00")), PlannedMinutes: 90},
			{TaskID: tID, Title: "Two pointers", StartAt: wire.Millis(testutil.At("15:40")), PlannedMinutes: 60},
		},
		Hours: &wire.DayHours{StartMinute: testutil.Ptr(14 * 60)},
	})
	return &wire.LlmRun{ID: runID, Kind: wire.RunDayPlan, SubjectID: testutil.Day0, Status: status, Output: out}
}

func TestLLMGolden(t *testing.T) {
	t.Parallel()
	retroOut, _ := json.Marshal(wire.RetroOutput{Markdown: "## Week of 2026-09-07\n\nQuiet.", GeneratedBy: "rules"})
	failedOut, _ := json.Marshal(wire.RunError{Error: "HTTP 503: Overloaded"})
	breakdownOut := "Run " + runID + " · breakdown · ok\n" +
		"0  P3  due 2026-09-20  1h          Outline\n" +
		"1  P2  due 2026-10-01  1h 30m  ×5  Drills\n" +
		"Accept with: gwen llm accept " + runID + " --pick 0,1\n"
	tests := []struct {
		name, method string
		args         []string
		result       any
		want         []any
		out          string
	}{
		{"goal breakdown", "GoalBreakdown", []string{"goal", "breakdown", "0000a0a0", "--instructions", "graphs first"},
			breakdownRun(wire.RunOK), []any{gID, wire.BreakdownRequest{Instructions: "graphs first"}}, breakdownOut},
		{"plan chat", "PlanChat", []string{"plan", "chat", "Start", "at", "14:00", "--run", runID},
			dayPlanRun(wire.RunOK), []any{wire.PlanChatRequest{Day: testutil.Day0, Message: "Start at 14:00", RunID: testutil.Ptr(runID)}},
			"Run " + runID + " · day_plan · ok\n\nPointers from 14:00.\n\n" +
				"0  2:00 pm  1h 30m  Two pointers\n" +
				"1  3:40 pm  1h      Two pointers\n" +
				"Sets the day's hours: from 14:00\n" +
				"Apply with: gwen llm accept " + runID + "\n" +
				"Reply with: gwen plan chat --day 2026-09-15 --run " + runID + " MESSAGE\n"},
		{"plan chat for a day", "PlanChat", []string{"plan", "chat", "--day", "2026-09-16", "plan tomorrow"},
			dayPlanRun(wire.RunOK), []any{wire.PlanChatRequest{Day: "2026-09-16", Message: "plan tomorrow"}}, ""},
		{"llm show failed", "GetLLMRun", []string{"llm", "show", runID},
			&wire.LlmRun{ID: runID, Kind: wire.RunBreakdown, Status: wire.RunFailed, Output: failedOut}, []any{runID},
			"Run " + runID + " · breakdown · failed\nFailed: HTTP 503: Overloaded\n"},
		{"llm accept", "AcceptLLMRun", []string{"llm", "accept", runID, "--pick", "1, 0"},
			&wire.TaskList{Tasks: []wire.Task{taskT}}, []any{runID, wire.AcceptRunRequest{Indexes: []int{1, 0}}},
			"0000cccc  open  P3  due 2026-10-01  Study  Two pointers  1h 5m\n"},
		{"llm reject", "RejectLLMRun", []string{"llm", "reject", runID}, breakdownRun(wire.RunRejected), []any{runID},
			"Run " + runID + " · breakdown · rejected\n" +
				"0  P3  due 2026-09-20  1h          Outline\n1  P2  due 2026-10-01  1h 30m  ×5  Drills\n"},
		{"retro defaults to last week", "Retro", []string{"retro"},
			&wire.LlmRun{ID: runID, Kind: wire.RunRetro, Status: wire.RunOK, Output: retroOut},
			[]any{wire.RetroRequest{WeekStart: "2026-09-07"}},
			"Run " + runID + " · retro · ok\n\n## Week of 2026-09-07\n\nQuiet.\n\n(Written from your numbers; the LLM did not answer.)\n"},
		{"retro for a week", "Retro", []string{"retro", "--week", "2026-08-31"},
			&wire.LlmRun{ID: runID, Kind: wire.RunRetro, Status: wire.RunOK, Output: retroOut},
			[]any{wire.RetroRequest{WeekStart: "2026-08-31"}}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := plannerFake().Returns(tc.method, tc.result, nil)
			out, errOut, code := runCLI(t, f, tc.args...)
			require.Equal(t, exitOK, code, errOut)
			require.Equal(t, tc.want, lastCall(t, f, tc.method))
			if tc.out != "" {
				require.Equal(t, tc.out, out)
			}
		})
	}
	f := plannerFake().Returns("GetLLMRun", dayPlanRun(wire.RunOK), nil).
		Returns("AcceptLLMRun", &wire.TaskList{Tasks: []wire.Task{taskT}}, nil)
	_, errOut, code := runCLI(t, f, "llm", "accept", runID)
	require.Equal(t, exitOK, code, errOut)
	require.Equal(t, []any{runID, wire.AcceptRunRequest{Indexes: []int{0, 1}}}, lastCall(t, f, "AcceptLLMRun"),
		"a day plan applies every block")
	_, _, code = runCLI(t, plannerFake().Returns("GetLLMRun", breakdownRun(wire.RunOK), nil), "llm", "accept", runID)
	require.Equal(t, exitUsage, code, "--pick is required for a breakdown")
	_, _, code = runCLI(t, plannerFake(), "llm", "accept", runID, "--pick", "a")
	require.Equal(t, exitUsage, code)
}

func TestSetupLLM(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults().Wire()
	f := clienttest.New().Returns("GetConfig", &cfg, nil).Returns("PatchConfig", &cfg, nil)
	r := setupTestEnv(t, f, strings.Join([]string{"openai_compatible", "llama3", "http://pi:11434/v1", "secret"}, "\n")+"\n")
	require.NoError(t, setupExec(t, r, "llm"))
	require.Equal(t, []any{wire.ConfigPatch{"llm": {"provider": "openai_compatible", "model": "llama3",
		"endpoint": "http://pi:11434/v1"}}}, f.CallsTo("PatchConfig")[0].Args)
	key, err := config.ReadCredentialLine(r.env.credDir, config.CredLLMAPIKey)
	require.NoError(t, err)
	require.Equal(t, "secret", key)

	f = clienttest.New().Returns("GetConfig", &cfg, nil).Returns("PatchConfig", &cfg, nil)
	r = setupTestEnv(t, f, "")
	require.NoError(t, setupExec(t, r, "llm", "--yes"))
	require.Equal(t, []any{wire.ConfigPatch{"llm": {"provider": "none"}}}, f.CallsTo("PatchConfig")[0].Args)
	_, err = config.ReadCredential(r.env.credDir, config.CredLLMAPIKey)
	require.Error(t, err, "no key without one given")
}

func TestSetupLLMClaudeCode(t *testing.T) {
	t.Parallel()
	const installed = "/home/u/.local/bin/claude"
	tests := []struct {
		name    string
		stdin   string
		command string // what the command resolves to and is checked as
		model   string
		status  setupOutput
		want    string
	}{
		{
			name: "signed in with a subscription", stdin: "claude_code\n\n\n", command: installed, model: "claude-sonnet-5",
			status: setupOutput{stdout: `{"loggedIn": true, "authMethod": "claude.ai", "subscriptionType": "pro"}`},
			want:   "Claude Code is signed in with your Claude pro subscription.",
		},
		{
			name: "signed out", stdin: "claude_code\n\n\n", command: installed, model: "claude-sonnet-5",
			status: setupOutput{stdout: `{"loggedIn": false, "authMethod": "none"}`, err: errors.New("exit status 1")},
			want:   "Claude Code is not signed in. Run claude auth login, then try again.",
		},
		{
			name: "a typed command that does not run", stdin: "claude_code\nclaude-opus-5-5\n/opt/claude/claude\n",
			command: "/opt/claude/claude", model: "claude-opus-5-5",
			status: setupOutput{err: errors.New("exec: permission denied")},
			want:   "Could not ask /opt/claude/claude whether Claude Code is signed in (exec: permission denied).",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Defaults().Wire()
			f := clienttest.New().Returns("GetConfig", &cfg, nil).Returns("PatchConfig", &cfg, nil)
			r := setupTestEnv(t, f, tc.stdin)
			r.paths = map[string]string{"claude": installed}
			check := tc.command + " auth status --json"
			r.outputs = map[string]setupOutput{check: tc.status}

			require.NoError(t, setupExec(t, r, "llm"))
			require.Equal(t, []any{wire.ConfigPatch{"llm": {"provider": "claude_code", "model": tc.model,
				"command": tc.command}}}, f.CallsTo("PatchConfig")[0].Args, "the config is saved either way")
			require.Contains(t, r.out.String(), "Claude Code command ["+installed+"]", "the default is the resolved path")
			require.Contains(t, r.out.String(), tc.want)
			require.Equal(t, []string{check}, r.ran())
			_, err := config.ReadCredential(r.env.credDir, config.CredLLMAPIKey)
			require.Error(t, err, "claude_code asks for no key")
		})
	}
}
