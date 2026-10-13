package learn

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// noSecrets is the error message for words that look like a secret.
const noSecrets = "that looks like a password, a card or account number, or a key, and Gwen never keeps those"

// Most characters of a note kept as it is when no fact is read from it.
const rememberChars = 300

// Remember learns from words the user typed to be remembered (spec 5, 3): an
// episode holds them, the model reads them as meant, and what it finds is
// committed as the user's own edit. When there is no model, or nothing it
// finds survives, the words become one user.fact claim. Words that look like
// a secret are refused and not stored.
func (l *Learner) Remember(ctx context.Context, text, channel string) ([]model.Claim, error) {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return nil, store.FailField(store.ErrInvalid, "text", "there is nothing to remember")
	case hasSecret(text):
		return nil, store.FailField(store.ErrInvalid, "text", noSecrets)
	}
	switch channel {
	case "":
		channel = model.ChannelUI
	case model.ChannelUI, model.ChannelCLI:
	default:
		return nil, store.FailField(store.ErrInvalid, "channel", "words to remember come from the ui or the cli, not %q",
			channel)
	}
	now := l.now()
	ep, err := l.o.Repo.AddEpisode(ctx, model.Episode{Kind: model.EpisodeNote, Channel: channel,
		Speaker: model.SpeakerUser, Body: text, At: now, Sensitivity: model.SensitivityPersonal})
	if err != nil {
		return nil, fmt.Errorf("remember: %w", err)
	}
	base := Candidate{SourceKind: model.SourceEpisode, SourceID: ep.ID, SourceText: text, Speaker: model.SpeakerUser,
		Channel: channel, At: now, Trust: model.TrustOwnerEdit}
	var ids []string
	if m, err := l.model(); err == nil {
		req, refs, err := l.baseRequest(ctx)
		if err != nil {
			return nil, fmt.Errorf("remember: %w", err)
		}
		req.Explicit = true
		req.Items = []llm.ExtractItem{{Ref: "i1", Speaker: "user", At: l.localStamp(now), Text: memory.Cut(text, itemChars)}}
		out, err := m.Extract(ctx, req)
		if err != nil {
			slog.Warn("memory: the model could not read what to remember, so it is kept as typed", "err", err)
		} else {
			var cands []Candidate
			for _, f := range out.Facts {
				c := base
				c.Fact, c.Extractor, c.Entities, c.Known = f, extractorOf(m), refs.entities, refs.known
				cands = append(cands, c)
			}
			if _, ids, err = l.propose(ctx, cands); err != nil {
				return nil, fmt.Errorf("remember: %w", err)
			}
		}
	}
	if len(ids) == 0 {
		cut := memory.Cut(text, rememberChars)
		c := base
		c.Fact = llm.ExtractedFact{Key: "user.fact", Subject: memory.SubjectUser, Value: jsonString(cut), Text: cut,
			Quote: strings.TrimSuffix(cut, "…"), Confidence: 1}
		c.Extractor = extractorUser
		if _, ids, err = l.propose(ctx, []Candidate{c}); err != nil {
			return nil, fmt.Errorf("remember: %w", err)
		}
	}
	claims := make([]model.Claim, 0, len(ids))
	for _, id := range ids {
		c, err := l.o.Repo.Claim(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("remember: %w", err)
		}
		claims = append(claims, c)
	}
	return claims, nil
}

