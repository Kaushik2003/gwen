package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"
)

// The memory jobs (Memory) on the adapter. Each reply parser rejects a reply
// that is not the job's JSON object, but drops an entry that is malformed,
// rather than failing the whole reply, and keeps at most the job's limit.

// entityKinds are the kinds a new entity may have (memory.EntityKinds).
var entityKinds = []string{"person", "org", "place", "topic"}

// Extract asks the model for the facts the items state. With no items there
// is nothing to read, and no request.
func (a *adapter) Extract(ctx context.Context, req ExtractRequest) (ExtractOutput, error) {
	if len(req.Items) == 0 {
		return ExtractOutput{Facts: []ExtractedFact{}}, nil
	}
	req.Keys, req.Entities, req.Known = orEmpty(req.Keys), orEmpty(req.Entities), orEmpty(req.Known)
	req.Context = orEmpty(req.Context)
	for i := range req.Entities {
		req.Entities[i].Aliases = orEmpty(req.Entities[i].Aliases)
	}
	text, _, err := a.ask(ctx, extractJob, req)
	if err != nil {
		return ExtractOutput{}, err
	}
	return parseExtract(text, req)
}

// Verify asks the model whether each check's words support its claim.
func (a *adapter) Verify(ctx context.Context, req VerifyRequest) (VerifyOutput, error) {
	if len(req.Checks) == 0 {
		return VerifyOutput{Verdicts: []Verdict{}}, nil
	}
	text, _, err := a.ask(ctx, verifyJob, req)
	if err != nil {
		return VerifyOutput{}, err
	}
	return parseVerify(text, req)
}

// Observe asks the model for observations of the messages.
func (a *adapter) Observe(ctx context.Context, req ObserveRequest) (ObserveOutput, error) {
	if len(req.Messages) == 0 {
		return ObserveOutput{Observations: []Observation{}}, nil
	}
	text, _, err := a.ask(ctx, observeJob, req)
	if err != nil {
		return ObserveOutput{}, err
	}
	return parseObserve(text)
}

// Reflect asks the model to condense the log. An empty log stays empty.
func (a *adapter) Reflect(ctx context.Context, req ReflectRequest) (ReflectOutput, error) {
	if strings.TrimSpace(req.Log) == "" {
		return ReflectOutput{}, nil
	}
	text, _, err := a.ask(ctx, reflectJob, req)
	if err != nil {
		return ReflectOutput{}, err
	}
	return parseReflect(text)
}

// Contradict asks the model for the pairs of claims that cannot both hold.
// Fewer than two claims cannot contradict each other.
func (a *adapter) Contradict(ctx context.Context, req ContradictRequest) (ContradictOutput, error) {
	if len(req.Claims) < 2 {
		return ContradictOutput{Conflicts: []Contradiction{}}, nil
	}
	text, _, err := a.ask(ctx, contradictJob, req)
	if err != nil {
		return ContradictOutput{}, err
	}
	return parseContradict(text, req)
}

// Insight asks the model what the data suggests. With no data there is
// nothing to find.
func (a *adapter) Insight(ctx context.Context, req InsightRequest) (InsightOutput, error) {
	if len(req.Data) == 0 {
		return InsightOutput{Insights: []Insight{}}, nil
	}
	req.Known = orEmpty(req.Known)
	text, _, err := a.ask(ctx, insightJob, req)
	if err != nil {
		return InsightOutput{}, err
	}
	return parseInsight(text)
}

