package api

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// This file compiles what an assistant turn sends besides the day's data
// (gwen-harness/harness.md section 3): the conversation as it was sent
// before, so the prompt cache holds; the context the model keeps in mind
// (the profile card and the chat's observation log); the memory note for the
// message; and the open tasks that bear on it.

// Sizes of an assistant turn.
const (
	// maxTranscript bounds the turns sent after an observation log, in case
	// the log stops growing.
	maxTranscript = 200
	// assistantTasks is how many open tasks are sent; with more, those that
	// bear on the message are chosen (chooseTasks).
	assistantTasks = 60
	assistantInbox = 15 // the newest inbox tasks among those chosen
	// recallHits and recallChars size the note recalled from chats when the
	// daemon runs no memory.
	recallHits  = 6
	recallChars = 400
)

// noteHeader opens a memory note, as the read path writes it.
const noteHeader = "What you remember that may bear on this (cite [mN] when you rely on one):"

// transcript is the part of a chat the model sees again, each turn exactly
// as it was sent before.
type transcript struct {
	all   []model.ChatMessage // every message of the chat, oldest first
	sent  []model.ChatMessage // the messages sent as turns
	notes map[string]model.ChatNote
	log   string // the chat's observation log, "" for none
	turns []llm.Turn
	// refStart numbers the new note's refs after every ref the chat's notes
	// used, so a ref names one item for the whole chat.
	refStart int
}

// transcript reads a chat for its next turn. With memory and an observation
// log, the turns are the messages the log does not cover; otherwise the
// newest llm.MaxAssistantMessages.
func (s *Server) transcript(ctx context.Context, threadID string) (transcript, error) {
	t := transcript{refStart: 1, notes: map[string]model.ChatNote{}}
	if threadID == "" {
		return t, nil
	}
	all, err := s.Repos.Chats.Messages(ctx, threadID)
	if err != nil {
		return t, err
	}
	notes, err := s.Repos.MemoryState.Notes(ctx, threadID)
	if err != nil {
		return t, err
	}
	t.all, t.notes = all, notes
	t.sent = all[max(0, len(all)-llm.MaxAssistantMessages):]
	if s.Memory != nil {
		o, ok, err := s.Memory.Observation(ctx, threadID)
		switch {
		case err != nil:
			slog.Warn("observation log unreadable; the chat goes on without it", "thread", threadID, "err", err)
		case ok:
			if i := slices.IndexFunc(all, func(m model.ChatMessage) bool { return m.ID == o.ThroughID }); i >= 0 {
				after := all[i+1:]
				t.sent, t.log = after[max(0, len(after)-maxTranscript):], o.Log
			}
		}
	}
	t.turns = renderTurns(t.sent, notes)
	t.refStart = nextNoteRef(notes)
	return t, nil
}

// renderTurns writes messages as the turns of a conversation: a user turn is
// the message and the memory note sent with it; an assistant turn is the
// short JSON of what was said and changed (pastReply). The same rows always
// render the same bytes.
func renderTurns(msgs []model.ChatMessage, notes map[string]model.ChatNote) []llm.Turn {
	turns := make([]llm.Turn, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			text := m.Text
			if n, ok := notes[m.ID]; ok && n.Note != "" {
				text += "\n\n<memory>\n" + n.Note + "\n</memory>"
			}
			turns = append(turns, llm.Turn{Role: llm.RoleUser, Text: text})
		case llm.RoleAssistant:
			turns = append(turns, llm.Turn{Role: llm.RoleAssistant, Text: pastReplyText(m)})
		}
	}
	return turns
}

// pastReply is an earlier reply as the conversation shows it: what was said,
// the face made, and what its actions changed.
type pastReply struct {
	Reply string   `json:"reply"`
	Mood  string   `json:"mood"`
	Did   []string `json:"did,omitempty"`
}