// Edit replaces a claim with the user's correction (spec 5, 4): a new claim
// of the same key and subject, the user's own edit, whose source is an edit
// episode holding both wordings. The old claim is superseded by it, so the
// correction keeps history.
func (l *Learner) Edit(ctx context.Context, id string, req memory.EditRequest) (model.Claim, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pl := l.newPlan(ctx)
	old, err := pl.claim(id)
	if err != nil {
		return model.Claim{}, fmt.Errorf("edit claim: %w", err)
	}
	if old.Status == model.ClaimSuperseded || old.Status == model.ClaimRejected {
		return model.Claim{}, store.Fail(store.ErrConflict, "only a claim that still holds can be corrected")
	}
	label := old.Key
	if spec, ok := memory.Spec(old.Key); ok {
		label = spec.Label
	}
	var value []byte
	var text string
	if s, isString := stringValue(old.Value); old.Cardinality == model.CardinalityOne && isString && req.Value != nil {
		v := strings.Join(strings.Fields(*req.Value), " ")
		if v == "" {
			return model.Claim{}, store.FailField(store.ErrInvalid, "value", "the corrected value is empty")
		}
		value = jsonString(v)
		text = label + " " + v
		if s = strings.TrimSpace(s); s != "" {
			if loc := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(s)).FindStringIndex(old.Text); loc != nil {
				text = old.Text[:loc[0]] + v + old.Text[loc[1]:]
			}
		}
	} else {
		text = strings.Join(strings.Fields(req.Text), " ")
		if text == "" {
			return model.Claim{}, store.FailField(store.ErrInvalid, "text", "the corrected words are empty")
		}
		value = jsonString(text)
	}
	text = memory.Cut(text, 500)
	if hasSecret(text, string(value)) {
		return model.Claim{}, store.FailField(store.ErrInvalid, "text", noSecrets)
	}
	vf, vt := old.ValidFrom, old.ValidTo
	if req.ValidFrom != nil {
		if vf = l.parseDay(*req.ValidFrom, false); vf == nil && strings.TrimSpace(*req.ValidFrom) != "" {
			return model.Claim{}, store.FailField(store.ErrInvalid, "valid_from", "a date is YYYY-MM-DD")
		}
	}
	if req.ValidTo != nil {
		if vt = l.parseDay(*req.ValidTo, true); vt == nil && strings.TrimSpace(*req.ValidTo) != "" {
			return model.Claim{}, store.FailField(store.ErrInvalid, "valid_to", "a date is YYYY-MM-DD")
		}
	}
	if vf != nil && vt != nil && vt.Before(*vf) {
		return model.Claim{}, store.FailField(store.ErrInvalid, "valid_to", "the end comes before the start")
	}
	now := l.now()
	ep, err := l.o.Repo.AddEpisode(ctx, model.Episode{Kind: model.EpisodeEdit, Channel: model.ChannelUI,
		Speaker: model.SpeakerUser, Body: `"` + old.Text + `" -> "` + text + `"`, At: now,
		Sensitivity: old.Sensitivity})
	if err != nil {
		return model.Claim{}, fmt.Errorf("edit claim: %w", err)
	}
	nc := &model.Claim{ID: model.NewID(), Key: old.Key, SubjectID: old.SubjectID, Value: value, Text: text,
		Cardinality: old.Cardinality, Status: model.ClaimActive, Trust: model.TrustOwnerEdit,
		Sensitivity: old.Sensitivity, Confidence: 1, ValidFrom: vf, ValidTo: vt, OccurredAt: old.OccurredAt,
		MentionedAt: now, SourceKind: model.SourceEpisode, SourceID: ep.ID, Span: cutRunes(text, 1000),
		Channel: model.ChannelUI, Extractor: extractorUser, ContentHash: memory.ClaimHash(old.SubjectID, old.Key, value)}
	pl.add(nc)
	if nc.SubjectID != nil {
		pl.link(nc.ID, "entity", *nc.SubjectID, "about")
	}
	pl.supersede(old, nc.ID, nil)
	if _, err := pl.commit(); err != nil {
		// The episode must not be learned from as a note of its own.
		if eraseErr := l.o.Repo.EraseEpisode(ctx, ep.ID, store.LedgerItem{}); eraseErr != nil {
			slog.Warn("memory: an edit that failed left its episode", "episode", ep.ID, "err", eraseErr)
		}
		return model.Claim{}, fmt.Errorf("edit claim: %w", err)
	}
	return l.o.Repo.Claim(ctx, nc.ID)
}

// Confirm makes a candidate or quarantined claim active on the user's word,
// then applies one-valued supersession (spec 5, 5). A claim already active
// is returned as it is.
func (l *Learner) Confirm(ctx context.Context, id string) (model.Claim, error) {
	return l.change(ctx, id, "confirm claim", func(pl *plan, c *model.Claim) error {
		switch c.Status {
		case model.ClaimActive:
			return nil
		case model.ClaimCandidate, model.ClaimQuarantined:
		default:
			return store.Fail(store.ErrConflict, "only a claim waiting for the user's word can be confirmed")
		}
		// store.ClaimUpdate cannot change trust, so the claim keeps its own.
		c.Trust = model.TrustOwnerEdit
		pl.setStatus(c, model.ClaimActive, "claim.confirm", "by_trust", model.TrustOwnerEdit)
		return pl.settle(c)
	})
}

// Reject marks a claim not true, whatever its status, and dismisses the open
// conflicts it is in. Its hash keeps the same words from teaching it again.
func (l *Learner) Reject(ctx context.Context, id string) (model.Claim, error) {
	return l.change(ctx, id, "reject claim", func(pl *plan, c *model.Claim) error {
		if c.Status == model.ClaimRejected {
			return nil
		}
		pl.setStatus(c, model.ClaimRejected, "claim.reject")
		return pl.dismissConflicts(c.ID, "claim rejected")
	})
}

