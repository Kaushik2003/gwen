package learn

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
)

// Extraction limits (spec 5, 1).
const (
	readLimit      = 200  // messages, and episodes, one run reads
	runTokens      = 1024 // pending tokens that start a run
	runAge         = time.Hour
	batchItems     = llm.MaxExtractItems
	batchTokens    = 6000 // about the most tokens of one batch's items
	itemChars      = 4000 // characters of an item's text
	contextBefore  = 4    // earlier messages of the thread sent as context
	maxEntityRefs  = 200  // entities sent
	maxKnownOne    = 80   // active one-valued claims sent
	maxKnownMany   = 40   // newest active many-valued claims sent
	maxKnownSubjEx = 20   // entities sent beyond maxEntityRefs for known claims
	tokensPerItem  = 12   // what an item's ref, speaker, and time cost besides its text
)

// rememberCues are words that make a pending user message worth learning
// from at once.
var rememberCues = []string{"remember", "don't forget", "note that", "keep in mind", "for future reference"}

// item is a message or an episode an extraction reads.
type item struct {
	kind    string // model.SourceMessage or model.SourceEpisode
	id      string
	rowid   int64
	thread  string
	speaker string // model.SpeakerUser, SpeakerGwen, or SpeakerExternal
	channel string
	at      time.Time
	text    string // whole
	cut     string // as the model reads it
	tokens  int
}

// cursor is a rowid cursor of local_state and the rows read after it. It
// moves to the end of the longest run of rows that are done, so rows of a
// batch not yet committed are read again after a crash.
type cursor struct {
	key  string
	at   int64
	rows []int64
	done map[int64]bool
}

func (c *cursor) next() int64 {
	v := c.at
	for _, r := range c.rows {
		if !c.done[r] {
			break
		}
		v = r
	}
	return v
}

// window is what an extraction would read now.
type window struct {
	msgs, eps []item
	msgCur    *cursor
	epCur     *cursor
}

func (w *window) items() int { return len(w.msgs) + len(w.eps) }

func (w *window) tokens() int {
	n := 0
	for _, it := range w.msgs {
		n += it.tokens
	}
	for _, it := range w.eps {
		n += it.tokens
	}
	return n
}

// readWindow reads the messages and episodes after the cursors. Messages
// with no words, resources (quarantined content, read only when a note or an
// edit quotes them), and notes and edits that already have claims (made by
// Remember or Edit) are passed over.
func (l *Learner) readWindow(ctx context.Context) (*window, error) {
	mc, err := l.getCursor(ctx, store.KeyMemoryMsgCursor)
	if err != nil {
		return nil, err
	}
	ec, err := l.getCursor(ctx, store.KeyMemoryEpCursor)
	if err != nil {
		return nil, err
	}
	w := &window{msgCur: &cursor{key: store.KeyMemoryMsgCursor, at: mc, done: map[int64]bool{}},
		epCur: &cursor{key: store.KeyMemoryEpCursor, at: ec, done: map[int64]bool{}}}
	msgs, err := l.o.Repo.MessagesAfter(ctx, mc, readLimit)
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		w.msgCur.rows = append(w.msgCur.rows, m.Rowid)
		if strings.TrimSpace(m.Text) == "" {
			w.msgCur.done[m.Rowid] = true
			continue
		}
		speaker := model.SpeakerUser
		if m.Role == model.ChatAssistant {
			speaker = model.SpeakerGwen
		}
		w.msgs = append(w.msgs, newItem(model.SourceMessage, m.ID, m.Rowid, m.ThreadID, speaker, model.ChannelChat,
			m.At, m.Text))
	}
	eps, err := l.o.Repo.EpisodesAfter(ctx, ec, readLimit)
	if err != nil {
		return nil, err
	}
	for _, e := range eps {
		w.epCur.rows = append(w.epCur.rows, e.Rowid)
		if e.Kind != model.EpisodeNote && e.Kind != model.EpisodeEdit || strings.TrimSpace(e.Body) == "" {
			w.epCur.done[e.Rowid] = true
			continue
		}
		made, err := l.o.Repo.ClaimsFrom(ctx, model.SourceEpisode, e.ID)
		if err != nil {
			return nil, err
		}
		if len(made) > 0 {
			w.epCur.done[e.Rowid] = true
			continue
		}
		w.eps = append(w.eps, newItem(model.SourceEpisode, e.ID, e.Rowid, "", e.Speaker, e.Channel, e.At, e.Body))
	}
	return w, nil
}

