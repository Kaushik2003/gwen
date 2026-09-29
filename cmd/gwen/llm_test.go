package main

import (
	"encoding/json"
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
	_, _, code := runCLI(t, plannerFake(), "llm", "accept", runID)
	require.Equal(t, exitUsage, code, "--pick is required")
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
