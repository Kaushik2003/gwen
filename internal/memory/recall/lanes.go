package recall

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// Hit kinds.
const (
	kindClaim    = "claim"
	kindMessage  = "message"
	kindEpisode  = "episode"
	kindResource = "resource"
	kindData     = "data"
)

// liveStatuses are the statuses a claim may be recalled in.
var liveStatuses = []string{model.ClaimActive, model.ClaimSuperseded}

func claimCand(c model.Claim) cand {
	h := memory.Hit{Ref: memory.Ref(memory.RefClaim, c.ID), Kind: kindClaim, Key: c.Key, Text: c.Text,
		Value: json.RawMessage(c.Value), At: c.RecordedAt, ValidFrom: c.ValidFrom, ValidTo: c.ValidTo,
		Trust: c.Trust, Sensitivity: c.Sensitivity}
	return cand{hit: h, claim: &c}
}

func msgCand(m model.ChatHit) cand {
	trust := model.TrustOwnerSaid
	if m.Role != model.ChatUser {
		trust = model.TrustInferred
	}
	h := memory.Hit{Ref: memory.Ref(memory.RefMessage, m.ID), Kind: kindMessage, Text: memory.Cut(m.Text, hitChars),
		At: m.At, Trust: trust}
	return cand{hit: h, msg: &m}
}

func epCand(e model.Episode) cand {
	h := memory.Hit{Ref: memory.Ref(memory.RefEpisode, e.ID), Kind: kindEpisode, Text: memory.Cut(e.Body, hitChars),
		At: e.At, Trust: episodeTrust(e), Sensitivity: e.Sensitivity}
	if e.Kind == model.EpisodeResource {
		h.Kind = kindResource
	}
	return cand{hit: h, ep: &e}
}

// episodeTrust is how far an episode's words are the user's own.
func episodeTrust(e model.Episode) string {
	switch {
	case e.Kind == model.EpisodeResource || e.Speaker == model.SpeakerExternal:
		return model.TrustExternal
	case e.Speaker == model.SpeakerGwen:
		return model.TrustInferred
	}
	return model.TrustOwnerEdit
}

// namedEntities are the live entities whose name or an alias text names as
// whole words, ignoring case.
func (r *Router) namedEntities(ctx context.Context, text string) ([]model.Entity, error) {
	all, err := r.o.Repo.Entities(ctx, "")
	if err != nil {
		return nil, err
	}
	var out []model.Entity
	for _, e := range all {
		if mentions(text, e.Name) || slices.ContainsFunc(e.Aliases, func(a string) bool { return mentions(text, a) }) {
			out = append(out, e)
		}
	}
	return out, nil
}

// claimsLane finds claims by key, from the words that name one, then by
// words. Key hits rank first.
func (r *Router) claimsLane(ctx context.Context, s query, ents func() ([]model.Entity, error)) ([]cand, error) {
	n := s.Budget.Candidates
	var out []cand
	seen := map[string]bool{}
	add := func(cs ...model.Claim) {
		for _, c := range cs {
			if !seen[c.ID] && len(out) < n {
				seen[c.ID] = true
				out = append(out, claimCand(c))
			}
		}
	}
	byKey := func(key string, subject *string) error {
		cs, err := r.o.Repo.ClaimsByKey(ctx, key, subject, liveStatuses...)
		add(cs...)
		return err
	}
	for _, key := range userKeys(s.Text) {
		if err := byKey(key, nil); err != nil {
			return nil, err
		}
	}
	if birthdayRE.MatchString(s.Text) {
		named, err := ents()
		if err != nil {
			return nil, err
		}
		people := 0
		for _, e := range named {
			if e.Kind == memory.EntityPerson {
				people++
				if err := byKey("person.birthday", &e.ID); err != nil {
					return nil, err
				}
			}
		}
		if people == 0 {
			if err := byKey("user.birthday", nil); err != nil {
				return nil, err
			}
		}
	}
	hits, err := r.o.Repo.SearchClaims(ctx, s.Text, n)
	if err != nil {
		return nil, err
	}
	for _, h := range hits {
		add(h.Claim)
	}
	return out, nil
}

// entityLane finds the claims about the people and things the query names.
func (r *Router) entityLane(ctx context.Context, s query, ents func() ([]model.Entity, error)) ([]cand, error) {
	named, err := ents()
	if err != nil {
		return nil, err
	}
	n := s.Budget.Candidates
	var out []cand
	for _, e := range named {
		cs, err := r.o.Repo.Claims(ctx, store.ClaimFilter{Statuses: liveStatuses, Subject: &e.ID, SubjectSet: true,
			Limit: n})
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			if len(out) < n {
				out = append(out, claimCand(c))
			}
		}
	}
	return out, nil
}

// excluded are the ids of the refs of one kind in a query's Exclude.
func excluded(s query, kind string) []string {
	var ids []string
	for _, ref := range s.Exclude {
		if k, id, ok := memory.ParseRef(ref); ok && k == kind {
			ids = append(ids, id)
		}
	}
	return ids
}

// chatsLane finds chat messages by words.
func (r *Router) chatsLane(ctx context.Context, s query) ([]cand, error) {
	hits, err := r.o.Chats.Search(ctx, s.Text, s.Budget.Candidates, excluded(s, memory.RefMessage))
	if err != nil {
		return nil, err
	}
	out := make([]cand, 0, len(hits))
	for _, h := range hits {
		out = append(out, msgCand(h))
	}
	return out, nil
}

