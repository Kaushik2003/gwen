package llm_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/stretchr/testify/require"
)

// memoryModel is the memory jobs over a fake OpenAI-compatible endpoint
// that replies with reply.
func memoryModel(t *testing.T, reply string) (llm.Memory, *provider) {
	t.Helper()
	p := &provider{reply: func(*http.Request) string { return reply }}
	srv := p.server(t, false)
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Endpoint, cfg.Model = config.ProviderOpenAICompatible, srv.URL, "llama3"
	m, err := llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "")})
	require.NoError(t, err)
	return m, p
}

// sentMemory is the system prompt and the decoded user message of the last
// request.
func sentMemory(t *testing.T, p *provider) (string, map[string]any) {
	t.Helper()
	_, body := p.last()
	msgs := body["messages"].([]any)
	require.Len(t, msgs, 2, "a memory job has no history")
	var user map[string]any
	require.NoError(t, json.Unmarshal([]byte(msgs[1].(map[string]any)["content"].(string)), &user))
	return msgs[0].(map[string]any)["content"].(string), user
}

func requests(p *provider) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

func TestNewMemory(t *testing.T) {
	t.Parallel()
	cfg := config.Defaults().LLM
	_, err := llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "k")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "provider none")
	cfg.Provider = config.ProviderAnthropic
	_, err = llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "anthropic needs a key")
	cfg.Provider = config.ProviderOpenAICompatible
	_, err = llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "openai_compatible needs an endpoint")
	cfg.Provider, cfg.Command = config.ProviderClaudeCode, filepath.Join(t.TempDir(), "claude")
	_, err = llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "")})
	require.ErrorIs(t, err, llm.ErrUnavailable, "claude_code needs the CLI")

	cfg.Provider = config.ProviderAnthropic
	m, err := llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "k")})
	require.NoError(t, err)
	require.Equal(t, "anthropic/claude-sonnet-5", m.Name(), "the provider and the model")
	cfg.Provider, cfg.Endpoint, cfg.Model = config.ProviderOpenAICompatible, "http://127.0.0.1:11434/v1", ""
	m, err = llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "")})
	require.NoError(t, err)
	require.Equal(t, "openai_compatible", m.Name(), "the provider alone without a model")
}

// The memory prompts carry no persona and no tail, whatever she chose about
// the user, and are cached for an hour like every system prompt.
func TestMemoryPromptsHaveNoPersona(t *testing.T) {
	t.Parallel()
	p := &provider{reply: func(*http.Request) string { return `{"facts": []}` }}
	srv := p.server(t, true)
	cfg := config.Defaults().LLM
	cfg.Provider = config.ProviderAnthropic
	m, err := llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "k"), AnthropicURL: srv.URL,
		Self: llm.Self{Attitude: "angry", Note: "Third excuse."}})
	require.NoError(t, err)
	out, err := m.Extract(context.Background(), extractReq())
	require.NoError(t, err)
	require.Equal(t, []llm.ExtractedFact{}, out.Facts)

	_, body := p.last()
	blocks := body["system"].([]any)
	require.Len(t, blocks, 1, "no context, no tail")
	block := blocks[0].(map[string]any)
	text := block["text"].(string)
	require.True(t, strings.HasPrefix(text, "You keep the long-term memory of a personal assistant"))
	require.True(t, strings.HasSuffix(text, llm.ExtractSchema))
	require.NotContains(t, text, "GWEN STACY")
	require.NotContains(t, text, "Third excuse.")
	require.Equal(t, map[string]any{"type": "ephemeral", "ttl": "1h"}, block["cache_control"])
}

