package learn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
)

// Candidate is a fact proposed for memory and the words it came from.
type Candidate struct {
	// Fact is what was proposed. Its Subject is "user", a ref of Entities, or
	// a new entity as "kind:Name"; its Updates a ref of Known or "". For an
	// insight, Quote is the basis.
	Fact llm.ExtractedFact
	// SourceKind and SourceID name the source: model.SourceMessage or
	// SourceEpisode and its id, or model.SourceData and "insight:<day>".
	SourceKind string
	SourceID   string
	// SourceText is the source's words, which must hold the quote.
	SourceText string
	// Speaker said the source: model.SpeakerUser, SpeakerGwen, or
	// SpeakerExternal.
	Speaker string
	Channel string
	At      time.Time // when the source was said
	// Context is the message before the source, for the entailment check.
	Context string
	// Extractor is recorded with the claim; "" is the model's.
	Extractor string
	// Trust, when set, is the claim's trust whatever the speaker: owner_edit
	// for words the user typed to be remembered.
	Trust string
	// Entities and Known are the refs of the request the fact answered: e1
	// to an entity, and k1 to a claim id.
	Entities map[string]model.Entity
	Known    map[string]string
}

// Why candidates are dropped, counted in the log.
const (
	dropKey       = "key"
	dropSubject   = "subject"
	dropValue     = "value"
	dropSecret    = "secret"
	dropQuote     = "quote"
	dropNegative  = "rejected before"
	dropEntailed  = "not supported"
	dropLowWeight = "low confidence"
)

// prep is a candidate that passed the checks that need no model, made into
// the claim it would commit.
type prep struct {
	cand  Candidate
	claim model.Claim
	// newEntity is the entity to create for a subject not known yet; claim's
	// SubjectID is set when it is committed.
	newEntity *model.Entity
	updates   string // the known claim it replaces, or ""
	needQuote bool   // the quote is found by the entailment check
	dropped   string
}

// Propose checks candidates and commits those that pass in one change (spec
// 5, 2): the key and subject, the value, the secret screen, the quote, trust,
// negative memory and duplicates, entailment, and supersession.
func (l *Learner) Propose(ctx context.Context, cands []Candidate) (Result, error) {
	res, _, err := l.propose(ctx, cands)
	return res, err
}