func newItem(kind, id string, rowid int64, thread, speaker, channel string, at time.Time, text string) item {
	cut := memory.Cut(text, itemChars)
	return item{kind: kind, id: id, rowid: rowid, thread: thread, speaker: speaker, channel: channel, at: at,
		text: text, cut: cut, tokens: memory.Tokens(cut) + tokensPerItem}
}

// Pending reports what Extract would read now: the items, and about how many
// tokens they hold.
func (l *Learner) Pending(ctx context.Context) (items, tokens int, err error) {
	w, err := l.readWindow(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("pending memory: %w", err)
	}
	return w.items(), w.tokens(), nil
}

// due reports whether a window is worth a model call now: enough words, an
// item waiting over an hour, or a user message asking to remember.
func (l *Learner) due(w *window) bool {
	if w.tokens() >= runTokens {
		return true
	}
	now := l.now()
	for _, list := range [][]item{w.msgs, w.eps} {
		for _, it := range list {
			if now.Sub(it.at) > runAge {
				return true
			}
		}
	}
	for _, it := range w.msgs {
		if it.speaker != model.SpeakerUser {
			continue
		}
		text := memory.Normalize(it.text)
		for _, cue := range rememberCues {
			if strings.Contains(text, cue) {
				return true
			}
		}
	}
	return false
}

// batch is items an extraction reads in one model call.
type batch struct {
	items    []item
	context  []model.ChatMessage
	explicit bool
	cur      *cursor
}

// batches splits a window: messages per thread in order, then episodes, each
// at most batchItems items and about batchTokens tokens.
func (l *Learner) batches(ctx context.Context, w *window) ([]batch, error) {
	var out []batch
	var threads []string
	byThread := map[string][]item{}
	for _, it := range w.msgs {
		if _, ok := byThread[it.thread]; !ok {
			threads = append(threads, it.thread)
		}
		byThread[it.thread] = append(byThread[it.thread], it)
	}
	for _, t := range threads {
		for _, chunk := range chunk(byThread[t]) {
			before, err := l.o.Repo.MessagesBefore(ctx, t, chunk[0].at, contextBefore)
			if err != nil {
				return nil, err
			}
			out = append(out, batch{items: chunk, context: before, cur: w.msgCur})
		}
	}
	for _, c := range chunk(w.eps) {
		out = append(out, batch{items: c, explicit: true, cur: w.epCur})
	}
	return out, nil
}

