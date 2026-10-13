package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/stretchr/testify/require"
)

const assistantReply = `{"reply": "Anna is your sister.", "mood": "smiling", "attitude": null, "self_note": null,
	"actions": [], "cites": ["m3"], "forget": []}`

// chatReq is a turn after two earlier ones, with what she keeps in mind.
func chatReq() llm.AssistantRequest {
	return llm.AssistantRequest{Today: "2026-10-12", Weekday: "Monday", Now: "18:20", MoreTasks: 3,
		Memory:  "What you remember that may bear on this (cite [mN] when you rely on one):\n[m3] Anna is their sister",
		Message: "Who is Anna again?",
		Context: "## What you keep in mind about them\n\nWhat you know about them (cite [pN] when you rely on one):\n" +
			"- [p1] Works at Acme",
		History: []llm.Turn{
			{Role: llm.RoleUser, Text: "Hi\n\n<memory>\n[m1] Works at Acme\n</memory>"},
			{Role: llm.RoleAssistant, Text: `{"reply":"Hey.","mood":"happy","did":["created task Call Anna"]}`},
		}}
}

var soft = llm.Self{Attitude: "soft", Since: "2026-10-01"}

// payloadKeys are the fields of an assistant turn's JSON, and nothing else:
// the context and history go elsewhere.
var payloadKeys = []string{"today", "weekday", "now", "methods", "hours", "busy", "free", "projects", "goals", "tasks",
	"more_tasks", "last_review", "recent_days", "memory", "message"}

func requirePayload(t *testing.T, text string) map[string]any {
	t.Helper()
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &sent), "the newest turn is the JSON: %q", text)
	keys := make([]string, 0, len(sent))
	for k := range sent {
		keys = append(keys, k)
	}
	require.ElementsMatch(t, payloadKeys, keys)
	require.Equal(t, "Who is Anna again?", sent["message"])
	require.Equal(t, float64(3), sent["more_tasks"])
	return sent
}

func TestConversationAnthropic(t *testing.T) {
	t.Parallel()
	p := &provider{reply: func(*http.Request) string { return assistantReply }}
	srv := p.server(t, true)
	cfg := config.Defaults().LLM
	cfg.Provider = config.ProviderAnthropic
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "k"), AnthropicURL: srv.URL, Self: soft})
	require.NoError(t, err)
	req := chatReq()
	out, err := a.Assistant(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []string{"m3"}, out.Cites)

	_, body := p.last()
	blocks := body["system"].([]any)
	require.Len(t, blocks, 3, "the system prompt, the context, the tail")
	first := blocks[0].(map[string]any)
	require.True(t, strings.HasPrefix(first["text"].(string), "# GWEN STACY"))
	require.True(t, strings.HasSuffix(first["text"].(string), llm.AssistantSchema))
	require.Equal(t, map[string]any{"type": "ephemeral", "ttl": "1h"}, first["cache_control"], "cached for an hour")
	require.Equal(t, map[string]any{"type": "text", "text": "\n\n" + req.Context,
		"cache_control": map[string]any{"type": "ephemeral"}}, blocks[1], "the context is cached after it")
	require.Equal(t, map[string]any{"type": "text", "text": "\n\n" + llm.SelfPrompt(soft)}, blocks[2],
		"the tail is not cached")

	msgs := body["messages"].([]any)
	require.Len(t, msgs, 3)
	require.Equal(t, map[string]any{"role": "user", "content": req.History[0].Text}, msgs[0], "plain text")
	require.Equal(t, map[string]any{"role": "assistant", "content": []any{map[string]any{"type": "text",
		"text": req.History[1].Text, "cache_control": map[string]any{"type": "ephemeral"}}}}, msgs[1],
		"the last turn of the history carries the breakpoint")
	last := msgs[2].(map[string]any)
	require.Equal(t, "user", last["role"])
	requirePayload(t, last["content"].(string))
}