// Suppress stops Gwen bringing a claim up (on), or lets her again.
func (l *Learner) Suppress(ctx context.Context, id string, on bool) (model.Claim, error) {
	return l.change(ctx, id, "hide claim", func(pl *plan, c *model.Claim) error {
		if c.Suppressed == on {
			return nil
		}
		c.Suppressed = on
		action := "claim.suppress"
		if !on {
			action = "claim.unsuppress"
		}
		pl.ledger = append(pl.ledger, ledgerRow(action, c))
		return nil
	})
}

// change runs fn on a claim in a plan and commits it, returning the claim as
// stored after.
func (l *Learner) change(ctx context.Context, id, what string,
	fn func(*plan, *model.Claim) error) (model.Claim, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	pl := l.newPlan(ctx)
	c, err := pl.claim(id)
	if err != nil {
		return model.Claim{}, fmt.Errorf("%s: %w", what, err)
	}
	if err := fn(pl, c); err != nil {
		return model.Claim{}, fmt.Errorf("%s: %w", what, err)
	}
	if _, err := pl.commit(); err != nil {
		return model.Claim{}, fmt.Errorf("%s: %w", what, err)
	}
	return l.o.Repo.Claim(ctx, id)
}

// Forget erases a claim (spec 5, 5): the store makes it a tombstone, takes
// its words out of notes, profile cards, conflicts, and observation logs,
// and dismisses its open conflicts. It returns the claim as it was.
func (l *Learner) Forget(ctx context.Context, id string) (model.Claim, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	c, err := l.o.Repo.Claim(ctx, id)
	if err != nil {
		return model.Claim{}, fmt.Errorf("forget claim: %w", err)
	}
	err = l.o.Repo.Apply(ctx, store.MemoryChange{
		Updates: []store.ClaimUpdate{{ID: id, Forget: true}},
		Ledger: []store.LedgerItem{{Data: map[string]any{"action": "claim.forget", "claim": id,
			"hash": c.ContentHash}}},
		Jobs: []store.NewJob{projectJob(id), filesJob()},
	})
	if err != nil {
		return model.Claim{}, fmt.Errorf("forget claim: %w", err)
	}
	return c, nil
}

// Resolve settles an open conflict (spec 5, 6): keep_a rejects B, keep_b
// rejects A, both keeps both, and neither rejects both. A rejected claim's
// other open conflicts are dismissed.
func (l *Learner) Resolve(ctx context.Context, conflictID, resolution string) (model.Conflict, error) {
	switch resolution {
	case model.ResolveKeepA, model.ResolveKeepB, model.ResolveBoth, model.ResolveNeither:
	default:
		return model.Conflict{}, store.FailField(store.ErrInvalid, "resolution",
			"a conflict is resolved keep_a, keep_b, both, or neither, not %q", resolution)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	cf, err := l.o.Repo.Conflict(ctx, conflictID)
	if err != nil {
		return model.Conflict{}, fmt.Errorf("resolve conflict: %w", err)
	}
	if cf.Status != model.ConflictOpen {
		return model.Conflict{}, store.Fail(store.ErrConflict, "this conflict is settled already")
	}
	pl := l.newPlan(ctx)
	r := resolution
	pl.conflictUpdates = append(pl.conflictUpdates, store.ConflictUpdate{ID: cf.ID, Status: model.ConflictResolved,
		Resolution: &r})
	pl.ledger = append(pl.ledger, store.LedgerItem{Data: map[string]any{"action": "conflict.resolve",
		"conflict": cf.ID, "resolution": r}})
	reject := func(id string) error {
		c, err := pl.claim(id)
		if isNotFound(err) {
			return nil
		}
		if err != nil || c.Status == model.ClaimRejected {
			return err
		}
		pl.setStatus(c, model.ClaimRejected, "claim.reject", "conflict", cf.ID)
		return pl.dismissConflicts(c.ID, "claim rejected")
	}
	var rejected []string
	switch resolution {
	case model.ResolveKeepA:
		rejected = []string{cf.ClaimB}
	case model.ResolveKeepB:
		rejected = []string{cf.ClaimA}
	case model.ResolveNeither:
		rejected = []string{cf.ClaimA, cf.ClaimB}
	}
	for _, id := range rejected {
		if err := reject(id); err != nil {
			return model.Conflict{}, fmt.Errorf("resolve conflict: %w", err)
		}
	}
	if _, err := pl.commit(); err != nil {
		return model.Conflict{}, fmt.Errorf("resolve conflict: %w", err)
	}
	return l.o.Repo.Conflict(ctx, conflictID)
}