// chunk splits items into runs of at most batchItems items and about
// batchTokens tokens.
func chunk(items []item) [][]item {
	var out [][]item
	var cur []item
	tokens := 0
	for _, it := range items {
		if len(cur) > 0 && (len(cur) == batchItems || tokens+it.tokens > batchTokens) {
			out = append(out, cur)
			cur, tokens = nil, 0
		}
		cur = append(cur, it)
		tokens += it.tokens
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// Extract learns from the messages and episodes after the cursors (spec 5,
// 1), when force is set or they are due. Each batch is committed and its
// cursor moved before the next; a model error stops the run and is returned,
// and the batch is read again next time.
func (l *Learner) Extract(ctx context.Context, force bool) (Result, error) {
	l.extractMu.Lock()
	defer l.extractMu.Unlock()
	w, err := l.readWindow(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("learn: %w", err)
	}
	if w.items() == 0 {
		// Only rows to pass over, if any.
		return Result{}, l.saveCursors(ctx, w)
	}
	if !force && !l.due(w) {
		return Result{}, nil
	}
	m, err := l.model()
	if err != nil {
		return Result{}, fmt.Errorf("learn: %w", err)
	}
	batches, err := l.batches(ctx, w)
	if err != nil {
		return Result{}, fmt.Errorf("learn: %w", err)
	}
	var res Result
	for i, b := range batches {
		r, err := l.extractBatch(ctx, m, b)
		if err != nil {
			return res, errors.Join(fmt.Errorf("learn: %w", err), l.learned(ctx, i > 0))
		}
		res.merge(r)
		for _, it := range b.items {
			b.cur.done[it.rowid] = true
		}
		if err := l.saveCursor(ctx, b.cur); err != nil {
			return res, fmt.Errorf("learn: %w", err)
		}
	}
	if err := l.saveCursors(ctx, w); err != nil {
		return res, fmt.Errorf("learn: %w", err)
	}
	return res, l.learned(ctx, true)
}

// learned records when the memory last learned, after a run that read
// anything.
func (l *Learner) learned(ctx context.Context, read bool) error {
	if !read {
		return nil
	}
	now := l.now()
	if err := store.SetLocalTime(ctx, l.o.DB.SQL(), store.KeyMemoryLastLearned, now, now); err != nil {
		return fmt.Errorf("learn: %w", err)
	}
	return nil
}

// saveCursor moves a cursor as far as its rows are done.
func (l *Learner) saveCursor(ctx context.Context, c *cursor) error {
	if n := c.next(); n > c.at {
		if err := l.setCursor(ctx, c.key, n); err != nil {
			return err
		}
		c.at = n
	}
	return nil
}

func (l *Learner) saveCursors(ctx context.Context, w *window) error {
	if err := l.saveCursor(ctx, w.msgCur); err != nil {
		return err
	}
	return l.saveCursor(ctx, w.epCur)
}

// requestRefs are the refs of an extraction request.
type requestRefs struct {
	entities map[string]model.Entity // e1 -> entity
	known    map[string]string       // k1 -> claim id
}

// baseRequest is an extraction request without items: the day, the keys,
// the entities, and the claims a fact may update.
func (l *Learner) baseRequest(ctx context.Context) (llm.ExtractRequest, requestRefs, error) {
	now := l.now()
	req := llm.ExtractRequest{Today: l.day(now), Now: now.In(l.loc).Format(time.RFC3339), Timezone: l.loc.String(),
		Keys: make([]llm.ExtractKey, len(memory.Keys)), Entities: []llm.ExtractEntity{}, Known: []llm.ExtractKnown{},
		Context: []llm.ExtractItem{}, Items: []llm.ExtractItem{}}
	for i, k := range memory.Keys {
		req.Keys[i] = llm.ExtractKey{Key: k.Key, Label: k.Label, Cardinality: k.Cardinality, About: k.Subject}
	}
	refs := requestRefs{entities: map[string]model.Entity{}, known: map[string]string{}}
	entities, err := l.o.Repo.Entities(ctx, "")
	if err != nil {
		return req, refs, err
	}
	refOf := map[string]string{}
	addEntity := func(e model.Entity) string {
		if r, ok := refOf[e.ID]; ok {
			return r
		}
		r := fmt.Sprintf("e%d", len(req.Entities)+1)
		aliases := e.Aliases
		if aliases == nil {
			aliases = []string{}
		}
		req.Entities = append(req.Entities, llm.ExtractEntity{Ref: r, Kind: e.Kind, Name: e.Name, Aliases: aliases})
		refs.entities[r], refOf[e.ID] = e, r
		return r
	}
	byID := map[string]model.Entity{}
	for i, e := range entities {
		byID[e.ID] = e
		if i < maxEntityRefs {
			addEntity(e)
		}
	}
	claims, err := l.o.Repo.Claims(ctx, store.ClaimFilter{Statuses: []string{model.ClaimActive},
		IncludeSuppressed: true, ExcludeSensitive: true})
	if err != nil {
		return req, refs, err
	}
	var one, many []model.Claim
	for _, c := range claims {
		switch {
		case c.Cardinality == model.CardinalityOne && len(one) < maxKnownOne:
			one = append(one, c)
		case c.Cardinality == model.CardinalityMany && len(many) < maxKnownMany:
			many = append(many, c)
		}
	}
	extra := 0
	for _, c := range append(one, many...) {
		subject := memory.SubjectUser
		if c.SubjectID != nil {
			e, ok := byID[*c.SubjectID]
			if !ok {
				continue
			}
			if _, sent := refOf[e.ID]; !sent {
				if extra == maxKnownSubjEx {
					continue
				}
				extra++
			}
			subject = addEntity(e)
		}
		r := fmt.Sprintf("k%d", len(req.Known)+1)
		req.Known = append(req.Known, llm.ExtractKnown{Ref: r, Key: c.Key, Subject: subject, Text: c.Text,
			Since: l.dayOf(c.ValidFrom, false)})
		refs.known[r] = c.ID
	}
	return req, refs, nil
}

// itemSpeaker is how an item's speaker reads to the model.
func itemSpeaker(s string) string {
	switch s {
	case model.SpeakerUser:
		return "user"
	case model.SpeakerGwen:
		return "gwen"
	}
	return "document"
}

// extractBatch asks the model for a batch's facts and proposes them.
func (l *Learner) extractBatch(ctx context.Context, m llm.Memory, b batch) (Result, error) {
	req, refs, err := l.baseRequest(ctx)
	if err != nil {
		return Result{}, err
	}
	req.Explicit = b.explicit
	for i, c := range b.context {
		speaker := model.SpeakerUser
		if c.Role == model.ChatAssistant {
			speaker = model.SpeakerGwen
		}
		req.Context = append(req.Context, llm.ExtractItem{Ref: fmt.Sprintf("c%d", i+1), Speaker: itemSpeaker(speaker),
			At: l.localStamp(c.At), Text: memory.Cut(c.Text, itemChars)})
	}
	for i, it := range b.items {
		req.Items = append(req.Items, llm.ExtractItem{Ref: fmt.Sprintf("i%d", i+1), Speaker: itemSpeaker(it.speaker),
			At: l.localStamp(it.at), Text: it.cut})
	}
	out, err := m.Extract(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("extract: %w", err)
	}
	trust := ""
	if b.explicit {
		trust = model.TrustOwnerEdit
	}
	cands := l.candidates(b, out.Facts, refs, extractorOf(m), trust)
	res, _, err := l.propose(ctx, cands)
	return res, err
}

// candidates makes the facts of a batch candidates. A fact sourced to
// Gwen's words whose quote the user said too, in the batch or just before
// it, is sourced to the user's message instead (the speaker rule).
func (l *Learner) candidates(b batch, facts []llm.ExtractedFact, refs requestRefs, extractor,
	trust string) []Candidate {
	var ctxItems []item
	for _, c := range b.context {
		speaker := model.SpeakerUser
		if c.Role == model.ChatAssistant {
			speaker = model.SpeakerGwen
		}
		ctxItems = append(ctxItems, newItem(model.SourceMessage, c.ID, 0, c.ThreadID, speaker, model.ChannelChat,
			c.At, c.Text))
	}
	all := append(ctxItems, b.items...)
	var out []Candidate
	for _, f := range facts {
		var n int
		if _, err := fmt.Sscanf(f.Source, "i%d", &n); err != nil || n < 1 || n > len(b.items) {
			continue
		}
		at := len(ctxItems) + n - 1
		src := all[at]
		if src.speaker == model.SpeakerGwen && f.Quote != "" {
			// The batch's own messages first, then those before it.
			for k := range all {
				i := (len(ctxItems) + k) % len(all)
				if it := all[i]; it.speaker == model.SpeakerUser && memory.ContainsQuote(it.text, f.Quote) {
					src, at = it, i
					break
				}
			}
		}
		prev := ""
		if at > 0 {
			prev = all[at-1].text
		}
		out = append(out, Candidate{Fact: f, SourceKind: src.kind, SourceID: src.id, SourceText: src.text,
			Speaker: src.speaker, Channel: src.channel, At: src.at, Context: prev, Extractor: extractor, Trust: trust,
			Entities: refs.entities, Known: refs.known})
	}
	return out
}

// isNotFound reports whether err is the store's ErrNotFound.
func isNotFound(err error) bool { return errors.Is(err, store.ErrNotFound) }