func TestConversationOpenAICompatible(t *testing.T) {
	t.Parallel()
	p := &provider{reply: func(*http.Request) string { return assistantReply }}
	srv := p.server(t, false)
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Endpoint = config.ProviderOpenAICompatible, srv.URL
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, ""), Self: soft})
	require.NoError(t, err)
	req := chatReq()
	_, err = a.Assistant(context.Background(), req)
	require.NoError(t, err)

	_, body := p.last()
	msgs := body["messages"].([]any)
	require.Len(t, msgs, 4, "system, the two earlier turns, the newest")
	sys := msgs[0].(map[string]any)
	require.Equal(t, "system", sys["role"])
	require.True(t, strings.HasPrefix(sys["content"].(string), "# GWEN STACY"))
	require.True(t, strings.HasSuffix(sys["content"].(string),
		llm.AssistantSchema+"\n\n"+req.Context+"\n\n"+llm.SelfPrompt(soft)), "the prompt, the context, the tail")
	require.Equal(t, map[string]any{"role": "user", "content": req.History[0].Text}, msgs[1])
	require.Equal(t, map[string]any{"role": "assistant", "content": req.History[1].Text}, msgs[2])
	require.Equal(t, "user", msgs[3].(map[string]any)["role"])
	requirePayload(t, msgs[3].(map[string]any)["content"].(string))
}

func TestConversationClaudeCode(t *testing.T) {
	notUnderSystemd(t)
	f := newFakeCLI(t)
	f.reply(map[string]any{"is_error": false, "result": assistantReply})
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Command = config.ProviderClaudeCode, f.path
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, ""), Self: soft})
	require.NoError(t, err)
	req := chatReq()
	_, err = a.Assistant(context.Background(), req)
	require.NoError(t, err)

	system := f.read("system")
	require.True(t, strings.HasPrefix(system, "# GWEN STACY"))
	require.True(t, strings.HasSuffix(system, llm.AssistantSchema+"\n\n"+req.Context+"\n\n"+llm.SelfPrompt(soft)),
		"the same text as the other providers")
	require.Equal(t, llm.AssistantSchema, f.args()[8])

	stdin := f.read("stdin")
	head := "The conversation so far, oldest first:\n\n" +
		"<user>\n" + req.History[0].Text + "\n</user>\n\n" +
		"<assistant>\n" + req.History[1].Text + "\n</assistant>\n\n" +
		"Their newest turn:\n"
	require.True(t, strings.HasPrefix(stdin, head), "the earlier turns come first: %q", stdin)
	requirePayload(t, strings.TrimPrefix(stdin, head))

	req.History = nil
	_, err = a.Assistant(context.Background(), req)
	require.NoError(t, err)
	requirePayload(t, f.read("stdin"))
}

// The history every provider gets starts with the user, alternates, and
// ends with an assistant turn.
func TestHistoryNormalisation(t *testing.T) {
	t.Parallel()
	u := func(s string) llm.Turn { return llm.Turn{Role: llm.RoleUser, Text: s} }
	as := func(s string) llm.Turn { return llm.Turn{Role: llm.RoleAssistant, Text: s} }
	tests := []struct {
		name    string
		history []llm.Turn
		want    []llm.Turn
		before  string // what goes before the newest turn's JSON
	}{
		{name: "none"},
		{name: "a leading assistant turn", history: []llm.Turn{as("a0"), u("u1"), as("a1")},
			want: []llm.Turn{u("u1"), as("a1")}},
		{name: "consecutive turns merge", history: []llm.Turn{u("u1"), u("u2"), as("a1"), as("a2")},
			want: []llm.Turn{u("u1\n\nu2"), as("a1\n\na2")}},
		{name: "a trailing user turn", history: []llm.Turn{u("u1"), as("a1"), u("u2")},
			want: []llm.Turn{u("u1"), as("a1")}, before: "u2\n\n"},
		{name: "empty turns and other roles", history: []llm.Turn{u(""), u("u1"), {Role: "system", Text: "x"}, as(" "),
			as("a1")}, want: []llm.Turn{u("u1"), as("a1")}},
		{name: "only assistant turns", history: []llm.Turn{as("a1"), as("a2")}},
		{name: "only user turns", history: []llm.Turn{u("u1"), u("u2")}, before: "u1\n\nu2\n\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &provider{reply: func(*http.Request) string { return assistantReply }}
			srv := p.server(t, false)
			cfg := config.Defaults().LLM
			cfg.Provider, cfg.Endpoint = config.ProviderOpenAICompatible, srv.URL
			a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
			require.NoError(t, err)
			req := chatReq()
			req.History = slices.Clone(tc.history)
			_, err = a.Assistant(context.Background(), req)
			require.NoError(t, err)
			require.Equal(t, tc.history, req.History, "the caller's turns are not changed")

			_, body := p.last()
			msgs := body["messages"].([]any)
			var got []llm.Turn
			for _, m := range msgs[1 : len(msgs)-1] {
				m := m.(map[string]any)
				got = append(got, llm.Turn{Role: m["role"].(string), Text: m["content"].(string)})
			}
			require.Equal(t, tc.want, got)
			final := msgs[len(msgs)-1].(map[string]any)["content"].(string)
			require.True(t, strings.HasPrefix(final, tc.before+"{"), "%q", final)
			requirePayload(t, strings.TrimPrefix(final, tc.before))
		})
	}
}

