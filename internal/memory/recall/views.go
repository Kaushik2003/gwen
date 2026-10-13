package recall

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

const (
	// refChars is how much of a message or an episode a resolved ref shows.
	refChars = 120
	// quoteChars is how much of an episode an explanation quotes.
	quoteChars = 2000
	// contextBefore and contextAfter are how many messages around a source
	// an explanation shows.
	contextBefore = 3
	contextAfter  = 2
)

// Views returns each claim with its subject's name, its key's label, its
// source in words, the chat it came from, and whether it is disputed.
func (r *Router) Views(ctx context.Context, claims []model.Claim) ([]memory.ClaimView, error) {
	d := r.describer(ctx, r.o.Clock.Now())
	out := make([]memory.ClaimView, 0, len(claims))
	for _, c := range claims {
		conflicts, err := r.o.Repo.OpenConflictsOf(ctx, c.ID)
		if err != nil {
			return nil, err
		}
		v, err := d.view(c, conflicts)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// Get returns the claim of key about subject (nil is the user) that held at
// asOf (zero is now), or store.ErrNotFound. Only claims that may be recalled
// count: active ones, or superseded ones at a past asOf; never suppressed
// ones.
func (r *Router) Get(ctx context.Context, key string, subjectID *string, asOf time.Time) (model.Claim, error) {
	now := r.o.Clock.Now()
	if asOf.IsZero() {
		asOf = now
	}
	past := asOf.Before(now.Add(-asOfSlack))
	cs, err := r.o.Repo.ClaimsByKey(ctx, key, subjectID, liveStatuses...)
	if err != nil {
		return model.Claim{}, err
	}
	for _, c := range cs {
		if recallable(c, asOf, past) {
			return c, nil
		}
	}
	return model.Claim{}, store.Fail(store.ErrNotFound, "nothing is remembered for %s then", key)
}

// badRef is the error of a ref that is not one.
func badRef(ref string) error {
	return store.FailField(store.ErrInvalid, "ref", "%q is not a ref", ref)
}

// Explain says where a ref came from and what became of it: for a claim, the
// claim, the words it rests on, the messages around them or the episode,
// the claims of the same key over time, and its links; for a message, its
// words and the messages around it; for an episode or a document, the
// episode. A forgotten claim is store.ErrNotFound.
func (r *Router) Explain(ctx context.Context, ref string) (memory.Explanation, error) {
	kind, id, ok := memory.ParseRef(ref)
	if !ok {
		return memory.Explanation{}, badRef(ref)
	}
	d := r.describer(ctx, r.o.Clock.Now())
	ex := memory.Explanation{Ref: ref}
	switch kind {
	case memory.RefClaim:
		c, err := r.o.Repo.Claim(ctx, id)
		if err != nil {
			return memory.Explanation{}, err
		}
		views, err := r.Views(ctx, []model.Claim{c})
		if err != nil {
			return memory.Explanation{}, err
		}
		ex.Claim, ex.Quote = &views[0], c.Span
		switch c.SourceKind {
		case model.SourceMessage:
			if ex.Context, err = r.around(ctx, c.SourceID); err != nil {
				return memory.Explanation{}, err
			}
		case model.SourceEpisode:
			if ex.Episode, err = d.episode(c.SourceID); err != nil {
				return memory.Explanation{}, err
			}
		}
		history, err := r.o.Repo.ClaimsByKey(ctx, c.Key, c.SubjectID)
		if err != nil {
			return memory.Explanation{}, err
		}
		if ex.History, err = r.Views(ctx, history); err != nil {
			return memory.Explanation{}, err
		}
		if ex.Links, err = r.o.Repo.LinksFrom(ctx, c.ID); err != nil {
			return memory.Explanation{}, err
		}
	case memory.RefMessage:
		m, err := r.o.Repo.Message(ctx, id)
		if err != nil {
			return memory.Explanation{}, err
		}
		ex.Quote = m.Text
		if ex.Context, err = r.around(ctx, id); err != nil {
			return memory.Explanation{}, err
		}
		if ex.Links, err = r.o.Repo.LinksTo(ctx, "message", id); err != nil {
			return memory.Explanation{}, err
		}
	case memory.RefEpisode:
		e, err := r.o.Repo.Episode(ctx, id)
		if err != nil {
			return memory.Explanation{}, err
		}
		ex.Episode, ex.Quote = &e, memory.Cut(e.Body, quoteChars)
		if ex.Links, err = r.o.Repo.LinksFrom(ctx, id); err != nil {
			return memory.Explanation{}, err
		}
	case memory.RefResource:
		if r.o.State == nil {
			return memory.Explanation{}, store.Fail(store.ErrNotFound, "no document %s", id)
		}
		res, err := r.o.State.Resource(ctx, id)
		if err != nil {
			return memory.Explanation{}, err
		}
		if res.EpisodeID != nil {
			if ex.Episode, err = d.episode(*res.EpisodeID); err != nil {
				return memory.Explanation{}, err
			}
		}
	}
	return ex, nil
}

// around returns the messages around a message of its thread, oldest first;
// none when it is gone.
func (r *Router) around(ctx context.Context, msgID string) ([]model.ChatMessage, error) {
	m, err := r.o.Repo.Message(ctx, msgID)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if r.o.Chats == nil {
		before, err := r.o.Repo.MessagesBefore(ctx, m.ThreadID, m.At, contextBefore)
		if err != nil {
			return nil, err
		}
		return append(before, m.ChatMessage), nil
	}
	all, err := r.o.Chats.Messages(ctx, m.ThreadID)
	if err != nil {
		return nil, ignoreNotFound(err)
	}
	i := slices.IndexFunc(all, func(x model.ChatMessage) bool { return x.ID == msgID })
	if i < 0 {
		return []model.ChatMessage{m.ChatMessage}, nil
	}
	return all[max(0, i-contextBefore):min(len(all), i+contextAfter+1)], nil
}

// Resolve resolves a ref for showing beside a chat reply: a claim's words
// and source; a message's words (cut) and chat; an episode's title or words;
// a document's title. Gone is set when it was forgotten or erased.
func (r *Router) Resolve(ctx context.Context, ref string) (wire.MemoryRef, error) {
	kind, id, ok := memory.ParseRef(ref)
	if !ok {
		return wire.MemoryRef{}, badRef(ref)
	}
	d := r.describer(ctx, r.o.Clock.Now())
	out := wire.MemoryRef{Ref: ref}
	gone := func(err error) (wire.MemoryRef, error) {
		if isNotFound(err) {
			out.Gone = true
			return out, nil
		}
		return wire.MemoryRef{}, fmt.Errorf("resolve %s: %w", ref, err)
	}
	switch kind {
	case memory.RefClaim:
		out.Kind = kindClaim
		c, err := r.o.Repo.Claim(ctx, id)
		if err != nil {
			return gone(err)
		}
		src, err := d.claimSource(c)
		if err != nil {
			return wire.MemoryRef{}, err
		}
		out.Text, out.Source, out.ThreadID = c.Text, src.words, src.threadID
	case memory.RefMessage:
		out.Kind = kindMessage
		m, err := r.o.Repo.Message(ctx, id)
		if err != nil {
			return gone(err)
		}
		out.Text, out.Source, out.ThreadID = memory.Cut(m.Text, refChars), d.messageSource(m), m.ThreadID
	case memory.RefEpisode:
		out.Kind = kindEpisode
		e, err := r.o.Repo.Episode(ctx, id)
		if err != nil {
			return gone(err)
		}
		if e.Kind == model.EpisodeResource {
			out.Kind = kindResource
		}
		out.Text = e.Title
		if out.Text == "" {
			out.Text = memory.Cut(e.Body, refChars)
		}
		out.Source = d.episodeSource(e).words
	case memory.RefResource:
		out.Kind = kindResource
		if r.o.State == nil {
			out.Gone = true
			return out, nil
		}
		res, err := r.o.State.Resource(ctx, id)
		if err != nil {
			return gone(err)
		}
		out.Text, out.Source = res.Title, documentSource(res.Title)
	case memory.RefData:
		out.Kind, out.Text, out.Source = kindData, id, "your tracked data"
	}
	return out, nil
}