// episodesLane finds episodes by words.
func (r *Router) episodesLane(ctx context.Context, s query) ([]cand, error) {
	hits, err := r.o.Repo.SearchEpisodes(ctx, s.Text, s.Budget.Candidates+len(s.exclude))
	if err != nil {
		return nil, err
	}
	var out []cand
	for _, h := range hits {
		c := epCand(h.Episode)
		if !s.exclude[c.hit.Ref] && len(out) < s.Budget.Candidates {
			out = append(out, c)
		}
	}
	return out, nil
}

// timeLane finds what was said, and what was learned, in the query's
// window: by the query's words when it has some, else newest first.
func (r *Router) timeLane(ctx context.Context, s query) ([]cand, error) {
	w := s.window
	msgs, err := r.o.Repo.MessagesBetween(ctx, w.From, w.To, timeCap)
	if err != nil {
		return nil, err
	}
	eps, err := r.o.Repo.EpisodesBetween(ctx, w.From, w.To, timeCap)
	if err != nil {
		return nil, err
	}
	claims, err := r.claimsRecorded(ctx, w)
	if err != nil {
		return nil, err
	}
	var out []cand
	for _, m := range msgs {
		out = append(out, msgCand(m))
	}
	for _, e := range eps {
		out = append(out, epCand(e))
	}
	for _, c := range claims {
		out = append(out, claimCand(c))
	}
	qw := words(s.Text)
	score := make(map[string]int, len(out))
	for _, c := range out {
		score[c.hit.Ref] = overlap(qw, c.hit.Text)
	}
	slices.SortStableFunc(out, func(a, b cand) int {
		if d := cmp.Compare(score[b.hit.Ref], score[a.hit.Ref]); d != 0 {
			return d
		}
		return b.hit.At.Compare(a.hit.At)
	})
	return out[:min(len(out), s.Budget.Candidates)], nil
}

// claimsRecorded are up to timeCap live claims recorded in w, newest first.
func (r *Router) claimsRecorded(ctx context.Context, w *memory.TimeRange) ([]model.Claim, error) {
	const page = 200
	var out []model.Claim
	for offset := 0; ; offset += page {
		cs, err := r.o.Repo.Claims(ctx, store.ClaimFilter{Statuses: liveStatuses, Limit: page, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			if c.RecordedAt.Before(w.From) {
				return out, nil
			}
			if c.RecordedAt.Before(w.To) {
				out = append(out, c)
				if len(out) == timeCap {
					return out, nil
				}
			}
		}
		if len(cs) < page {
			return out, nil
		}
	}
}

// dataLane asks the tracked data.
func (r *Router) dataLane(ctx context.Context, s query) ([]cand, error) {
	hits, err := r.o.Data.Search(ctx, s.Query, s.window)
	if err != nil {
		return nil, err
	}
	out := make([]cand, 0, len(hits))
	for _, h := range hits {
		if h.Kind == "" {
			h.Kind = kindData
		}
		if h.Source == "" {
			h.Source = "your tracked data"
		}
		if h.Trust == "" {
			h.Trust = model.TrustObserved
		}
		out = append(out, cand{hit: h})
	}
	return out, nil
}

// extraLane asks an extra lane and resolves its refs: claims, messages, and
// episodes are read from the store, so the filters see them as they are now;
// documents and data keep the lane's words.
func (r *Router) extraLane(ctx context.Context, s query, l memory.Lane) ([]cand, error) {
	q := s.Query
	q.Budget.Candidates = s.Budget.Candidates
	if q.AsOf.IsZero() {
		q.AsOf = s.asOf
	}
	if q.Window == nil {
		q.Window = s.window
	}
	if q.Intent == "" {
		q.Intent = s.intent
	}
	hits, err := l.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []cand
	for _, h := range hits {
		c, ok, err := r.resolveHit(ctx, h)
		if err != nil {
			return nil, err
		}
		if ok && !s.exclude[c.hit.Ref] {
			out = append(out, c)
		}
	}
	return out, nil
}

// resolveHit reads what a lane's hit names; ok is false when it is gone or
// is not a ref.
func (r *Router) resolveHit(ctx context.Context, h memory.Hit) (c cand, ok bool, err error) {
	kind, id, valid := memory.ParseRef(h.Ref)
	if !valid {
		return cand{}, false, nil
	}
	switch kind {
	case memory.RefClaim:
		cl, err := r.o.Repo.Claim(ctx, id)
		if err != nil {
			return cand{}, false, ignoreNotFound(err)
		}
		return claimCand(cl), true, nil
	case memory.RefMessage:
		m, err := r.o.Repo.Message(ctx, id)
		if err != nil {
			return cand{}, false, ignoreNotFound(err)
		}
		return msgCand(m), true, nil
	case memory.RefEpisode:
		e, err := r.o.Repo.Episode(ctx, id)
		if err != nil {
			return cand{}, false, ignoreNotFound(err)
		}
		return epCand(e), true, nil
	case memory.RefResource:
		h.Kind = kindResource
		if h.Trust == "" {
			h.Trust = model.TrustExternal
		}
		if r.o.State != nil {
			res, err := r.o.State.Resource(ctx, id)
			if err != nil {
				return cand{}, false, ignoreNotFound(err)
			}
			if h.Source == "" && res.Title != "" {
				h.Source = documentSource(res.Title)
			}
		}
		h.Text = memory.Cut(h.Text, hitChars)
		return cand{hit: h}, true, nil
	case memory.RefData:
		if h.Kind == "" {
			h.Kind = kindData
		}
		return cand{hit: h}, true, nil
	}
	return cand{}, false, nil
}

// ignoreNotFound is nil for ErrNotFound, and err otherwise.
func ignoreNotFound(err error) error {
	if isNotFound(err) {
		return nil
	}
	return err
}