// Every memory schema is JSON, closed, and requires each of its fields.
func TestMemorySchemas(t *testing.T) {
	t.Parallel()
	var check func(name string, node map[string]any)
	check = func(name string, node map[string]any) {
		if props, ok := node["properties"].(map[string]any); ok {
			require.Equal(t, false, node["additionalProperties"], name)
			var required []string
			for _, r := range node["required"].([]any) {
				required = append(required, r.(string))
			}
			keys := make([]string, 0, len(props))
			for k, v := range props {
				keys = append(keys, k)
				check(name+"."+k, v.(map[string]any))
			}
			require.ElementsMatch(t, keys, required, "%s requires every field", name)
		}
		if items, ok := node["items"].(map[string]any); ok {
			check(name+"[]", items)
		}
	}
	for name, schema := range map[string]string{"extract": llm.ExtractSchema, "verify": llm.VerifySchema,
		"observe": llm.ObserveSchema, "reflect": llm.ReflectSchema, "contradict": llm.ContradictSchema,
		"insight": llm.InsightSchema} {
		var node map[string]any
		require.NoError(t, json.Unmarshal([]byte(schema), &node), name)
		check(name, node)
	}
}

func extractReq() llm.ExtractRequest {
	return llm.ExtractRequest{Today: "2026-10-12", Now: "2026-10-12T18:20:00+02:00", Timezone: "Europe/Berlin",
		Keys:     []llm.ExtractKey{{Key: "user.employer", Label: "Works at", Cardinality: "one", About: "user"}},
		Entities: []llm.ExtractEntity{{Ref: "e1", Kind: "person", Name: "Anna"}},
		Known:    []llm.ExtractKnown{{Ref: "k1", Key: "user.employer", Subject: "user", Text: "Works at Initech"}},
		Context: []llm.ExtractItem{{Ref: "c1", Speaker: "gwen", At: "2026-10-12 18:18",
			Text: "Where do you work now?"}},
		Items: []llm.ExtractItem{{Ref: "i1", Speaker: "user", At: "2026-10-12 18:19",
			Text: "Started at Acme last Monday. Anna works from 9 to 5."}}}
}

func TestExtract(t *testing.T) {
	t.Parallel()
	fact := func(change map[string]any) map[string]any {
		f := map[string]any{"source": "i1", "quote": "Started at Acme last Monday", "subject": "user",
			"key": "user.employer", "updates": "k1", "value": "Acme", "text": "Works at Acme",
			"valid_from": "2026-10-05", "valid_to": "", "occurred_at": "", "sensitivity": "personal",
			"confidence": 0.95}
		for k, v := range change {
			if v == nil {
				delete(f, k)
			} else {
				f[k] = v
			}
		}
		return f
	}
	facts := []any{
		fact(nil),
		fact(nil), // a repeat
		fact(map[string]any{"subject": " Person : Anna  Lee ", "key": "Person.Fact",
			"value": `{"start": "09:00", "end": "17:00"}`, "text": "Anna works\n from 9 to 5",
			"quote": "Anna works from 9 to 5", "updates": "k9", "valid_from": "last week", "sensitivity": "SENSITIVE",
			"confidence": 1.4}),
		fact(map[string]any{"subject": "e1", "key": "person.relation", "value": "sister",
			"text": "Anna is their sister", "valid_from": "2026-10-05T00:00:00+02:00", "sensitivity": "family",
			"confidence": nil}),
		fact(map[string]any{"sensitivity": "secret"}),
		fact(map[string]any{"source": "c1"}),
		fact(map[string]any{"subject": "e9"}),
		fact(map[string]any{"subject": "pet:Rex"}),
		fact(map[string]any{"value": nil}),
		fact(map[string]any{"value": "  "}),
		fact(map[string]any{"text": " "}),
		fact(map[string]any{"quote": nil}),
		fact(map[string]any{"key": ""}),
		fact(map[string]any{"confidence": "high"}),
		"not a fact",
	}
	reply, err := json.Marshal(map[string]any{"facts": facts})
	require.NoError(t, err)
	m, p := memoryModel(t, string(reply))
	require.Equal(t, "openai_compatible/llama3", m.Name())

	out, err := m.Extract(context.Background(), extractReq())
	require.NoError(t, err)
	require.Equal(t, []llm.ExtractedFact{
		{Key: "user.employer", Subject: "user", Value: json.RawMessage(`"Acme"`), Text: "Works at Acme",
			Quote: "Started at Acme last Monday", Source: "i1", ValidFrom: "2026-10-05", Sensitivity: "personal",
			Confidence: 0.95, Updates: "k1"},
		{Key: "person.fact", Subject: "person:Anna Lee", Value: json.RawMessage(`{"end":"17:00","start":"09:00"}`),
			Text: "Anna works from 9 to 5", Quote: "Anna works from 9 to 5", Source: "i1", Sensitivity: "sensitive",
			Confidence: 1},
		{Key: "person.relation", Subject: "e1", Value: json.RawMessage(`"sister"`), Text: "Anna is their sister",
			Quote: "Started at Acme last Monday", Source: "i1", ValidFrom: "2026-10-05T00:00:00+02:00", Confidence: 0.5,
			Updates: "k1"},
	}, out.Facts, "malformed facts are dropped, the rest made safe")

	system, user := sentMemory(t, p)
	require.True(t, strings.HasPrefix(system, "You keep the long-term memory"))
	require.True(t, strings.HasSuffix(system, llm.ExtractSchema))
	require.Equal(t, "i1", user["items"].([]any)[0].(map[string]any)["ref"])
	require.Equal(t, false, user["explicit"])
	require.Equal(t, []any{}, user["entities"].([]any)[0].(map[string]any)["aliases"], "lists are sent as [], not null")
}

