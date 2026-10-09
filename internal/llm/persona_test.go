package llm_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/stretchr/testify/require"
)

func TestPersonaPrompt(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults().LLM
	fresh := llm.PersonaPrompt(cfg, llm.Self{})
	require.True(t, strings.HasPrefix(fresh, "# GWEN STACY"), "one personality, always")
	require.Contains(t, fresh, "The user calls you Gwen.")
	require.NotContains(t, fresh, "How you have chosen to treat them", "nothing chosen yet")

	cfg.AssistantName, cfg.Instructions = "Rex", "Call me boss."
	got := llm.PersonaPrompt(cfg, llm.Self{Attitude: "angry", Note: "Third excuse about the gym.", Since: "2026-09-14"})
	require.Contains(t, got, "The user calls you Rex.")
	require.Contains(t, got, "on 2026-09-14")
	require.Contains(t, got, "you are being angry (fed up with excuses")
	require.Contains(t, got, "Third excuse about the gym.")
	require.Contains(t, got, "Call me boss.")

	noteOnly := llm.PersonaPrompt(cfg, llm.Self{Note: "He likes teasing."})
	require.Contains(t, noteOnly, "He likes teasing.")
	require.NotContains(t, noteOnly, "you are being")
}

func TestParseAssistantSelf(t *testing.T) {
	t.Parallel()
	parse := func(attitude, note any) llm.AssistantOutput {
		t.Helper()
		b, err := json.Marshal(map[string]any{"reply": "Hm.", "mood": "annoyed", "attitude": attitude, "self_note": note,
			"actions": []any{}})
		require.NoError(t, err)
		out, err := llm.ParseAssistant(string(b))
		require.NoError(t, err)
		return out
	}

	out := parse(" Angry ", "  Waiting for him to finish the report.  ")
	require.Equal(t, "angry", *out.Attitude)
	require.Equal(t, "Waiting for him to finish the report.", *out.SelfNote)

	out = parse(nil, nil)
	require.Nil(t, out.Attitude, "null keeps the attitude")
	require.Nil(t, out.SelfNote, "null keeps the note")

	out = parse("murderous", "  ")
	require.Nil(t, out.Attitude, "an unknown attitude is dropped, not the reply")
	require.Nil(t, out.SelfNote, "an empty note keeps the one she had")
	require.Equal(t, "Hm.", out.Reply)

	out = parse("soft", strings.Repeat("é", llm.MaxSelfNote+50))
	require.Len(t, []rune(*out.SelfNote), llm.MaxSelfNote, "a long note is cut, not refused")

	out, err := llm.ParseAssistant(`{"reply": "Hi.", "actions": []}`)
	require.NoError(t, err, "replies from before attitudes still read")
	require.Nil(t, out.Attitude)
}

func TestAssistantSchemaNamesAttitudes(t *testing.T) {
	t.Parallel()
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Enum []any `json:"enum"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal([]byte(llm.AssistantSchema), &schema))
	require.Contains(t, schema.Required, "attitude")
	require.Contains(t, schema.Required, "self_note")
	enum := schema.Properties["attitude"].Enum
	require.Len(t, enum, len(llm.Attitudes)+1, "every attitude, and null")
	for _, a := range llm.Attitudes {
		require.Contains(t, enum, a.ID)
	}
	require.Contains(t, enum, nil)
}
