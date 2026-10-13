package notify

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

// Moment is what Gwen knows when a nudge fires. At is local time.
type Moment struct {
	At                    time.Time
	SoftIdle, HardIdle    time.Duration
	BreakFor              time.Duration // the open break so far
	BreakReminder, Repeat time.Duration
	Worked, Target        time.Duration
	Briefing              string // clock_in: today's plan in a few words, if any
}

// line is one thing Gwen can say: her face, a title, and a body in the body
// markup Plasma renders (<b>, <i>, <br/>).
type line struct{ mood, title, body string }

// labels are Gwen's words for the engine's buttons, by action id.
var labels = map[string]string{"back": "I'm here", "break": "Take a break", "end_break": "I'm back", "clock_in": "Let's go"}

// Speak rewrites a nudge in Gwen's voice: a mood that fits the moment, one of
// a few lines so repeats do not read the same, and a tone that firms up the
// longer a break runs. Kinds she has no lines for pass through unchanged.
func Speak(n Notification, m Moment) Notification {
	var lines []line
	switch n.Kind {
	case "idle":
		lines = idleLines(m)
	case "break_long":
		lines = breakLines(m)
	case "break_active":
		lines = []line{
			{"shocked", "Caught you typing!", "You're on a break, but your hands aren't. End it so this counts as work."},
			{"smug", "Working on your break?", "I see those keystrokes. Tap <b>I'm back</b> so I can count them."},
		}
	case "clock_in":
		lines = clockInLines(m)
	}
	if len(lines) == 0 {
		return n
	}
	h := fnv.New64a()
	fmt.Fprintf(h, "%s|%d", n.Kind, m.At.UnixNano())
	l := lines[h.Sum64()%uint64(len(lines))]
	n.Mood, n.Title, n.Markup, n.Body = l.mood, l.title, l.body, plain(l.body)
	actions := make([]Action, len(n.Actions))
	for i, a := range n.Actions {
		if label, ok := labels[a.ID]; ok {
			a.Label = label
		}
		actions[i] = a
	}
	n.Actions = actions
	return n
}

func idleLines(m Moment) []line {
	soft, left := span(m.SoftIdle), span(m.HardIdle-m.SoftIdle)
	if m.targetMet() {
		return []line{
			{"happy", "You've earned a rest", fmt.Sprintf("Target done and quiet for <b>%s</b>. Take a proper break?", soft)},
			{"love", "Proud of you today", fmt.Sprintf("<b>%s</b> worked. Quiet for %s, so maybe call it here?", span(m.Worked), soft)},
		}
	}
	if h := m.At.Hour(); h >= 23 || h < 5 {
		return []line{
			{"thinking", "Did you fall asleep on me?", fmt.Sprintf("It's late and it's gone quiet. I'll call it a break in <b>%s</b>.", left)},
			{"crying", "Still up?", fmt.Sprintf("No input for <b>%s</b>. If you're done for the night, I'll count it as a break.", soft)},
		}
	}
	return []line{
		{"thinking", "Hello? Still there?", fmt.Sprintf("No input for <b>%s</b>. I'll stop counting it as work in <b>%s</b>.", soft, left)},
		{"smug", "Caught you daydreaming", fmt.Sprintf("<b>%s</b> without a keystroke. <b>%s</b> more and it's a break.", soft, left)},
		{"winking", "Psst, still with me?", fmt.Sprintf("It's been quiet for <b>%s</b>. Say the word and I'll start a break.", soft)},
		{"shocked", "Where'd you go?", fmt.Sprintf("Nothing for <b>%s</b>. In <b>%s</b> I'll count it as a break.", soft, left)},
	}
}

func breakLines(m Moment) []line {
	took, progress := span(m.BreakFor), m.progress()
	if m.targetMet() {
		return []line{
			{"love", "Rest as long as you like", progress + "<br/><i>This break is on me.</i>"},
			{"happy", "Still resting? Good.", fmt.Sprintf("<b>%s</b> off so far. You earned every minute.", took)},
		}
	}
	round := 0
	if m.Repeat > 0 && m.BreakFor > m.BreakReminder {
		round = int((m.BreakFor - m.BreakReminder + m.Repeat/2) / m.Repeat)
	}
	switch round {
	case 0:
		return []line{
			{"smiling", "Break's been " + took, progress + "<br/><i>Ready when you are.</i>"},
			{"happy", "Feeling refreshed?", fmt.Sprintf("<b>%s</b> on a break. %s", took, progress)},
		}
	case 1:
		return []line{
			{"winking", "Coffee's done by now, right? ☕", fmt.Sprintf("<b>%s</b> on a break. %s", took, progress)},
			{"smug", "That's a long coffee", fmt.Sprintf("<b>%s</b> and counting. Your tasks miss you.", took)},
		}
	case 2:
		todo := "Your work"
		if m.Target > 0 {
			todo = "The other <b>" + span(m.Target-m.Worked) + "</b>"
		}
		return []line{
			{"annoyed", took + ". Just saying.", "This break is getting longer than your focus blocks. " + progress},
			{"grumpy", "I'm still waiting…", fmt.Sprintf("<b>%s</b> on a break. %s won't do itself.", took, todo)},
		}
	default:
		return []line{
			{"angry", took + "?! Seriously?", "Come back and do one small thing. Just one.<br/>" + progress},
			{"crying", "Did you forget about me?", fmt.Sprintf("<b>%s</b> on a break. I'm still here, holding your spot.", took)},
		}
	}
}

func clockInLines(m Moment) []line {
	body := "Clock in and I'll start counting."
	if m.Briefing != "" {
		body = "<b>" + escape(m.Briefing) + "</b><br/>" + body
	}
	switch h := m.At.Hour(); {
	case h >= 5 && h < 12:
		return []line{{"happy", "Good morning! ☀️", body}, {"cool", "Morning. Ready to own today?", body}}
	case h >= 12 && h < 17:
		return []line{{"smiling", "Hey, ready to start?", body}, {"winking", "Afternoon! Let's get going", body}}
	case h >= 17 && h < 22:
		return []line{{"smiling", "Evening session?", body}, {"cool", "Night shift? I'm in.", body}}
	default:
		return []line{{"thinking", "Working this late?", body}, {"smug", "Can't sleep either?", body}}
	}
}

func (m Moment) targetMet() bool { return m.Target > 0 && m.Worked >= m.Target }

// progress is today's work against the target, in markup.
func (m Moment) progress() string {
	switch {
	case m.Target <= 0:
		return fmt.Sprintf("<b>%s</b> done today.", span(m.Worked))
	case m.Worked >= m.Target:
		return fmt.Sprintf("<b>%s</b> done today, target hit.", span(m.Worked))
	default:
		return fmt.Sprintf("<b>%s</b> done, <b>%s</b> to go.", span(m.Worked), span(m.Target-m.Worked))
	}
}

// span renders "45s", "15m", or "1h 5m".
func span(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

var (
	escaper   = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	unmarkup  = strings.NewReplacer("<b>", "", "</b>", "", "<i>", "", "</i>", "", "<br/>", "\n")
	unescaper = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")
)

func escape(s string) string { return escaper.Replace(s) }

// plain is markup as plain text, for the phone and the event stream.
func plain(markup string) string { return unescaper.Replace(unmarkup.Replace(markup)) }