func TestExtractEdges(t *testing.T) {
	t.Parallel()
	m, p := memoryModel(t, "Here are the facts!")
	out, err := m.Extract(context.Background(), llm.ExtractRequest{Today: "2026-10-12"})
	require.NoError(t, err)
	require.Empty(t, out.Facts)
	require.Zero(t, requests(p), "no items, no request")

	_, err = m.Extract(context.Background(), extractReq())
	require.ErrorContains(t, err, "not the extraction JSON")

	m, _ = memoryModel(t, `{"facts": {"key": "user.employer"}}`)
	_, err = m.Extract(context.Background(), extractReq())
	require.ErrorContains(t, err, "not a list")

	m, p = memoryModel(t, `{}`)
	req := extractReq()
	req.Keys, req.Entities, req.Known, req.Context, req.Explicit = nil, nil, nil, nil, true
	out, err = m.Extract(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []llm.ExtractedFact{}, out.Facts, "a reply without facts has none")
	_, user := sentMemory(t, p)
	for _, k := range []string{"keys", "entities", "known", "context"} {
		require.Equal(t, []any{}, user[k], k)
	}
	require.Equal(t, true, user["explicit"])

	many := make([]string, llm.MaxExtractFacts+5)
	for i := range many {
		many[i] = fmt.Sprintf(`{"source": "i1", "quote": "Acme", "subject": "user", "key": "user.fact", "updates": "",
			"value": "fact %d", "text": "Fact %d", "valid_from": "", "valid_to": "", "occurred_at": "",
			"sensitivity": "public", "confidence": 0.9}`, i, i)
	}
	m, _ = memoryModel(t, `{"facts": [`+strings.Join(many, ",")+`]}`)
	out, err = m.Extract(context.Background(), extractReq())
	require.NoError(t, err)
	require.Len(t, out.Facts, llm.MaxExtractFacts)
}

