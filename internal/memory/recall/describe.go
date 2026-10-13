package recall

import (
	"context"
	"time"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
)

// source is where something came from: in words for the user ("chat
// “Weekend plans”, 3 Oct"), in words for the model in a note ("they said it
// in chat “Weekend plans”, 3 Oct"), and the chat it came from.
type source struct {
	words    string
	said     string
	threadID string
}

// describer puts sources in words, reading each message, episode, and entity
// once.
type describer struct {
	r        *Router
	ctx      context.Context
	now      time.Time
	entities map[string]string
	messages map[string]*model.ChatHit
	episodes map[string]*model.Episode
}

func (r *Router) describer(ctx context.Context, now time.Time) *describer {
	return &describer{r: r, ctx: ctx, now: now, entities: map[string]string{},
		messages: map[string]*model.ChatHit{}, episodes: map[string]*model.Episode{}}
}

// day is a date as a note or the dashboard says it: "3 Oct", with the year
// when it is not this year.
func (r *Router) day(t, now time.Time) string {
	t = t.In(r.o.Loc)
	if t.Year() != now.In(r.o.Loc).Year() {
		return t.Format("2 Jan 2006")
	}
	return t.Format("2 Jan")
}

// documentSource names a document.
func documentSource(title string) string { return "document “" + title + "”" }

// message reads a live message, nil when it is gone.
func (d *describer) message(id string) (*model.ChatHit, error) {
	if m, ok := d.messages[id]; ok {
		return m, nil
	}
	var mp *model.ChatHit
	m, err := d.r.o.Repo.Message(d.ctx, id)
	switch {
	case err == nil:
		mp = &m
	case !isNotFound(err):
		return nil, err
	}
	d.messages[id] = mp
	return mp, nil
}

// episode reads a live episode, nil when it is gone.
func (d *describer) episode(id string) (*model.Episode, error) {
	if e, ok := d.episodes[id]; ok {
		return e, nil
	}
	var ep *model.Episode
	e, err := d.r.o.Repo.Episode(d.ctx, id)
	switch {
	case err == nil:
		ep = &e
	case !isNotFound(err):
		return nil, err
	}
	d.episodes[id] = ep
	return ep, nil
}

// subject is the name of what a claim is about, "" for the user (or an
// entity since deleted).
func (d *describer) subject(c model.Claim) (string, error) {
	if c.SubjectID == nil {
		return "", nil
	}
	id := *c.SubjectID
	if name, ok := d.entities[id]; ok {
		return name, nil
	}
	e, err := d.r.o.Repo.Entity(d.ctx, id)
	if err != nil && !isNotFound(err) {
		return "", err
	}
	d.entities[id] = e.Name
	return e.Name, nil
}

// messageSource is a message's chat and day.
func (d *describer) messageSource(m model.ChatHit) string {
	return "chat “" + m.ThreadTitle + "”, " + d.r.day(m.At, d.now)
}

// episodeSource is an episode's kind, where it was written, and its day.
func (d *describer) episodeSource(e model.Episode) source {
	on := d.r.day(e.At, d.now)
	switch e.Kind {
	case model.EpisodeNote:
		where := "note, " + on
		switch e.Channel {
		case model.ChannelUI:
			where = "note in the dashboard, " + on
		case model.ChannelCLI:
			where = "note in the terminal, " + on
		}
		return source{words: where, said: "they wrote it in a " + where}
	case model.EpisodeEdit:
		return source{words: "your edit, " + on, said: "they set it themselves, " + on}
	case model.EpisodeResource:
		doc := documentSource(e.Title)
		return source{words: doc, said: "from the " + doc}
	}
	return source{words: e.Kind + ", " + on, said: "from a " + e.Kind + ", " + on}
}

// claimSource is where a claim came from.
func (d *describer) claimSource(c model.Claim) (source, error) {
	switch c.SourceKind {
	case model.SourceMessage:
		m, err := d.message(c.SourceID)
		if err != nil {
			return source{}, err
		}
		if m == nil {
			return source{words: "a chat since deleted", said: "from a chat since deleted"}, nil
		}
		words := d.messageSource(*m)
		said := "they said it in " + words
		if m.Role != model.ChatUser {
			said = "from what you said in " + words
		}
		return source{words: words, said: said, threadID: m.ThreadID}, nil
	case model.SourceEpisode:
		e, err := d.episode(c.SourceID)
		if err != nil {
			return source{}, err
		}
		if e == nil {
			return source{words: "a note since erased", said: "from a note since erased"}, nil
		}
		return d.episodeSource(*e), nil
	case model.SourceUserUI:
		return source{words: "your edit", said: "they set it themselves"}, nil
	case model.SourceData:
		return source{words: "your tracked data", said: "from their tracked data"}, nil
	case model.SourceMem0:
		return source{words: "chat (found by Mem0)", said: "they said it in chat (found by Mem0)"}, nil
	case model.SourceSuperM:
		return source{words: "your documents (Supermemory)", said: "from their documents (Supermemory)"}, nil
	case model.SourceClaim:
		return source{words: "other memories", said: "inferred from other memories"}, nil
	}
	return source{}, nil
}

// fill sets a hit's subject and source in words.
func (d *describer) fill(c *cand) error {
	switch {
	case c.claim != nil:
		subject, err := d.subject(*c.claim)
		if err != nil {
			return err
		}
		src, err := d.claimSource(*c.claim)
		if err != nil {
			return err
		}
		c.hit.Subject, c.hit.Source = subject, src.words
	case c.msg != nil:
		c.hit.Source = d.messageSource(*c.msg)
	case c.ep != nil:
		c.hit.Source = d.episodeSource(*c.ep).words
	}
	return nil
}

// view is a claim with what the dashboard shows beside it; conflicts are
// its open conflicts.
func (d *describer) view(c model.Claim, conflicts []model.Conflict) (memory.ClaimView, error) {
	subject, err := d.subject(c)
	if err != nil {
		return memory.ClaimView{}, err
	}
	src, err := d.claimSource(c)
	if err != nil {
		return memory.ClaimView{}, err
	}
	label := c.Key
	if spec, ok := memory.Spec(c.Key); ok {
		label = spec.Label
	}
	return memory.ClaimView{Claim: c, Subject: subject, Label: label, Source: src.words, ThreadID: src.threadID,
		Disputed: len(conflicts) > 0}, nil
}