func pastReplyText(m model.ChatMessage) string {
	r := pastReply{Reply: m.Text, Mood: m.Mood}
	for _, a := range messageActions(m) {
		if a.OK {
			r.Did = append(r.Did, a.Summary)
		} else {
			r.Did = append(r.Did, "failed: "+a.Summary)
		}
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(r) // strings and a slice of strings always encode
	return strings.TrimSuffix(b.String(), "\n")
}

// messageActions are what an assistant message's actions did; none when the
// column does not read.
func messageActions(m model.ChatMessage) []wire.ActionResult {
	var acts []wire.ActionResult
	if len(m.Actions) > 0 {
		_ = json.Unmarshal(m.Actions, &acts)
	}
	return acts
}

var noteRefPattern = regexp.MustCompile(`^m([0-9]+)$`)

// nextNoteRef is 1 + the highest N of the notes' refs mN.
func nextNoteRef(notes map[string]model.ChatNote) int {
	top := 0
	for _, n := range notes {
		for ref := range n.Refs {
			if m := noteRefPattern.FindStringSubmatch(ref); m != nil {
				if v, err := strconv.Atoi(m[1]); err == nil {
					top = max(top, v)
				}
			}
		}
	}
	return top + 1
}

// messageRefs are the refs of messages, msg:<id>.
func messageRefs(msgs []model.ChatMessage) []string {
	refs := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.ID != "" {
			refs = append(refs, memory.Ref(memory.RefMessage, m.ID))
		}
	}
	return refs
}

func messageIDs(msgs []model.ChatMessage) []string {
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids
}

// memoryContext joins what the model keeps in mind across the conversation:
// the profile card and the chat's observation log, leaving out what is empty.
func memoryContext(card, log string) string {
	var parts []string
	if card = strings.TrimSpace(card); card != "" {
		parts = append(parts, "## What you keep in mind about them\n\n"+card)
	}
	if log = strings.TrimSpace(log); log != "" {
		parts = append(parts, "## Earlier in this conversation (your observations)\n\n"+log)
	}
	return strings.Join(parts, "\n\n")
}

// profileCard is today's profile card, or none when the memory has none or
// cannot say.
func (s *Server) profileCard(ctx context.Context) model.ProfileCard {
	if s.Memory == nil {
		return model.ProfileCard{}
	}
	card, err := s.Memory.Profile(ctx, s.today())
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, memory.ErrDisabled) {
			slog.Warn("profile card unavailable; the turn goes on without it", "err", err)
		}
		return model.ProfileCard{}
	}
	return card
}

// memoryNote is the memory note for msg: from the memory when the daemon runs
// one, else from the words of earlier chats. A note that fails is logged and
// the turn goes on without one.
func (s *Server) memoryNote(ctx context.Context, msg, threadID string, tr transcript) memory.Note {
	if s.Memory != nil {
		profile := memory.ProfileFrontier
		if s.Tracker.Config().LLM.Provider == config.ProviderOpenAICompatible {
			profile = memory.ProfileLocal
		}
		n, err := s.Memory.Note(ctx, memory.NoteRequest{Text: msg, ThreadID: threadID, Exclude: messageRefs(tr.sent),
			RefStart: tr.refStart, AsOf: s.now(), Profile: profile})
		if err != nil {
			slog.Warn("memory note failed; the turn goes on without one", "err", err)
			return memory.Note{}
		}
		return n
	}
	n, err := s.recallNote(ctx, msg, messageIDs(tr.sent), tr.refStart)
	if err != nil {
		slog.Warn("chat recall failed; the turn goes on without it", "err", err)
		return memory.Note{}
	}
	return n
}

