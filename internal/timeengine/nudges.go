package timeengine

import (
	"fmt"
	"time"
)

// nudge builds a notification with the text of
// docs/07-integrations.md#notifications.
func (d *decider) nudge(kind string, at time.Time) Notify {
	c := d.s.cfg
	snooze := Action{ID: ActionSnooze, Label: "Snooze " + formatDuration(c.Snooze)}
	backToWork := Action{ID: ActionEndBreak, Label: "Back to work"}
	n := Notify{Kind: kind, At: at}
	switch kind {
	case NudgeIdle:
		n.Title = "Still there?"
		n.Body = fmt.Sprintf("No input for %s. It stops counting as work at %s.",
			formatDuration(c.SoftIdle), formatDuration(c.HardIdle))
		n.Actions = []Action{{ID: ActionBack, Label: "I'm back"}, {ID: ActionBreak, Label: "Start break"}, snooze}
	case NudgeBreakLong:
		n.Title = "On a break for " + formatDuration(at.Sub(d.s.Segment.StartedAt))
		n.Body = "Ready to get back to it?"
		n.Actions = []Action{backToWork, snooze}
	case NudgeBreakActive:
		n.Title = "You're typing during a break"
		n.Body = "End the break so this counts as work."
		n.Actions = []Action{backToWork}
	case NudgeClockIn:
		n.Title = "Ready to start?"
		n.Body = "Clock in to start tracking."
		n.Actions = []Action{{ID: ActionClockIn, Label: "Clock in"}}
	}
	return n
}

// formatDuration renders "45s", "15m", or "1h 5m", the display format shared
// with internal/client.FormatDuration.
func formatDuration(d time.Duration) string {
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