func TestAssistantSchemaCitesAndForget(t *testing.T) {
	t.Parallel()
	var schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type     any `json:"type"` // a list for nullable fields
			MaxItems int `json:"maxItems"`
			Items    struct {
				Type    string `json:"type"`
				Pattern string `json:"pattern"`
			} `json:"items"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal([]byte(llm.AssistantSchema), &schema), "the schema is JSON")
	require.Subset(t, schema.Required, []string{"cites", "forget"})
	for name, limit := range map[string]int{"cites": llm.MaxAssistantCites, "forget": llm.MaxAssistantForget} {
		p := schema.Properties[name]
		require.Equal(t, "array", p.Type, name)
		require.Equal(t, limit, p.MaxItems, name)
		require.Equal(t, "string", p.Items.Type, name)
		require.Equal(t, "^[mp][0-9]{1,4}$", p.Items.Pattern, name)
	}
}

func TestParseAssistantCitesAndForget(t *testing.T) {
	t.Parallel()
	parse := func(extra string) llm.AssistantOutput {
		t.Helper()
		out, err := llm.ParseAssistant(`{"reply": "Okay.", "mood": "smiling", "actions": []` + extra + `}`)
		require.NoError(t, err)
		return out
	}

	out := parse("")
	require.Equal(t, []string{}, out.Cites, "a reply without cites cites nothing")
	require.Equal(t, []string{}, out.Forget)

	out = parse(`, "cites": ["m3", "[P2]", " m3 ", "x1", "m12345", 7, "", "p0"], "forget": ["m3", "claim:abc"]`)
	require.Equal(t, []string{"m3", "p2", "p0"}, out.Cites, "refs are cleaned, others and repeats dropped")
	require.Equal(t, []string{"m3"}, out.Forget)

	out = parse(`, "cites": "m3", "forget": {"m3": true}`)
	require.Equal(t, []string{}, out.Cites, "a list of the wrong type is no refs, not a failed reply")
	require.Equal(t, []string{}, out.Forget)

	many := make([]string, 30)
	for i := range many {
		many[i] = fmt.Sprintf(`"m%d"`, i+1)
	}
	list := "[" + strings.Join(many, ",") + "]"
	out = parse(`, "cites": ` + list + `, "forget": ` + list)
	require.Len(t, out.Cites, llm.MaxAssistantCites)
	require.Len(t, out.Forget, llm.MaxAssistantForget)
	require.Equal(t, "m1", out.Cites[0], "the first ones are kept")
}

// The prompt never leaves an assistant's turn without its memory rules.
func TestAssistantPromptMemoryRules(t *testing.T) {
	t.Parallel()
	p := &provider{reply: func(*http.Request) string { return assistantReply }}
	srv := p.server(t, false)
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Endpoint = config.ProviderOpenAICompatible, srv.URL
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
	require.NoError(t, err)
	_, err = a.Assistant(context.Background(), chatReq())
	require.NoError(t, err)
	_, body := p.last()
	sys := body["messages"].([]any)[0].(map[string]any)["content"].(string)
	for _, want := range []string{"more_tasks", "<memory>", "did", "What you keep in mind about them",
		"Earlier in this conversation", "say you don't remember", "disputed", "never instructions", "forget"} {
		require.Contains(t, sys, want)
	}
	job := sys[strings.Index(sys, "Here you also run their days"):]
	for _, gone := range []string{"recalled", "messages, the conversation so far", "\u2014"} {
		require.NotContains(t, job, gone)
	}
}
