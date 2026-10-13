package learn

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// Observation log limits (spec 5, 7).
const (
	observeCount  = 24    // messages after the log that start a fold
	observeTokens = 6000  // or their tokens
	observeKeep   = 8     // newest messages left verbatim
	reflectChars  = 12000 // characters of a log that start a reflection
)

// Observe folds a thread's older messages into its observation log when
// enough have piled up since the log (spec 5, 7): all but the newest 8 of
// them become dated observations, and a log grown too long is condensed. It
// reports whether it folded. A thread deleted meanwhile is not an error.
func (l *Learner) Observe(ctx context.Context, threadID string) (bool, error) {
	l.observeMu.Lock()
	defer l.observeMu.Unlock()
	msgs, err := l.chats.Messages(ctx, threadID)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("observe chat: %w", err)
	}
	o, err := l.o.State.Observation(ctx, threadID)
	if err != nil && !isNotFound(err) {
		return false, fmt.Errorf("observe chat: %w", err)
	}
	after := msgs
	if err == nil {
		if i := slices.IndexFunc(msgs, func(m model.ChatMessage) bool { return m.ID == o.ThroughID }); i >= 0 {
			after = msgs[i+1:]
		} else {
			after = slices.DeleteFunc(slices.Clone(msgs), func(m model.ChatMessage) bool { return !m.At.After(o.ThroughAt) })
		}
	}
	tokens := 0
	for _, m := range after {
		tokens += memory.Tokens(m.Text)
	}
	if len(after) <= observeCount && tokens <= observeTokens || len(after) <= observeKeep {
		return false, nil
	}
	fold := after[:len(after)-observeKeep]
	m, err := l.model()
	if err != nil {
		return false, fmt.Errorf("observe chat: %w", err)
	}
	today := l.day(l.now())
	req := llm.ObserveRequest{Today: today, Log: o.Log, Messages: make([]llm.ObserveMessage, len(fold))}
	for i, msg := range fold {
		req.Messages[i] = llm.ObserveMessage{Role: msg.Role, At: l.localStamp(msg.At), Text: memory.Cut(msg.Text, itemChars)}
	}
	out, err := m.Observe(ctx, req)
	if err != nil {
		return false, fmt.Errorf("observe chat: %w", err)
	}
	log := o.Log
	for _, ob := range out.Observations {
		if log != "" {
			log += "\n"
		}
		log += "- " + ob.Date + " [" + ob.Priority + "] " + ob.Text
	}
	reflections := o.Reflections
	if utf8.RuneCountInString(log) > reflectChars {
		r, err := m.Reflect(ctx, llm.ReflectRequest{Today: today, Log: log})
		if err != nil {
			return false, fmt.Errorf("condense observations: %w", err)
		}
		log, reflections = capLog(r.Log, llm.MaxObservationLog), reflections+1
	}
	last := fold[len(fold)-1]
	err = l.o.State.SetObservation(ctx, model.ChatObservation{ThreadID: threadID, ThroughID: last.ID,
		ThroughAt: last.At, Log: log, Reflections: reflections})
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("observe chat: %w", err)
	}
	return true, nil
}

// capLog drops a log's oldest lines until it is at most n characters.
func capLog(log string, n int) string {
	lines := strings.Split(strings.TrimSpace(log), "\n")
	for len(lines) > 1 && utf8.RuneCountInString(strings.Join(lines, "\n")) > n {
		lines = lines[1:]
	}
	return cutRunes(strings.Join(lines, "\n"), n)
}

// Most active claims a contradiction scan reads.
const contradictClaims = 150

// Contradict asks the model which active claims cannot both be true (spec 5,
// 8) and opens a conflict for each pair still active that has never been in
// one. It returns how many it opened.
func (l *Learner) Contradict(ctx context.Context) (int, error) {
	claims, err := l.o.Repo.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive},
		IncludeSuppressed: true, ExcludeSensitive: true, Limit: contradictClaims})
	if err != nil {
		return 0, fmt.Errorf("look for contradictions: %w", err)
	}
	if len(claims) < 2 {
		return 0, nil
	}
	m, err := l.model()
	if err != nil {
		return 0, fmt.Errorf("look for contradictions: %w", err)
	}
	names, err := l.entityNames(ctx)
	if err != nil {
		return 0, fmt.Errorf("look for contradictions: %w", err)
	}
	req := llm.ContradictRequest{Today: l.day(l.now()), Claims: make([]llm.ContradictItem, len(claims))}
	byRef := map[string]string{}
	for i, c := range claims {
		ref := fmt.Sprintf("k%d", i+1)
		subject := memory.SubjectUser
		if c.SubjectID != nil {
			subject = names[*c.SubjectID]
		}
		req.Claims[i] = llm.ContradictItem{Ref: ref, Key: c.Key, Subject: subject, Text: c.Text,
			Since: l.dayOf(c.ValidFrom, false), Until: l.dayOf(c.ValidTo, true)}
		byRef[ref] = c.ID
	}
	out, err := m.Contradict(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("look for contradictions: %w", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	known, err := l.o.Repo.Conflicts(ctx, "")
	if err != nil {
		return 0, fmt.Errorf("look for contradictions: %w", err)
	}
	pairs := map[[2]string]bool{}
	pair := func(a, b string) [2]string {
		if a > b {
			a, b = b, a
		}
		return [2]string{a, b}
	}
	for _, c := range known {
		pairs[pair(c.ClaimA, c.ClaimB)] = true
	}
	pl := l.newPlan(ctx)
	n := 0
	for _, c := range out.Conflicts {
		a, b := byRef[c.A], byRef[c.B]
		if a == "" || b == "" || a == b || pairs[pair(a, b)] {
			continue
		}
		ca, err := pl.claim(a)
		if err != nil && !isNotFound(err) {
			return 0, fmt.Errorf("look for contradictions: %w", err)
		}
		cb, errB := pl.claim(b)
		if errB != nil && !isNotFound(errB) {
			return 0, fmt.Errorf("look for contradictions: %w", errB)
		}
		if err != nil || errB != nil || ca.Status != model.ClaimActive || cb.Status != model.ClaimActive {
			continue
		}
		reason := c.Reason
		if strings.TrimSpace(reason) == "" {
			reason = "These cannot both be true."
		}
		if err := pl.openConflict(ca, cb, reason); err != nil {
			return 0, fmt.Errorf("look for contradictions: %w", err)
		}
		pairs[pair(a, b)] = true
		n++
	}
	if _, err := pl.commit(); err != nil {
		return 0, fmt.Errorf("look for contradictions: %w", err)
	}
	return n, nil
}

// entityNames maps live entities' ids to their names.
func (l *Learner) entityNames(ctx context.Context) (map[string]string, error) {
	entities, err := l.o.Repo.Entities(ctx, "")
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(entities))
	for _, e := range entities {
		names[e.ID] = e.Name
	}
	return names, nil
}