// parseExtract reads an extraction reply. A fact is dropped when its key,
// subject, value, text, quote, or source is missing or wrong: its source must
// be one of the items, and its subject "user", a known entity's ref, or a new
// "kind:Name". A fact marked secret is dropped too, so a secret never leaves
// this package. Other fields are made safe instead: a date that is not
// YYYY-MM-DD (or RFC 3339) becomes "", an unknown sensitivity "", an updates
// ref that is not a known claim "", and confidence is kept within 0 to 1 (0.5
// when missing). Repeats are dropped.
func parseExtract(text string, req ExtractRequest) (ExtractOutput, error) {
	list, err := entries(text, "facts", "extraction")
	if err != nil {
		return ExtractOutput{}, err
	}
	items := refSet(req.Items, func(i ExtractItem) string { return i.Ref })
	entities := refSet(req.Entities, func(e ExtractEntity) string { return e.Ref })
	known := refSet(req.Known, func(k ExtractKnown) string { return k.Ref })
	out := ExtractOutput{Facts: []ExtractedFact{}}
	for _, raw := range list {
		if len(out.Facts) == MaxExtractFacts {
			break
		}
		var f struct {
			Key         string          `json:"key"`
			Subject     string          `json:"subject"`
			Value       json.RawMessage `json:"value"`
			Text        string          `json:"text"`
			Quote       string          `json:"quote"`
			Source      string          `json:"source"`
			ValidFrom   string          `json:"valid_from"`
			ValidTo     string          `json:"valid_to"`
			OccurredAt  string          `json:"occurred_at"`
			Sensitivity string          `json:"sensitivity"`
			Confidence  *float64        `json:"confidence"`
			Updates     string          `json:"updates"`
		}
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		fact := ExtractedFact{Key: strings.ToLower(strings.TrimSpace(f.Key)), Text: oneLine(f.Text),
			Quote: strings.TrimSpace(f.Quote), Source: strings.TrimSpace(f.Source), ValidFrom: day(f.ValidFrom),
			ValidTo: day(f.ValidTo), OccurredAt: day(f.OccurredAt), Confidence: confidence(f.Confidence)}
		subject, ok := factSubject(f.Subject, entities)
		value, vok := factValue(f.Value)
		if !ok || !vok || fact.Key == "" || fact.Text == "" || fact.Quote == "" || !items[fact.Source] {
			continue
		}
		fact.Subject, fact.Value = subject, value
		switch s := strings.ToLower(strings.TrimSpace(f.Sensitivity)); s {
		case "secret":
			continue
		case "public", "personal", "sensitive":
			fact.Sensitivity = s
		}
		if u := strings.TrimSpace(f.Updates); known[u] {
			fact.Updates = u
		}
		if slices.ContainsFunc(out.Facts, func(o ExtractedFact) bool {
			return o.Key == fact.Key && o.Subject == fact.Subject && bytes.Equal(o.Value, fact.Value) &&
				o.Source == fact.Source
		}) {
			continue
		}
		out.Facts = append(out.Facts, fact)
	}
	return out, nil
}

// factSubject reads a fact's subject: "user", a known entity's ref, or a new
// entity as "kind:Name" with the kind lowercased and the name's spaces
// collapsed.
func factSubject(s string, entities map[string]bool) (string, bool) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "user") {
		return "user", true
	}
	if entities[s] {
		return s, true
	}
	kind, name, ok := strings.Cut(s, ":")
	kind, name = strings.ToLower(strings.TrimSpace(kind)), oneLine(name)
	if !ok || name == "" || !slices.Contains(entityKinds, kind) {
		return "", false
	}
	return kind + ":" + name, true
}

// factValue reads a value: any JSON but null and the empty string. A string
// is trimmed, and a string that holds a JSON object, as the schema has the
// model write one, is that object.
func factValue(raw json.RawMessage) (json.RawMessage, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) || string(raw) == "null" {
		return nil, false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		var compact bytes.Buffer
		if json.Compact(&compact, raw) != nil {
			return nil, false
		}
		return compact.Bytes(), true
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, false
	}
	var obj map[string]any
	if strings.HasPrefix(s, "{") && json.Unmarshal([]byte(s), &obj) == nil && len(obj) > 0 {
		return marshal(obj), true
	}
	return marshal(s), true
}