func TestVerify(t *testing.T) {
	t.Parallel()
	m, p := memoryModel(t, `{"verdicts": [
		{"ref": "v1", "supported": "Yes", "quote": ""},
		{"ref": "v1", "supported": "no", "quote": ""},
		{"ref": "v2", "supported": "partly", "quote": " Started at Acme "},
		{"ref": "v3", "supported": "yes", "quote": ""},
		{"ref": "v4", "supported": "maybe", "quote": ""},
		{"ref": "v9", "supported": "yes", "quote": ""},
		{"ref": 5}
	]}`)
	req := llm.VerifyRequest{Checks: []llm.VerifyCheck{
		{Ref: "v1", Claim: "Works at Acme", Quote: "Started at Acme", Speaker: "user", Context: "Started at Acme."},
		{Ref: "v2", Claim: "Works at Acme since Monday", Speaker: "user", Context: "Started at Acme."},
		{Ref: "v3", Claim: "Anna is their sister", Speaker: "user", Context: "Anna visits."},
		{Ref: "v4", Claim: "Likes tea", Quote: "tea", Speaker: "user", Context: "tea"},
	}}
	out, err := m.Verify(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []llm.Verdict{
		{Ref: "v1", Supported: "yes"},
		{Ref: "v2", Supported: "partly", Quote: "Started at Acme"},
		{Ref: "v3", Supported: "no"},
	}, out.Verdicts, "a check without a quote needs the words found, or it is no")

	system, user := sentMemory(t, p)
	require.True(t, strings.HasSuffix(system, llm.VerifySchema))
	require.Len(t, user["checks"], 4)

	empty, _ := memoryModel(t, "")
	out, err = empty.Verify(context.Background(), llm.VerifyRequest{})
	require.NoError(t, err)
	require.Empty(t, out.Verdicts)
}

func TestObserve(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", llm.MaxObservationLen+20)
	m, p := memoryModel(t, `{"observations": [
		{"date": "2026-10-10", "priority": "high", "text": "They decided to drop\nthe Rust course."},
		{"date": "2026-10-10", "priority": "high", "text": "They decided to drop the Rust course."},
		{"date": "2026-10-11", "priority": "urgent", "text": "You moved the Rust tasks to someday."},
		{"date": "11 Oct", "priority": "low", "text": "Small talk."},
		{"date": "2026-10-11", "priority": "low", "text": "  "},
		{"date": "2026-10-11", "priority": "low", "text": "`+long+`"},
		["not", "one"]
	]}`)
	out, err := m.Observe(context.Background(), llm.ObserveRequest{Today: "2026-10-12", Log: "",
		Messages: []llm.ObserveMessage{{Role: "user", At: "2026-10-10 09:00", Text: "Drop the Rust course."}}})
	require.NoError(t, err)
	require.Equal(t, []llm.Observation{
		{Date: "2026-10-10", Priority: "high", Text: "They decided to drop the Rust course."},
		{Date: "2026-10-11", Priority: "medium", Text: "You moved the Rust tasks to someday."},
		{Date: "2026-10-11", Priority: "low", Text: long[:2*llm.MaxObservationLen]},
	}, out.Observations)
	system, user := sentMemory(t, p)
	require.True(t, strings.HasSuffix(system, llm.ObserveSchema))
	require.Equal(t, "", user["log"])

	many := make([]string, llm.MaxObservations+3)
	for i := range many {
		many[i] = fmt.Sprintf(`{"date": "2026-10-11", "priority": "low", "text": "Note %d."}`, i)
	}
	m, _ = memoryModel(t, `{"observations": [`+strings.Join(many, ",")+`]}`)
	out, err = m.Observe(context.Background(), llm.ObserveRequest{Messages: []llm.ObserveMessage{{Text: "x"}}})
	require.NoError(t, err)
	require.Len(t, out.Observations, llm.MaxObservations)
}

