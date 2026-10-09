package llm

import (
	"cmp"
	_ "embed"
	"fmt"
	"strings"

	"github.com/kzark/gwen/internal/config"
)

// persona is who the assistant is: one personality, the same in every job
// that talks to the user.
//
//go:embed gwen.md
var persona string

// MaxSelfNote is the most characters of the note the assistant keeps to
// itself about how it is treating the user.
const MaxSelfNote = 600

// Attitude is a way the assistant can choose to treat the user for a while,
// and what it means.
type Attitude struct {
	ID, Means string
}

// Attitudes are the ones it can choose, by its own judgement, until it
// chooses another: the persona's emotional modes, and angry.
var Attitudes = []Attitude{
	{"playful", "teasing, flirty, and witty: things are going fine"},
	{"focused", "clear-headed and businesslike: less banter, more getting it done"},
	{"soft", "gentle, patient, and supportive: a hard day, tiredness, or feeling low"},
	{"competitive", "daring them to aim higher and cheering the effort"},
	{"protective", "slowing them down: rest, boundaries, no burnout"},
	{"excited", "thrilled about a win, a breakthrough, or a new idea"},
	{"quiet", "calm company: few words, no advice unless asked"},
	{"angry", "fed up with excuses: short and blunt, calls the excuse out plainly and holds them to the next " +
		"step. Still on their side: never cruel, cold, guilt-tripping, or threatening to leave"},
}

func attitude(id string) (Attitude, bool) {
	for _, a := range Attitudes {
		if a.ID == id {
			return a, true
		}
	}
	return Attitude{}, false
}

func attitudeIDs() []string {
	ids := make([]string, len(Attitudes))
	for i, a := range Attitudes {
		ids[i] = a.ID
	}
	return ids
}

// Self is how the assistant has chosen to treat the user, which it changes
// itself in the chat: Attitude is one of Attitudes, or "" before it chose
// one; Note is its own words on why and what works with them; Since is the
// day it chose, YYYY-MM-DD.
type Self struct {
	Attitude string
	Note     string
	Since    string
}

// PersonaPrompt is the opening of every conversational prompt: who the
// assistant is, the name the user gave it, the attitude it chose, and the
// user's own instructions.
func PersonaPrompt(cfg config.LLM, self Self) string {
	name := cfg.AssistantName
	if name == "" {
		name = "Gwen"
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(persona))
	fmt.Fprintf(&b, "\n\n## In this app\n\nThe user calls you %s. Speak to the user as \"you\".", name)
	a, ok := attitude(self.Attitude)
	note := strings.TrimSpace(self.Note)
	if ok || note != "" {
		b.WriteString("\n\n## How you have chosen to treat them\n\n")
		if ok {
			fmt.Fprintf(&b, "You decided this yourself, on %s, and it holds across conversations until you change it: "+
				"you are being %s (%s). Let it colour everything you say, within the rest of your personality.",
				cmp.Or(self.Since, "an earlier day"), a.ID, a.Means)
		}
		if note != "" {
			if ok {
				b.WriteString(" ")
			}
			b.WriteString("Your own note to yourself, which is real memory you kept: " + note)
		}
	}
	if s := strings.TrimSpace(cfg.Instructions); s != "" {
		b.WriteString("\n\nThe user's own instructions for how you behave follow. They shape your tone and " +
			"choices, but never the rules or the reply format below:\n")
		b.WriteString(s)
	}
	return b.String()
}