// parseVerify reads a verification reply. A verdict is dropped when its ref
// is not one of the checks or repeats one, or when supported is not yes,
// partly, or no. A check that had no quote is supported only with the words
// the model found, so yes or partly without them becomes no.
func parseVerify(text string, req VerifyRequest) (VerifyOutput, error) {
	list, err := entries(text, "verdicts", "verification")
	if err != nil {
		return VerifyOutput{}, err
	}
	checks := map[string]VerifyCheck{}
	for _, c := range req.Checks {
		checks[c.Ref] = c
	}
	out := VerifyOutput{Verdicts: []Verdict{}}
	for _, raw := range list {
		if len(out.Verdicts) == MaxVerifyChecks {
			break
		}
		var v Verdict
		if json.Unmarshal(raw, &v) != nil {
			continue
		}
		v.Ref, v.Quote = strings.TrimSpace(v.Ref), strings.TrimSpace(v.Quote)
		v.Supported = strings.ToLower(strings.TrimSpace(v.Supported))
		check, ok := checks[v.Ref]
		if !ok || !slices.Contains([]string{"yes", "partly", "no"}, v.Supported) ||
			slices.ContainsFunc(out.Verdicts, func(o Verdict) bool { return o.Ref == v.Ref }) {
			continue
		}
		if strings.TrimSpace(check.Quote) == "" && v.Quote == "" {
			v.Supported = "no"
		}
		out.Verdicts = append(out.Verdicts, v)
	}
	return out, nil
}

// parseObserve reads an observer reply. An observation is dropped when its
// date is not YYYY-MM-DD or it has no text; its text is put on one line and
// cut to MaxObservationLen, and an unknown priority is medium. Repeats are
// dropped.
func parseObserve(text string) (ObserveOutput, error) {
	list, err := entries(text, "observations", "observation")
	if err != nil {
		return ObserveOutput{}, err
	}
	out := ObserveOutput{Observations: []Observation{}}
	for _, raw := range list {
		if len(out.Observations) == MaxObservations {
			break
		}
		var o Observation
		if json.Unmarshal(raw, &o) != nil {
			continue
		}
		o.Date, o.Text, o.Priority = strings.TrimSpace(o.Date), cutRunes(oneLine(o.Text), MaxObservationLen),
			priority(o.Priority)
		if !isDate(o.Date) || o.Text == "" || slices.Contains(out.Observations, o) {
			continue
		}
		out.Observations = append(out.Observations, o)
	}
	return out, nil
}

// logLine is a line of an observation log, with or without its leading dash.
var logLine = regexp.MustCompile(`^(?:[-*]\s*)?(\d{4}-\d{2}-\d{2})\s+\[(?i:(high|medium|low))\]\s+(\S.*)$`)