func TestReflect(t *testing.T) {
	t.Parallel()
	reply, err := json.Marshal(map[string]string{"log": "Here is the log:\n" +
		"- 2026-10-01 [high] They start at Acme on 5 Oct.\n" +
		"* 2026-10-02 [MEDIUM]   They want  a quieter week.\n" +
		"2026-10-03 [low] Talked about the weather.\n" +
		"- 2026-13-40 [high] A bad date.\n" +
		"- 2026-10-04 [urgent] A bad priority.\n"})
	require.NoError(t, err)
	m, p := memoryModel(t, string(reply))
	log := "- 2026-10-01 [high] They start at Acme on 5 Oct."
	out, err := m.Reflect(context.Background(), llm.ReflectRequest{Today: "2026-10-12", Log: log})
	require.NoError(t, err)
	require.Equal(t, "- 2026-10-01 [high] They start at Acme on 5 Oct.\n"+
		"- 2026-10-02 [medium] They want a quieter week.\n"+
		"- 2026-10-03 [low] Talked about the weather.", out.Log, "lines in the format, written in it exactly")
	system, user := sentMemory(t, p)
	require.True(t, strings.HasSuffix(system, llm.ReflectSchema))
	require.Equal(t, log, user["log"])

	// A log still too long loses low lines first, oldest first, then medium.
	var lines []string
	line := func(day int, p string) string {
		return fmt.Sprintf("- 2026-10-%02d [%s] %s", day, p, strings.Repeat("x", 980))
	}
	for day, p := range []string{"low", "high", "low", "medium", "high", "medium", "high", "low", "high", "medium",
		"high"} {
		lines = append(lines, line(day+1, p))
	}
	reply, err = json.Marshal(map[string]string{"log": strings.Join(lines, "\n")})
	require.NoError(t, err)
	m, _ = memoryModel(t, string(reply))
	out, err = m.Reflect(context.Background(), llm.ReflectRequest{Log: log})
	require.NoError(t, err)
	require.LessOrEqual(t, len([]rune(out.Log)), llm.MaxObservationLog)
	require.Equal(t, []string{line(2, "high"), line(4, "medium"), line(5, "high"), line(6, "medium"), line(7, "high"),
		line(9, "high"), line(10, "medium"), line(11, "high")}, strings.Split(out.Log, "\n"))

	m, _ = memoryModel(t, `{"log": "I could not condense it."}`)
	_, err = m.Reflect(context.Background(), llm.ReflectRequest{Log: log})
	require.ErrorContains(t, err, "no line", "an empty reflection never replaces the log")
	m, _ = memoryModel(t, `{"summary": ""}`)
	_, err = m.Reflect(context.Background(), llm.ReflectRequest{Log: log})
	require.ErrorContains(t, err, "no log")
	m, _ = memoryModel(t, `log: none`)
	_, err = m.Reflect(context.Background(), llm.ReflectRequest{Log: log})
	require.ErrorContains(t, err, "not the reflection JSON")

	m, p = memoryModel(t, "")
	out, err = m.Reflect(context.Background(), llm.ReflectRequest{Log: " \n"})
	require.NoError(t, err)
	require.Empty(t, out.Log)
	require.Zero(t, requests(p), "nothing to condense, no request")
}

func TestContradict(t *testing.T) {
	t.Parallel()
	m, p := memoryModel(t, `{"conflicts": [
		{"a": "k1", "b": "k2", "reason": "Two current\n employers."},
		{"a": "k2", "b": "k1", "reason": "The same pair."},
		{"a": "k1", "b": "k1", "reason": "One claim."},
		{"a": "k1", "b": "k9", "reason": "Not a claim."},
		{"a": "k3", "b": "k2", "reason": ""}
	]}`)
	req := llm.ContradictRequest{Today: "2026-10-12", Claims: []llm.ContradictItem{
		{Ref: "k1", Key: "user.employer", Subject: "user", Text: "Works at Acme"},
		{Ref: "k2", Key: "user.employer", Subject: "user", Text: "Works at Initech"},
		{Ref: "k3", Key: "user.home", Subject: "user", Text: "Lives in Berlin"},
	}}
	out, err := m.Contradict(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, []llm.Contradiction{{A: "k1", B: "k2", Reason: "Two current employers."}, {A: "k3", B: "k2"}},
		out.Conflicts)
	system, _ := sentMemory(t, p)
	require.True(t, strings.HasSuffix(system, llm.ContradictSchema))

	m, p = memoryModel(t, "")
	out, err = m.Contradict(context.Background(), llm.ContradictRequest{Claims: req.Claims[:1]})
	require.NoError(t, err)
	require.Empty(t, out.Conflicts)
	require.Zero(t, requests(p), "one claim cannot contradict itself")
}

