package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/stretchr/testify/require"
)

// provider is a fake LLM endpoint that records requests and replies with text.
type provider struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   []map[string]any
	status   int
	reply    func(r *http.Request) string // the model's text
	delay    time.Duration
}

func (p *provider) server(t *testing.T, anthropic bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		json.Unmarshal(b, &body)
		p.mu.Lock()
		p.requests, p.bodies = append(p.requests, r), append(p.bodies, body)
		status, reply, delay := p.status, p.reply, p.delay
		p.mu.Unlock()
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		if status != 0 {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"type": "error", "error": {"type": "overloaded_error", "message": "Overloaded"}}`)
			return
		}
		text, _ := json.Marshal(reply(r))
		if anthropic {
			fmt.Fprintf(w, `{"type": "message", "role": "assistant", "content": [{"type": "thinking", "thinking": ""}, {"type": "text", "text": %s}], "stop_reason": "end_turn", "usage": {"input_tokens": 50, "cache_creation_input_tokens": 0, "cache_read_input_tokens": 3000, "output_tokens": 120}}`, text)
		} else {
			fmt.Fprintf(w, `{"choices": [{"message": {"role": "assistant", "content": %s}, "finish_reason": "stop"}], "usage": {"prompt_tokens": 3050, "completion_tokens": 120, "prompt_tokens_details": {"cached_tokens": 3000}}}`, text)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// system is the text of each block of an Anthropic request's system prompt.
func system(t *testing.T, body map[string]any) []string {
	t.Helper()
	blocks, ok := body["system"].([]any)
	require.True(t, ok, "the system prompt is a list of blocks")
	var out []string
	for _, b := range blocks {
		out = append(out, b.(map[string]any)["text"].(string))
	}
	return out
}

func (p *provider) last() (*http.Request, map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests[len(p.requests)-1], p.bodies[len(p.bodies)-1]
}

func withKey(t *testing.T, key string) string {
	t.Helper()
	dir := t.TempDir()
	if key != "" {
		require.NoError(t, config.WriteCredential(dir, config.CredLLMAPIKey, []byte(key+"\n")))
	}
	return dir
}

func goalReq() llm.BreakdownRequest {
	return llm.BreakdownRequest{Title: "300 problems", Kind: "quantity", Unit: "problems", TargetQuantity: ptr(300),
		MinutesPerUnit: ptr(30), StartDay: "2026-09-15", DueDay: "2026-12-15", ExistingTasks: []string{"Arrays"},
		Instructions: "focus on graphs", Today: "2026-09-20"}
}

func ptr[T any](v T) *T { return &v }

const goodBreakdown = `{"tasks": [{"title": "Graphs: BFS set", "notes": "", "estimate_minutes": 90, "due_day": "2026-09-27", "priority": 3, "quantity": 5}, {"title": "Review", "estimate_minutes": 30, "due_day": "2026-12-15", "priority": 1, "quantity": null}]}`

func TestAnthropicRequestShape(t *testing.T) {
	t.Parallel()
	p := &provider{reply: func(*http.Request) string { return "```json\n" + goodBreakdown + "\n```" }}
	srv := p.server(t, true)
	cfg := config.Defaults().LLM
	cfg.Provider = config.ProviderAnthropic
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "sk-test"), AnthropicURL: srv.URL + "/v1/messages"})
	require.NoError(t, err)
	require.Equal(t, "anthropic", a.Name())

	out, err := a.Breakdown(context.Background(), goalReq())
	require.NoError(t, err)
	require.Len(t, out.Tasks, 2)
	require.Equal(t, 5, *out.Tasks[0].Quantity)
	require.Nil(t, out.Tasks[1].Quantity)

	r, body := p.last()
	require.Equal(t, "/v1/messages", r.URL.Path)
	require.Equal(t, "sk-test", r.Header.Get("x-api-key"))
	require.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
	require.Empty(t, r.Header.Get("Authorization"))
	require.Equal(t, "claude-sonnet-5", body["model"])
	require.Equal(t, float64(16000), body["max_tokens"])
	require.Equal(t, []any{map[string]any{"type": "text", "text": system(t, body)[0],
		"cache_control": map[string]any{"type": "ephemeral"}}}, body["system"], "one cached block")
	require.Contains(t, system(t, body)[0], llm.BreakdownSchema, "the schema is sent verbatim")
	msgs := body["messages"].([]any)
	require.Len(t, msgs, 1)
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(msgs[0].(map[string]any)["content"].(string)), &sent))
	require.Equal(t, map[string]any{"title": "300 problems", "kind": "quantity", "unit": "problems",
		"target_quantity": float64(300), "minutes_per_unit": float64(30), "start_day": "2026-09-15",
		"due_day": "2026-12-15", "existing_tasks": []any{"Arrays"}, "instructions": "focus on graphs",
		"today": "2026-09-20", "daily_minutes": nil, "done_quantity": float64(0),
		"remaining_quantity": float64(0)}, sent, "exactly the fields of the spec, nothing else")
}

func TestOpenAICompatibleRequestShape(t *testing.T) {
	t.Parallel()
	p := &provider{reply: func(*http.Request) string { return goodBreakdown }}
	srv := p.server(t, false)
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Endpoint, cfg.Model = config.ProviderOpenAICompatible, srv.URL+"/v1/", "llama3"

	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
	require.NoError(t, err)
	_, err = a.Breakdown(context.Background(), goalReq())
	require.NoError(t, err)
	r, body := p.last()
	require.Equal(t, "/v1/chat/completions", r.URL.Path)
	require.Empty(t, r.Header.Get("Authorization"), "no key file, no Authorization header")
	require.Equal(t, "llama3", body["model"])
	require.Equal(t, map[string]any{"type": "json_object"}, body["response_format"])
	msgs := body["messages"].([]any)
	require.Equal(t, "system", msgs[0].(map[string]any)["role"])
	require.Equal(t, "user", msgs[1].(map[string]any)["role"])

	a, err = llm.New(cfg, llm.Options{CredDir: withKey(t, "k")})
	require.NoError(t, err)
	_, err = a.Breakdown(context.Background(), goalReq())
	require.NoError(t, err)
	r, _ = p.last()
	require.Equal(t, "Bearer k", r.Header.Get("Authorization"))
}

func TestUnavailable(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults().LLM
	_, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "k")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "provider none")
	cfg.Provider = config.ProviderAnthropic
	_, err = llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "anthropic needs a key")
	cfg.Provider = config.ProviderOpenAICompatible
	_, err = llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "openai_compatible needs an endpoint")
}

func TestBreakdownValidation(t *testing.T) {
	t.Parallel()
	task := func(mut string) string {
		base := map[string]any{"title": "t", "notes": "", "estimate_minutes": 60, "due_day": "2026-10-01", "priority": 2, "quantity": nil}
		if mut != "" {
			var m map[string]any
			json.Unmarshal([]byte(mut), &m)
			for k, v := range m {
				base[k] = v
			}
		}
		b, _ := json.Marshal(base)
		return string(b)
	}
	many := make([]string, 101)
	for i := range many {
		many[i] = task("")
	}
	tests := []struct{ name, reply, want string }{
		{"not JSON", "Sure! Here are some tasks.", "not the breakdown JSON"},
		{"no tasks", `{"tasks": []}`, "1 to 100 tasks"},
		{"too many", `{"tasks": [` + strings.Join(many, ",") + `]}`, "1 to 100 tasks"},
		{"long notes", `{"tasks": [` + task(`{"notes": "`+strings.Repeat("x", 2001)+`"}`) + `]}`, "notes"},
		{"unknown field", `{"tasks": [` + task(`{"colour": "red"}`) + `]}`, "not valid"},
		{"missing field", `{"tasks": [{"title": "t"}]}`, "lacks"},
		{"empty title", `{"tasks": [` + task(`{"title": " "}`) + `]}`, "title"},
		{"long title", `{"tasks": [` + task(`{"title": "`+strings.Repeat("x", 201)+`"}`) + `]}`, "title"},
		{"short estimate", `{"tasks": [` + task(`{"estimate_minutes": 4}`) + `]}`, "estimate_minutes"},
		{"long estimate", `{"tasks": [` + task(`{"estimate_minutes": 481}`) + `]}`, "estimate_minutes"},
		{"due before today", `{"tasks": [` + task(`{"due_day": "2026-09-19"}`) + `]}`, "outside"},
		{"due after the goal", `{"tasks": [` + task(`{"due_day": "2026-12-16"}`) + `]}`, "outside"},
		{"bad due", `{"tasks": [` + task(`{"due_day": "soon"}`) + `]}`, "not a date"},
		{"priority", `{"tasks": [` + task(`{"priority": 5}`) + `]}`, "priority"},
		{"quantity", `{"tasks": [` + task(`{"quantity": 0}`) + `]}`, "quantity"},
		{"trailing", `{"tasks": [` + task("") + `]} {}`, "trailing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &provider{reply: func(*http.Request) string { return tc.reply }}
			srv := p.server(t, false)
			cfg := config.Defaults().LLM
			cfg.Provider, cfg.Endpoint = config.ProviderOpenAICompatible, srv.URL
			a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "")})
			require.NoError(t, err)
			_, err = a.Breakdown(context.Background(), goalReq())
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestFailures(t *testing.T) {
	t.Parallel()
	p := &provider{status: http.StatusServiceUnavailable}
	srv := p.server(t, true)
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Timeout = config.ProviderAnthropic, 50*time.Millisecond
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "k"), AnthropicURL: srv.URL})
	require.NoError(t, err)
	_, err = a.Breakdown(context.Background(), goalReq())
	require.ErrorContains(t, err, "HTTP 503: Overloaded")

	p.mu.Lock()
	p.status, p.delay = 0, time.Second
	p.reply = func(*http.Request) string { return goodBreakdown }
	p.mu.Unlock()
	_, err = a.Breakdown(context.Background(), goalReq())
	require.ErrorContains(t, err, "timed out after 50ms")
}

func retroReq() llm.RetroRequest {
	return llm.RetroRequest{WeekStart: "2026-09-14",
		Days: []llm.RetroDay{
			{Day: "2026-09-14", WorkedMin: 480, BreakMin: 40, TargetMin: 480, TargetMet: true},
			{Day: "2026-09-15", WorkedMin: 300, BreakMin: 20, TargetMin: 480},
			{Day: "2026-09-16"}, {Day: "2026-09-17"}, {Day: "2026-09-18"}, {Day: "2026-09-19"}, {Day: "2026-09-20"},
		},
		Projects:  []llm.RetroProject{{"Internship", 600}, {"Study", 150}, {"Reading", 25}, {"Chores", 5}},
		Completed: []string{"Report"},
		Goals: []llm.RetroGoal{
			{Title: "300 problems", Done: 12, Remaining: 288, Unit: "problems", RequiredPerDay: 3.2, ActualPerDay: 1.5, Pace: "behind", DueDay: "2026-12-15"},
			{Title: "Thesis", Pace: "ahead", DueDay: "2026-10-31"},
		},
		PlannedMinutes: 600, DoneMinutes: 450}
}

func TestRetro(t *testing.T) {
	t.Parallel()
	p := &provider{reply: func(*http.Request) string { return `{"markdown": "A solid Monday."}` }}
	srv := p.server(t, true)
	cfg := config.Defaults().LLM
	cfg.Provider = config.ProviderAnthropic
	a, err := llm.New(cfg, llm.Options{CredDir: withKey(t, "k"), AnthropicURL: srv.URL})
	require.NoError(t, err)
	out, err := a.Retro(context.Background(), retroReq())
	require.NoError(t, err)
	require.Equal(t, llm.RetroOutput{Markdown: "A solid Monday.", GeneratedBy: "llm"}, out)
	_, body := p.last()
	require.Contains(t, system(t, body)[0], llm.RetroSchema)

	for name, reply := range map[string]string{
		"bad JSON":  "no",
		"too long":  `{"markdown": "` + strings.Repeat("x", 4001) + `"}`,
		"empty":     `{"markdown": ""}`,
		"extra key": `{"markdown": "x", "mood": "good"}`,
	} {
		p.mu.Lock()
		p.reply = func(*http.Request) string { return reply }
		p.mu.Unlock()
		out, err := a.Retro(context.Background(), retroReq())
		require.Error(t, err, name)
		require.Equal(t, "rules", out.GeneratedBy, name)
		require.NotEmpty(t, out.Markdown, name)
	}
}

func TestRulesRetro(t *testing.T) {
	t.Parallel()
	out := llm.RulesRetro(retroReq())
	require.Equal(t, "rules", out.GeneratedBy)
	require.Equal(t, `## Week of 2026-09-14

Worked 13h over 2 days, with 1h on breaks. The target was met on 1 of 7 days.
Of 10h planned, 7h 30m was done (75%).

### Top projects

- Internship: 10h
- Study: 2h 30m
- Reading: 25m

Completed 1 task.

### Goals behind pace

- 300 problems: 12 done, 288 left by 2026-12-15; needs 3.2 a day, doing 1.5.`, out.Markdown)
}

// The system prompt is the same from one request to the next, whatever the
// assistant chose about the user, so it is cached; what she chose follows it.
func TestPromptCaching(t *testing.T) {
	t.Parallel()
	reply := `{"reply": "Hi.", "mood": "smiling", "attitude": null, "self_note": null, "actions": []}`
	p := &provider{reply: func(*http.Request) string { return reply }}
	srv := p.server(t, true)
	cfg := config.Defaults().LLM
	cfg.Provider = config.ProviderAnthropic
	creds := withKey(t, "k")
	ask := func(self llm.Self) (llm.AssistantOutput, []string, map[string]any) {
		a, err := llm.New(cfg, llm.Options{CredDir: creds, AnthropicURL: srv.URL, Self: self})
		require.NoError(t, err)
		out, err := a.Assistant(context.Background(), llm.AssistantRequest{Today: "2026-09-20"})
		require.NoError(t, err)
		_, body := p.last()
		return out, system(t, body), body
	}

	out, fresh, body := ask(llm.Self{})
	require.Len(t, fresh, 1, "nothing chosen yet, nothing follows")
	require.Equal(t, map[string]any{"type": "ephemeral"}, body["system"].([]any)[0].(map[string]any)["cache_control"])
	require.True(t, strings.HasPrefix(fresh[0], "# GWEN STACY"))
	require.True(t, strings.HasSuffix(fresh[0], llm.AssistantSchema), "the job's prompt and schema are cached with the persona")
	require.Equal(t, llm.Usage{Input: 50, CacheRead: 3000, Output: 120}, out.Usage)

	_, angry, body := ask(llm.Self{Attitude: "angry", Note: "Third excuse.", Since: "2026-09-14"})
	require.Equal(t, fresh[0], angry[0], "the cached prefix does not change with her attitude")
	require.Len(t, angry, 2)
	require.Nil(t, body["system"].([]any)[1].(map[string]any)["cache_control"])
	require.True(t, strings.HasPrefix(angry[1], "\n\n## "), "the tail starts a section of its own: %q", angry[1][:20])
	require.Contains(t, angry[1], "you are being angry")
	require.Contains(t, angry[1], "Third excuse.")

	o := &provider{reply: func(*http.Request) string { return reply }}
	osrv := o.server(t, false)
	ocfg := config.Defaults().LLM
	ocfg.Provider, ocfg.Endpoint = config.ProviderOpenAICompatible, osrv.URL
	a, err := llm.New(ocfg, llm.Options{CredDir: creds, Self: llm.Self{Attitude: "soft"}})
	require.NoError(t, err)
	out, err = a.Assistant(context.Background(), llm.AssistantRequest{})
	require.NoError(t, err)
	require.Equal(t, llm.Usage{Input: 50, CacheRead: 3000, Output: 120}, out.Usage)
	_, obody := o.last()
	sys := obody["messages"].([]any)[0].(map[string]any)["content"].(string)
	require.True(t, strings.HasPrefix(sys, fresh[0]), "one system message, the stable part first")
	require.True(t, strings.HasSuffix(sys, llm.SelfPrompt(llm.Self{Attitude: "soft"})))
}
