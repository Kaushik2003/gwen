package recall

import (
	"context"
	"log/slog"
	"strconv"
	"strings"

	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
)

// The note's first line, and its last when nothing answers a question about
// the past.
const (
	noteHeader  = "What you remember that may bear on this (cite [mN] when you rely on one):"
	noteAbstain = "Nothing in memory answers this. If they ask about it, say you don't remember."
)

// Note is the memory note for a user message: a recall within the reader's
// budget, packed as lines in order (claims, then messages and episodes, then
// data, then documents) with refs numbered from req.RefStart, stopping before
// the budget would be exceeded. The claims in it are counted as used.
func (r *Router) Note(ctx context.Context, req memory.NoteRequest) (memory.Note, error) {
	budget := memory.NoteTokensFrontier
	if req.Profile == memory.ProfileLocal {
		budget = memory.NoteTokensLocal
	}
	p, cands, err := r.recall(ctx, memory.Query{Text: req.Text, AsOf: req.AsOf, Exclude: req.Exclude,
		Budget: memory.Budget{Tokens: budget}})
	if err != nil {
		return memory.Note{}, err
	}
	note := memory.Note{Refs: map[string]string{}, Packet: p}

	var ordered []*cand
	for _, group := range [][]string{{kindClaim}, {kindMessage, kindEpisode}, {kindData}, {kindResource}} {
		for _, c := range cands {
			if c.claim != nil && c.claim.Status != model.ClaimActive && c.claim.Status != model.ClaimSuperseded {
				continue // never a candidate, quarantined, or rejected claim
			}
			for _, k := range group {
				if c.hit.Kind == k {
					ordered = append(ordered, c)
				}
			}
		}
	}
	start := max(req.RefStart, 1)
	number := func(n int) map[string]int {
		nums := map[string]int{}
		for i, c := range ordered[:n] {
			nums[c.hit.Ref] = start + i
		}
		return nums
	}

	now := r.o.Clock.Now()
	d := r.describer(ctx, now)
	reserve := 0
	if p.Abstain {
		reserve = memory.Tokens("\n" + noteAbstain)
	}
	all := number(len(ordered))
	text := noteHeader
	fit := 0
	for _, c := range ordered {
		line, err := r.noteLine(d, c, all)
		if err != nil {
			return memory.Note{}, err
		}
		next := text + "\n" + line
		if memory.Tokens(next)+reserve > budget {
			break
		}
		text, fit = next, fit+1
	}
	if fit == 0 && !p.Abstain {
		return note, nil
	}

	// Render again, so a disputed claim names only conflicts in the note.
	nums := number(fit)
	lines := []string{noteHeader}
	var used []string
	for _, c := range ordered[:fit] {
		line, err := r.noteLine(d, c, nums)
		if err != nil {
			return memory.Note{}, err
		}
		lines = append(lines, line)
		note.Refs["m"+strconv.Itoa(nums[c.hit.Ref])] = c.hit.Ref
		if c.claim != nil {
			used = append(used, c.claim.ID)
		}
	}
	if p.Abstain {
		lines = append(lines, noteAbstain)
	}
	note.Text = strings.Join(lines, "\n")
	if len(used) > 0 && r.o.State != nil {
		if err := r.o.State.TouchClaims(ctx, used, now); err != nil {
			slog.Warn("memory note: count the claims used", "err", err)
		}
	}
	return note, nil
}

// noteLine is one line of a note; nums are the refs in the note.
func (r *Router) noteLine(d *describer, c *cand, nums map[string]int) (string, error) {
	ref := "[m" + strconv.Itoa(nums[c.hit.Ref]) + "] "
	h := c.hit
	switch {
	case c.claim != nil:
		src, err := d.claimSource(*c.claim)
		if err != nil {
			return "", err
		}
		var parts []string
		if v := r.validity(c.claim); v != "" {
			parts = append(parts, v)
		}
		if src.said != "" {
			parts = append(parts, src.said)
		}
		if h.Disputed {
			parts = append(parts, disputed(c, nums))
		}
		line := ref + h.Text
		if len(parts) > 0 {
			line += " (" + strings.Join(parts, "; ") + ")"
		}
		return line, nil
	case c.msg != nil:
		who := "they said"
		if c.msg.Role != model.ChatUser {
			who = "you said"
		}
		return ref + r.day(c.msg.At, d.now) + ", chat “" + c.msg.ThreadTitle + "”, " + who + ": “" + h.Text + "”", nil
	case c.ep != nil && c.ep.Kind != model.EpisodeResource:
		who := "they"
		if c.ep.Speaker == model.SpeakerGwen {
			who = "you"
		}
		what := c.ep.Kind + " " + who + " wrote"
		if c.ep.Kind == model.EpisodeEdit {
			what = "edit " + who + " made"
		}
		return ref + r.day(c.ep.At, d.now) + ", " + what + ": “" + h.Text + "”", nil
	case h.Kind == kindResource:
		from := "from a document"
		switch {
		case c.ep != nil:
			from = "from the " + documentSource(c.ep.Title)
		case strings.HasPrefix(h.Source, "document "):
			from = "from the " + h.Source
		case h.Source != "":
			from = "from " + h.Source
		}
		return ref + from + " (written by others): “" + h.Text + "”", nil
	}
	return ref + h.Text, nil
}

// validity says when a claim held: "since Oct 2026", "until Mar 2026", or
// "Oct 2025 to Mar 2026"; "" when always.
func (r *Router) validity(c *model.Claim) string {
	month := func(t interface{ Format(string) string }) string { return t.Format("Jan 2006") }
	switch {
	case c.ValidFrom != nil && c.ValidTo != nil:
		return month(c.ValidFrom.In(r.o.Loc)) + " to " + month(c.ValidTo.In(r.o.Loc))
	case c.ValidFrom != nil:
		return "since " + month(c.ValidFrom.In(r.o.Loc))
	case c.ValidTo != nil:
		return "until " + month(c.ValidTo.In(r.o.Loc))
	}
	return ""
}

// disputed says a claim is disputed, naming the claims it conflicts with
// that the note holds.
func disputed(c *cand, nums map[string]int) string {
	var with []string
	for _, k := range c.conflicts {
		other := k.ClaimA
		if other == c.claim.ID {
			other = k.ClaimB
		}
		if n, ok := nums[memory.Ref(memory.RefClaim, other)]; ok {
			with = append(with, "[m"+strconv.Itoa(n)+"]")
		}
	}
	if len(with) == 0 {
		return "disputed"
	}
	return "disputed: conflicts with " + strings.Join(with, ", ")
}