// parseReflect reads a reflector reply. Lines not in the log's format are
// dropped and the rest written in it exactly; a log that is still longer
// than MaxObservationLog loses low lines first, then medium, then high,
// oldest first. A reply with no line left is an error, so the log it would
// replace is kept.
func parseReflect(text string) (ReflectOutput, error) {
	var raw struct {
		Log *string `json:"log"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return ReflectOutput{}, fmt.Errorf("the reply is not the reflection JSON: %w", err)
	}
	if raw.Log == nil {
		return ReflectOutput{}, fmt.Errorf("the reflection has no log")
	}
	type line struct {
		text string
		rank int // 0 low, 1 medium, 2 high
	}
	var lines []line
	size := 0
	for _, l := range strings.Split(*raw.Log, "\n") {
		m := logLine.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil || !isDate(m[1]) {
			continue
		}
		p := strings.ToLower(m[2])
		lines = append(lines, line{"- " + m[1] + " [" + p + "] " + oneLine(m[3]),
			slices.Index([]string{"low", "medium", "high"}, p)})
		size += len([]rune(lines[len(lines)-1].text)) + 1
	}
	if len(lines) == 0 {
		return ReflectOutput{}, fmt.Errorf("the reflection has no line in the log's format")
	}
	// The joined log is size-1 characters long: one line break fewer.
	drop := make([]bool, len(lines))
	for rank := 0; rank <= 2; rank++ {
		for i, l := range lines {
			if size-1 <= MaxObservationLog {
				break
			}
			if l.rank == rank {
				drop[i], size = true, size-len([]rune(l.text))-1
			}
		}
	}
	var texts []string
	for i, l := range lines {
		if !drop[i] {
			texts = append(texts, l.text)
		}
	}
	return ReflectOutput{Log: strings.Join(texts, "\n")}, nil
}

// parseContradict reads a contradiction scan's reply. A pair is dropped when
// either ref is not one of the claims, both are the same, or it repeats a
// pair in either order; its reason is put on one line and cut to 300
// characters.
func parseContradict(text string, req ContradictRequest) (ContradictOutput, error) {
	list, err := entries(text, "conflicts", "contradiction")
	if err != nil {
		return ContradictOutput{}, err
	}
	claims := refSet(req.Claims, func(c ContradictItem) string { return c.Ref })
	out := ContradictOutput{Conflicts: []Contradiction{}}
	for _, raw := range list {
		if len(out.Conflicts) == MaxContradictions {
			break
		}
		var c Contradiction
		if json.Unmarshal(raw, &c) != nil {
			continue
		}
		c.A, c.B, c.Reason = strings.TrimSpace(c.A), strings.TrimSpace(c.B), cutRunes(oneLine(c.Reason), 300)
		if !claims[c.A] || !claims[c.B] || c.A == c.B || slices.ContainsFunc(out.Conflicts, func(o Contradiction) bool {
			return o.A == c.A && o.B == c.B || o.A == c.B && o.B == c.A
		}) {
			continue
		}
		out.Conflicts = append(out.Conflicts, c)
	}
	return out, nil
}

// insightKey is a key an insight may have: pref.work, user.routine, or
// user.struggle, or a key under one of them.
var insightKey = regexp.MustCompile(`^(pref\.work|user\.routine|user\.struggle)(\.[a-z0-9_]+)*$`)

// parseInsight reads an insight reply. An insight is dropped when its key is
// not an insight key, or its value, text, or basis is missing; confidence is
// kept within 0 to 1 (0.5 when missing).
func parseInsight(text string) (InsightOutput, error) {
	list, err := entries(text, "insights", "insight")
	if err != nil {
		return InsightOutput{}, err
	}
	out := InsightOutput{Insights: []Insight{}}
	for _, raw := range list {
		if len(out.Insights) == MaxInsights {
			break
		}
		var in struct {
			Key        string          `json:"key"`
			Value      json.RawMessage `json:"value"`
			Text       string          `json:"text"`
			Basis      string          `json:"basis"`
			Confidence *float64        `json:"confidence"`
		}
		if json.Unmarshal(raw, &in) != nil {
			continue
		}
		insight := Insight{Key: strings.ToLower(strings.TrimSpace(in.Key)), Text: oneLine(in.Text),
			Basis: oneLine(in.Basis), Confidence: confidence(in.Confidence)}
		value, ok := factValue(in.Value)
		if !ok || !insightKey.MatchString(insight.Key) || insight.Text == "" || insight.Basis == "" {
			continue
		}
		insight.Value = value
		out.Insights = append(out.Insights, insight)
	}
	return out, nil
}

// entries decodes a reply object and returns the entries of its list field,
// none when the field is missing or null. A reply that is not a JSON object,
// or whose field is not a list, is an error.
func entries(text, field, job string) ([]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return nil, fmt.Errorf("the reply is not the %s JSON: %w", job, err)
	}
	raw, ok := obj[field]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("the %s reply's %s is not a list", job, field)
	}
	return list, nil
}

// refSet is the set of the refs of a request's list.
func refSet[T any](list []T, ref func(T) string) map[string]bool {
	set := make(map[string]bool, len(list))
	for _, x := range list {
		set[ref(x)] = true
	}
	return set
}

// orEmpty is s, or an empty slice for nil, so the request sends [] rather
// than null.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// day is s when it is a date, YYYY-MM-DD or RFC 3339, else "".
func day(s string) string {
	s = strings.TrimSpace(s)
	if isDate(s) {
		return s
	}
	if _, err := time.Parse(time.RFC3339, s); err == nil {
		return s
	}
	return ""
}

func isDate(s string) bool {
	_, err := time.Parse(time.DateOnly, s)
	return err == nil
}

// priority is an observation's priority, medium when unknown.
func priority(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "high" || s == "low" {
		return s
	}
	return "medium"
}

// confidence is c within 0 to 1, and 0.5 when the model gave none.
func confidence(c *float64) float64 {
	if c == nil || math.IsNaN(*c) {
		return 0.5
	}
	return min(max(*c, 0), 1)
}

// oneLine is s with its runs of white space, line breaks included, made one
// space, and trimmed.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// cutRunes is s cut to at most n characters.
func cutRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}

// marshal is the compact JSON of v, with <, >, and & left as they are.
func marshal(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil
	}
	return bytes.TrimRight(b.Bytes(), "\n")
}