// propose is Propose that also returns, for each candidate that survived,
// the claim it made or corroborated.
func (l *Learner) propose(ctx context.Context, cands []Candidate) (Result, []string, error) {
	m, merr := l.model()
	verifying := merr == nil && l.verifyOn()
	dropped := map[string]int{}
	newEntities := map[string]*model.Entity{}
	var preps []*prep
	for _, c := range cands {
		p, err := l.prepare(ctx, c, m, verifying, newEntities)
		if err != nil {
			return Result{}, nil, err
		}
		if p.dropped != "" {
			dropped[p.dropped]++
			continue
		}
		preps = append(preps, p)
	}
	if verifying {
		if err := l.entail(ctx, m, preps); err != nil {
			return Result{}, nil, err
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	pl := l.newPlan(ctx)
	var ids []string
	for _, p := range preps {
		if p.dropped == "" {
			id, err := pl.propose(p)
			if err != nil {
				return Result{}, nil, err
			}
			if id != "" && !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		if p.dropped != "" {
			dropped[p.dropped]++
		}
	}
	if len(dropped) > 0 {
		slog.Debug("memory: candidates dropped", "reasons", dropped, "of", len(cands))
	}
	res, err := pl.commit()
	if err != nil {
		return Result{}, nil, fmt.Errorf("commit what was learned: %w", err)
	}
	return res, ids, nil
}

// prepare runs the checks of a candidate that need no write: key and
// subject, value, secrets, the quote, and trust.
func (l *Learner) prepare(ctx context.Context, c Candidate, m llm.Memory, verifying bool,
	newEntities map[string]*model.Entity) (*prep, error) {
	f := c.Fact
	p := &prep{cand: c}
	spec, ok := memory.Spec(f.Key)
	if !ok {
		p.dropped = dropKey
		return p, nil
	}
	// The subject must fit the key's namespace.
	var subjectID *string
	switch spec.Subject {
	case memory.SubjectUser:
		if !strings.EqualFold(strings.TrimSpace(f.Subject), memory.SubjectUser) {
			p.dropped = dropSubject
			return p, nil
		}
	default:
		e, isNew, err := l.entityOf(ctx, f.Subject, memory.EntityKindOf(f.Key), c.Entities, newEntities)
		if err != nil {
			return nil, err
		}
		if e == nil {
			p.dropped = dropSubject
			return p, nil
		}
		if isNew {
			p.newEntity = e
		} else {
			id := e.ID
			subjectID = &id
		}
	}
	value, ok := cleanValue(f.Value)
	text := memory.Cut(strings.Join(strings.Fields(f.Text), " "), 500)
	if !ok || text == "" {
		p.dropped = dropValue
		return p, nil
	}
	quote := strings.TrimSpace(f.Quote)
	if strings.EqualFold(f.Sensitivity, "secret") || hasSecret(string(value), text, quote) {
		p.dropped = dropSecret
		return p, nil
	}
	sensitivity := f.Sensitivity
	if memory.SensitivityRank(sensitivity) < 0 {
		sensitivity = model.SensitivityPersonal
	}
	if spec.Sensitive {
		sensitivity = memory.MoreSensitive(sensitivity, model.SensitivitySensitive)
	}
	trust := c.Trust
	if trust == "" {
		trust = trustOf(c)
	}
	// The quote must be the source's words, unless it is an insight's basis,
	// or the entailment check is to find it.
	switch {
	case c.SourceKind == model.SourceData:
	case quote == "":
		if !verifying || trust == model.TrustOwnerEdit {
			p.dropped = dropQuote
			return p, nil
		}
		p.needQuote = true
	case !memory.ContainsQuote(c.SourceText, quote):
		p.dropped = dropQuote
		return p, nil
	}
	extractor := c.Extractor
	if extractor == "" && m != nil {
		extractor = extractorOf(m)
	}
	confidence := f.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 0.5
	}
	if c.Trust == model.TrustOwnerEdit {
		confidence = 1
	}
	at := c.At.Truncate(time.Millisecond)
	if at.IsZero() {
		at = l.now()
	}
	channel := c.Channel
	if channel == "" {
		channel = model.ChannelChat
	}
	vf, vt := l.parseDay(f.ValidFrom, false), l.parseDay(f.ValidTo, true)
	if vf != nil && vt != nil && vt.Before(*vf) {
		vt = nil
	}
	p.claim = model.Claim{ID: model.NewID(), Key: f.Key, SubjectID: subjectID, Value: value, Text: text,
		Cardinality: spec.Cardinality, Status: statusFor(trust), Trust: trust, Sensitivity: sensitivity,
		Confidence: confidence, ValidFrom: vf, ValidTo: vt, OccurredAt: l.parseDay(f.OccurredAt, false),
		MentionedAt: at, SourceKind: c.SourceKind, SourceID: c.SourceID, Span: cutRunes(quote, 1000),
		Channel: channel, Extractor: cutRunes(extractor, 200)}
	if f.Updates != "" {
		p.updates = c.Known[f.Updates]
	}
	// Negative memory: the same fact from the same words was rejected or
	// forgotten. Checked again when committing, for a subject made then.
	if subjectID != nil || spec.Subject == memory.SubjectUser {
		p.claim.ContentHash = memory.ClaimHash(subjectID, f.Key, value)
		neg, err := l.rejectedBefore(ctx, &p.claim)
		if err != nil {
			return nil, err
		}
		if neg {
			p.dropped = dropNegative
		}
	}
	return p, nil
}

// trustOf is the trust of a candidate by where its words came from (spec
// section 1): the user's words, Gwen's, or someone else's.
func trustOf(c Candidate) string {
	switch {
	case c.SourceKind == model.SourceData:
		return model.TrustInferred
	case c.Speaker == model.SpeakerUser:
		return model.TrustOwnerSaid
	case c.Speaker == model.SpeakerGwen:
		return model.TrustInferred
	}
	return model.TrustExternal
}

// cleanValue checks a value: JSON, not null, and a string trimmed and not
// empty.
func cleanValue(raw json.RawMessage) ([]byte, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || !json.Valid(raw) || string(raw) == "null" {
		return nil, false
	}
	if s, ok := stringValue(raw); ok {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false
		}
		return jsonString(s), true
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return nil, false
	}
	return b.Bytes(), true
}

// cutRunes is s cut to at most n characters.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// entityOf resolves a fact's subject to an entity of kind: a ref of the
// request, a known entity of that name, or a new one (isNew). It is nil when
// the subject is not an entity of kind.
func (l *Learner) entityOf(ctx context.Context, subject, kind string, refs map[string]model.Entity,
	newEntities map[string]*model.Entity) (*model.Entity, bool, error) {
	subject = strings.TrimSpace(subject)
	if e, ok := refs[subject]; ok {
		if e.Kind != kind {
			return nil, false, nil
		}
		return &e, false, nil
	}
	k, name, ok := strings.Cut(subject, ":")
	k, name = strings.ToLower(strings.TrimSpace(k)), strings.Join(strings.Fields(name), " ")
	if !ok || name == "" || k != kind || !memory.ValidEntityKind(k) {
		return nil, false, nil
	}
	name = cutRunes(name, 200)
	found, err := l.findEntity(ctx, kind, name)
	if err != nil || found != nil {
		return found, false, err
	}
	nk := kind + "\x00" + strings.ToLower(name)
	if e, ok := newEntities[nk]; ok {
		return e, true, nil
	}
	e := &model.Entity{ID: model.NewID(), Kind: kind, Name: name, Aliases: []string{}}
	newEntities[nk] = e
	return e, true, nil
}

// findEntity returns the first live entity of kind named name, or nil.
func (l *Learner) findEntity(ctx context.Context, kind, name string) (*model.Entity, error) {
	got, err := l.o.Repo.FindEntities(ctx, name)
	if err != nil {
		return nil, err
	}
	for _, e := range got {
		if e.Kind == kind {
			return &e, nil
		}
	}
	return nil, nil
}

// rejectedBefore reports whether a claim with c's hash was rejected or
// forgotten and came from the same words: the same source id, or, for an
// insight, any insight.
func (l *Learner) rejectedBefore(ctx context.Context, c *model.Claim) (bool, error) {
	same, err := l.o.Repo.ClaimsByHash(ctx, c.ContentHash)
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(same, func(o model.Claim) bool {
		gone := o.Status == model.ClaimRejected || o.DeletedAt != nil
		from := o.SourceID == c.SourceID || c.SourceKind == model.SourceData && o.SourceKind == model.SourceData
		return gone && from
	}), nil
}

// Most characters of a verification's context.
const verifyContext = 2000

// entail asks the model, in one call per llm.MaxVerifyChecks, whether each
// candidate's quote supports it (spec 5, 2.7). The user's own edits are not
// checked. "no" drops a candidate; "partly" lowers its confidence, and drops
// it below 0.3. A check that had no quote takes the words the model found,
// which must be the source's.
func (l *Learner) entail(ctx context.Context, m llm.Memory, preps []*prep) error {
	var todo []*prep
	for _, p := range preps {
		if p.claim.Trust != model.TrustOwnerEdit {
			todo = append(todo, p)
		}
	}
	for start := 0; start < len(todo); start += llm.MaxVerifyChecks {
		batch := todo[start:min(start+llm.MaxVerifyChecks, len(todo))]
		req := llm.VerifyRequest{Checks: make([]llm.VerifyCheck, len(batch))}
		for i, p := range batch {
			speaker := p.cand.Speaker
			if speaker == model.SpeakerExternal || speaker == "" {
				speaker = "document"
			}
			req.Checks[i] = llm.VerifyCheck{Ref: fmt.Sprintf("v%d", i+1), Claim: p.claim.Text, Quote: p.claim.Span,
				Speaker: speaker, Context: contextAround(p.cand.Context, p.cand.SourceText, p.claim.Span)}
		}
		out, err := m.Verify(ctx, req)
		if err != nil {
			return fmt.Errorf("check what was learned: %w", err)
		}
		verdicts := map[string]llm.Verdict{}
		for _, v := range out.Verdicts {
			verdicts[v.Ref] = v
		}
		for i, p := range batch {
			v, ok := verdicts[fmt.Sprintf("v%d", i+1)]
			switch {
			case !ok || v.Supported == "no":
				p.dropped = dropEntailed
				continue
			case v.Supported == "partly":
				p.claim.Confidence *= 0.6
				if p.claim.Confidence < 0.3 {
					p.dropped = dropLowWeight
					continue
				}
			}
			if p.needQuote {
				if !memory.ContainsQuote(p.cand.SourceText, v.Quote) || hasSecret(v.Quote) {
					p.dropped = dropQuote
					continue
				}
				p.claim.Span = cutRunes(strings.TrimSpace(v.Quote), 1000)
			}
		}
	}
	return nil
}

// contextAround is the words a verifier reads: the previous message and the
// source, cut to verifyContext characters around the quote.
func contextAround(prev, source, quote string) string {
	text := source
	if prev = strings.TrimSpace(prev); prev != "" {
		text = prev + "\n\n" + source
	}
	r := []rune(text)
	if len(r) <= verifyContext {
		return text
	}
	at := 0
	if q := strings.TrimSpace(quote); q != "" {
		if i := strings.Index(strings.ToLower(text), strings.ToLower(q)); i >= 0 {
			at = utf8.RuneCountInString(text[:i])
		}
	}
	from := max(0, min(at-verifyContext/2, len(r)-verifyContext))
	return string(r[from : from+verifyContext])
}

// propose decides a prepared candidate in the plan (spec 5, 2.6 and 2.8)
// and returns the claim it made or corroborated, or "" when it was dropped.
func (pl *plan) propose(p *prep) (string, error) {
	c := p.claim
	if p.newEntity != nil {
		// Someone may have made the entity since the candidate was prepared.
		e, err := pl.l.findEntity(pl.ctx, p.newEntity.Kind, p.newEntity.Name)
		if err != nil {
			return "", err
		}
		if e == nil {
			e = p.newEntity
			pl.addEntity(*e)
		}
		id := e.ID
		c.SubjectID = &id
	}
	c.ContentHash = memory.ClaimHash(c.SubjectID, c.Key, c.Value)
	if neg, err := pl.l.rejectedBefore(pl.ctx, &c); err != nil || neg {
		if neg {
			p.dropped = dropNegative
		}
		return "", err
	}
	// The same fact held already: the new words are more evidence for it,
	// and the user's own words make a candidate active.
	same, err := pl.sameFact(c)
	if err != nil {
		return "", err
	}
	if same != nil {
		if same.SourceID != c.SourceID {
			pl.link(same.ID, linkKind(c.SourceKind), c.SourceID, "evidence")
		}
		if (same.Status == model.ClaimCandidate || same.Status == model.ClaimQuarantined) &&
			(c.Trust == model.TrustOwnerEdit || c.Trust == model.TrustOwnerSaid) {
			// The store's claim updates cannot change trust (store.ClaimUpdate
			// has no field for it), so the claim keeps the trust it had.
			same.Confidence = max(same.Confidence, c.Confidence)
			same.Trust = c.Trust
			pl.setStatus(same, model.ClaimActive, "claim.activate", "by_trust", c.Trust)
			if err := pl.settle(same); err != nil {
				return "", err
			}
		}
		return same.ID, nil
	}
	nc := &c
	pl.add(nc)
	if c.SubjectID != nil {
		pl.link(c.ID, "entity", *c.SubjectID, "about")
	}
	if nc.Status != model.ClaimActive {
		return nc.ID, nil
	}
	if c.Cardinality == model.CardinalityOne {
		return nc.ID, pl.settle(nc)
	}
	// A many-valued fact that says it replaces a known claim of the same key
	// and subject supersedes it.
	if p.updates != "" {
		old, err := pl.claim(p.updates)
		if err != nil {
			if isNotFound(err) {
				return nc.ID, nil
			}
			return "", err
		}
		if old.Key == c.Key && sameSubject(old.SubjectID, c.SubjectID) && old.Status == model.ClaimActive &&
			old.Cardinality == model.CardinalityMany && old.ID != nc.ID {
			pl.supersede(old, nc.ID, nil)
		}
	}
	return nc.ID, nil
}

// sameFact returns the live claim, active, candidate, or quarantined, with
// c's hash, in the store or added by this plan, or nil. An active one is
// preferred.
func (pl *plan) sameFact(c model.Claim) (*model.Claim, error) {
	for _, a := range pl.added {
		if a.ContentHash == c.ContentHash && live(*a) {
			return a, nil
		}
	}
	got, err := pl.l.o.Repo.ClaimsByHash(pl.ctx, c.ContentHash)
	if err != nil {
		return nil, err
	}
	var best *model.Claim
	for _, o := range got {
		if t, ok := pl.loaded[o.ID]; ok {
			o = *t
		}
		if !live(o) {
			continue
		}
		if best == nil || o.Status == model.ClaimActive && best.Status != model.ClaimActive {
			cp := o
			best = &cp
		}
	}
	if best == nil {
		return nil, nil
	}
	return pl.track(*best), nil
}