func TestInsight(t *testing.T) {
	t.Parallel()
	insight := `{"key": %q, "value": %s, "text": "Works best before noon", "basis": %q, "confidence": %s}`
	list := []string{
		fmt.Sprintf(insight, "pref.work", `"mornings"`, "9 of 11 mornings on target", "0.8"),
		fmt.Sprintf(insight, "Pref.Work.Time", `"before noon"`, "9 of 11", "-2"),
		fmt.Sprintf(insight, "user.routine", `3`, "3 runs a week", "0.7"),
		fmt.Sprintf(insight, "user.fact", `"x"`, "basis", "0.9"),
		fmt.Sprintf(insight, "user.struggle", `"Fridays"`, " ", "0.9"),
		fmt.Sprintf(insight, "user.struggle", `null`, "basis", "0.9"),
		fmt.Sprintf(insight, "pref.workout", `"x"`, "basis", "0.9"),
	}
	for i := range 6 {
		list = append(list, fmt.Sprintf(insight, "user.routine", fmt.Sprintf(`"r%d"`, i), "basis", "0.6"))
	}
	m, p := memoryModel(t, `{"insights": [`+strings.Join(list, ",")+`]}`)
	out, err := m.Insight(context.Background(), llm.InsightRequest{Today: "2026-10-12",
		Data: []string{"Worked 9 of 11 mornings on target."}})
	require.NoError(t, err)
	require.Len(t, out.Insights, llm.MaxInsights)
	require.Equal(t, llm.Insight{Key: "pref.work", Value: json.RawMessage(`"mornings"`), Text: "Works best before noon",
		Basis: "9 of 11 mornings on target", Confidence: 0.8}, out.Insights[0])
	require.Equal(t, "pref.work.time", out.Insights[1].Key)
	require.Zero(t, out.Insights[1].Confidence, "kept within 0 to 1")
	require.Equal(t, json.RawMessage(`3`), out.Insights[2].Value)
	require.Equal(t, "user.routine", out.Insights[3].Key, "the malformed ones are skipped")
	system, user := sentMemory(t, p)
	require.True(t, strings.HasSuffix(system, llm.InsightSchema))
	require.Equal(t, []any{}, user["known"])

	m, p = memoryModel(t, "")
	out, err = m.Insight(context.Background(), llm.InsightRequest{Known: []string{"Works at Acme"}})
	require.NoError(t, err)
	require.Empty(t, out.Insights)
	require.Zero(t, requests(p), "no data, nothing to find")
}

// A memory job runs through the Claude Code CLI like any other job.
func TestMemoryOverClaudeCode(t *testing.T) {
	notUnderSystemd(t)
	f := newFakeCLI(t)
	f.reply(map[string]any{"is_error": false, "structured_output": map[string]any{"conflicts": []any{
		map[string]any{"a": "k1", "b": "k2", "reason": "Two employers."}}}})
	cfg := config.Defaults().LLM
	cfg.Provider, cfg.Command, cfg.Model = config.ProviderClaudeCode, f.path, "claude-sonnet-5-5"
	m, err := llm.NewMemory(cfg, llm.Options{CredDir: withKey(t, "")})
	require.NoError(t, err)
	require.Equal(t, "claude_code/claude-sonnet-5-5", m.Name())
	out, err := m.Contradict(context.Background(), llm.ContradictRequest{Claims: []llm.ContradictItem{
		{Ref: "k1", Text: "Works at Acme"}, {Ref: "k2", Text: "Works at Initech"}}})
	require.NoError(t, err)
	require.Equal(t, []llm.Contradiction{{A: "k1", B: "k2", Reason: "Two employers."}}, out.Conflicts)
	require.True(t, strings.HasPrefix(f.read("system"), "You check the memory of a personal assistant"))
	require.Equal(t, llm.ContradictSchema, f.args()[8])
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.read("stdin")), &sent), "no history, just the request")
	require.Len(t, sent["claims"], 2)
}