// recallNote is the note of a daemon that runs no memory: the messages of
// chats, other than those in skip, that share words with text, in the order
// they were said, written as the read path writes messages.
func (s *Server) recallNote(ctx context.Context, text string, skip []string, refStart int) (memory.Note, error) {
	hits, err := s.Repos.Chats.Search(ctx, text, recallHits, skip)
	if err != nil || len(hits) == 0 {
		return memory.Note{}, err
	}
	// In the order said; a turn said within one millisecond puts the user first.
	slices.SortStableFunc(hits, func(a, b model.ChatHit) int {
		userFirst := func(h model.ChatHit) int {
			if h.Role == llm.RoleUser {
				return 0
			}
			return 1
		}
		return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.ThreadID, b.ThreadID), userFirst(a)-userFirst(b))
	})
	now := s.now()
	lines := []string{noteHeader}
	refs := map[string]string{}
	for i, h := range hits {
		ref := fmt.Sprintf("m%d", refStart+i)
		who := "they said"
		if h.Role == llm.RoleAssistant {
			who = "you said"
		}
		words := memory.Cut(strings.Join(strings.Fields(h.Text), " "), recallChars)
		lines = append(lines, fmt.Sprintf("[%s] %s, chat “%s”, %s: “%s”", ref, s.shortDate(h.At, now), h.ThreadTitle,
			who, words))
		refs[ref] = memory.Ref(memory.RefMessage, h.ID)
	}
	return memory.Note{Text: strings.Join(lines, "\n"), Refs: refs}, nil
}

// shortDate is "3 Oct" in the device's zone, with the year when it is not
// now's.
func (s *Server) shortDate(t, now time.Time) string {
	t, now = t.In(s.Loc), now.In(s.Loc)
	if t.Year() != now.Year() {
		return t.Format("2 Jan 2006")
	}
	return t.Format("2 Jan")
}

// refTable maps the refs a reply may name (m3, p2) to what they name: every
// note of the chat, this turn's note, and the profile card the turn sent.
// Note refs are unique within a chat, so they never clash.
func refTable(notes map[string]model.ChatNote, note memory.Note, card model.ProfileCard) map[string]string {
	table := map[string]string{}
	for _, n := range notes {
		maps.Copy(table, n.Refs)
	}
	maps.Copy(table, note.Refs)
	maps.Copy(table, card.Refs)
	return table
}