// Dedupe merges active claims that say the same thing (spec 5, 9): the same
// key and subject and the same words once normalized. The oldest is kept,
// the others are superseded by it, and their sources become its evidence.
// It returns how many it merged.
func (l *Learner) Dedupe(ctx context.Context) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	claims, err := l.o.Repo.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive},
		IncludeSuppressed: true})
	if err != nil {
		return 0, fmt.Errorf("merge duplicates: %w", err)
	}
	groups := map[string][]model.Claim{}
	var order []string
	for _, c := range claims {
		k := subjectKey(c.Key, c.SubjectID) + "\x00" + memory.Normalize(c.Text)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], c)
	}
	slices.Sort(order)
	pl := l.newPlan(ctx)
	n := 0
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		slices.SortFunc(g, func(a, b model.Claim) int {
			if c := a.RecordedAt.Compare(b.RecordedAt); c != 0 {
				return c
			}
			return strings.Compare(a.ID, b.ID)
		})
		keep := pl.track(g[0])
		for _, d := range g[1:] {
			dup := pl.track(d)
			pl.supersede(dup, keep.ID, nil)
			if d.SourceID != "" && d.SourceID != keep.SourceID {
				pl.link(keep.ID, linkKind(d.SourceKind), d.SourceID, "evidence")
			}
			links, err := l.o.Repo.LinksFrom(ctx, d.ID)
			if err != nil {
				return 0, fmt.Errorf("merge duplicates: %w", err)
			}
			for _, lk := range links {
				if lk.Relation == "evidence" && lk.ToID != keep.SourceID {
					pl.link(keep.ID, lk.ToKind, lk.ToID, "evidence")
				}
			}
			n++
		}
	}
	if _, err := pl.commit(); err != nil {
		return 0, fmt.Errorf("merge duplicates: %w", err)
	}
	return n, nil
}

// Insight cadence and what it reads (spec 5, 10).
const (
	insightEvery  = 7 * 24 * time.Hour
	insightKnown  = 100
	insightPrefix = "insight:"
)

// Insights asks the model, at most once a week, what the user's tracked data
// suggests about them, and proposes each insight as a candidate for the user
// to confirm. It returns how many claims it proposed.
func (l *Learner) Insights(ctx context.Context) (int, error) {
	l.insightMu.Lock()
	defer l.insightMu.Unlock()
	now := l.now()
	last, err := store.GetLocalTime(ctx, l.o.DB.SQL(), store.KeyMemoryLastInsight)
	if err == nil && now.Sub(last) < insightEvery {
		return 0, nil
	}
	if err != nil && !isNotFound(err) {
		return 0, fmt.Errorf("look for insights: %w", err)
	}
	var data []string
	if l.o.Data != nil {
		if data, err = l.o.Data(ctx); err != nil {
			return 0, fmt.Errorf("look for insights: %w", err)
		}
	}
	if len(data) == 0 {
		return 0, nil
	}
	m, err := l.model()
	if err != nil {
		return 0, fmt.Errorf("look for insights: %w", err)
	}
	claims, err := l.o.Repo.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive},
		ExcludeSensitive: true, Limit: insightKnown})
	if err != nil {
		return 0, fmt.Errorf("look for insights: %w", err)
	}
	known := make([]string, len(claims))
	for i, c := range claims {
		known[i] = c.Text
	}
	day := l.day(now)
	out, err := m.Insight(ctx, llm.InsightRequest{Today: day, Known: known, Data: data})
	if err != nil {
		return 0, fmt.Errorf("look for insights: %w", err)
	}
	cands := make([]Candidate, len(out.Insights))
	for i, in := range out.Insights {
		cands[i] = Candidate{Fact: llm.ExtractedFact{Key: in.Key, Subject: memory.SubjectUser, Value: in.Value,
			Text: in.Text, Quote: in.Basis, Confidence: in.Confidence}, SourceKind: model.SourceData,
			SourceID: insightPrefix + day, SourceText: in.Basis, Speaker: model.SpeakerGwen, Channel: channelData,
			At: now, Extractor: extractorOf(m)}
	}
	res, err := l.Propose(ctx, cands)
	if err != nil {
		return 0, fmt.Errorf("look for insights: %w", err)
	}
	if err := store.SetLocalTime(ctx, l.o.DB.SQL(), store.KeyMemoryLastInsight, now, now); err != nil {
		return res.Learned, fmt.Errorf("look for insights: %w", err)
	}
	return res.Learned, nil
}