// citedRefs are the refs a reply cited, mapped to what they name; refs the
// turn never sent are dropped.
func citedRefs(cites []string, table map[string]string) []string {
	var out []string
	for _, c := range cites {
		if r, ok := table[c]; ok && !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// forgetClaims are the ids of the claims a reply asked to stop bringing up;
// refs naming anything but a claim are dropped.
func forgetClaims(forget []string, table map[string]string) []string {
	var ids []string
	for _, f := range forget {
		kind, id, ok := memory.ParseRef(table[f])
		if ok && kind == memory.RefClaim && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	return ids
}

// chooseTasks picks the open tasks a turn sends: all of them when there are
// at most assistantTasks; else, until there are that many, those in progress,
// planned in the next days (blocks), overdue or due within a week, the
// newest inbox tasks, those sharing words with what the user said, by
// overlap, those the conversation's actions named, and then the rest by
// priority and due day. The chosen keep the order of tasks.
func chooseTasks(tasks []model.Task, blocks map[string][]llm.AssistantBlock, today civil.Day, said []string,
	named map[string]bool) []model.Task {
	if len(tasks) <= assistantTasks {
		return tasks
	}
	chosen := map[string]bool{}
	take := func(ts []model.Task) {
		for _, t := range ts {
			if len(chosen) == assistantTasks {
				return
			}
			chosen[t.ID] = true
		}
	}
	where := func(keep func(t model.Task) bool) []model.Task {
		var out []model.Task
		for _, t := range tasks {
			if keep(t) {
				out = append(out, t)
			}
		}
		return out
	}
	soon := today.AddDays(7).String()
	take(where(func(t model.Task) bool { return t.Stage == model.StageDoing }))
	take(where(func(t model.Task) bool { return len(blocks[t.ID]) > 0 }))
	take(where(func(t model.Task) bool { return t.DueDay != nil && *t.DueDay <= soon }))

	inbox := where(func(t model.Task) bool { return t.Stage == model.StageInbox })
	slices.SortStableFunc(inbox, func(a, b model.Task) int { return b.CreatedAt.Compare(a.CreatedAt) })
	take(inbox[:min(len(inbox), assistantInbox)])

	words := map[string]bool{}
	for _, text := range said {
		for _, w := range taskWords(text) {
			words[w] = true
		}
	}
	overlap := map[string]int{}
	for _, t := range tasks {
		for _, w := range taskWords(t.Title + " " + t.Notes) {
			if words[w] {
				overlap[t.ID]++
			}
		}
	}
	related := where(func(t model.Task) bool { return overlap[t.ID] > 0 })
	slices.SortStableFunc(related, func(a, b model.Task) int { return overlap[b.ID] - overlap[a.ID] })
	take(related)
	take(where(func(t model.Task) bool { return named[t.ID] }))

	rest := slices.Clone(tasks)
	slices.SortStableFunc(rest, func(a, b model.Task) int {
		if c := b.Priority - a.Priority; c != 0 {
			return c
		}
		switch {
		case a.DueDay == nil && b.DueDay == nil:
			return 0
		case a.DueDay == nil:
			return 1
		case b.DueDay == nil:
			return -1
		}
		return cmp.Compare(*a.DueDay, *b.DueDay)
	})
	take(rest)
	return where(func(t model.Task) bool { return chosen[t.ID] })
}

// taskWords are the distinct words of text worth matching a task by: three
// letters or more, lowercased, a plural or past ending stripped, less the
// words chat search skips.
func taskWords(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len([]rune(w)) < 3 || chatStopWords[w] {
			continue
		}
		if w = stemWord(w); !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// stemWord strips a trailing -ing, -ed, -es (after s, x, z, ch, or sh), or
// -s, keeping at least three letters, so "essays" matches "essay".
func stemWord(w string) string {
	keep := func(suffix string) bool {
		return strings.HasSuffix(w, suffix) && len([]rune(w))-len(suffix) >= 3
	}
	if keep("ing") {
		return strings.TrimSuffix(w, "ing")
	}
	if keep("ed") {
		return strings.TrimSuffix(w, "ed")
	}
	if keep("es") {
		base := strings.TrimSuffix(w, "es")
		for _, end := range []string{"s", "x", "z", "ch", "sh"} {
			if strings.HasSuffix(base, end) {
				return base
			}
		}
	}
	if keep("s") && !strings.HasSuffix(w, "ss") {
		return strings.TrimSuffix(w, "s")
	}
	return w
}

// chatStopWords are the words chat search skips as too common to say what a
// message is about (store's stopWords).
var chatStopWords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`about above after again all also and any are aren because been before being
		below between both but can cannot could couldn did didn does doesn doing don down during each few for from
		further get got had hadn has hasn have haven having her here hers herself him himself his how into isn its
		itself just let like made make many may more most much must mustn myself not now off once only other ought
		our ours ourselves out over own really same shan she should shouldn some such than that the their theirs
		them themselves then there these they this those through too under until very was wasn were weren what
		when where which while who whom why will with won would wouldn you your yours yourself yourselves
		hey hello thanks thank please okay yeah yes sure gonna wanna want need today tomorrow yesterday`) {
		m[w] = true
	}
	return m
}()

// lastUserTexts are the texts of the newest n user messages, oldest first.
func lastUserTexts(msgs []model.ChatMessage, n int) []string {
	var out []string
	for i := len(msgs) - 1; i >= 0 && len(out) < n; i-- {
		if msgs[i].Role == llm.RoleUser {
			out = append([]string{msgs[i].Text}, out...)
		}
	}
	return out
}

// namedTasks are the tasks the actions of messages changed or tried to.
func namedTasks(msgs []model.ChatMessage) map[string]bool {
	named := map[string]bool{}
	for _, m := range msgs {
		for _, a := range messageActions(m) {
			if a.TaskID != nil {
				named[*a.TaskID] = true
			}
		}
	}
	return named
}
