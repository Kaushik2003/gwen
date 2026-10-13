package notify

import (
	"context"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
)

// previewStep is how long each nudge of a preview shows before the next.
const previewStep = 7 * time.Second

// previewKey is where a preview nudge is kept among the nudges showing: one at
// a time, apart from the real ones, and its buttons do nothing.
const previewKey = "preview"

// previews are a sample of every nudge Gwen can send, in her voice, as cfg and
// now would have them: idle by day and late at night, a long break as it
// firms up and once the target is met, typing on a break, and clocking in in
// the morning and the evening.
func previews(cfg config.Config, now time.Time) []Notification {
	day := func(h, m int) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	}
	base := Moment{SoftIdle: cfg.Tracking.SoftIdle, HardIdle: cfg.Tracking.HardIdle, BreakReminder: cfg.Nudge.BreakReminder,
		Repeat: cfg.Nudge.Repeat, Target: cfg.Tracking.DailyTarget}
	base.Worked = base.Target * 28 / 100
	snooze := Action{"snooze", "Snooze " + span(cfg.Nudge.Snooze)}
	idle := Notification{Kind: "idle", Actions: []Action{{"back", "I'm back"}, {"break", "Start break"}, snooze}}
	long := Notification{Kind: "break_long", Actions: []Action{{"end_break", "Back to work"}, snooze}}
	active := Notification{Kind: "break_active", Actions: []Action{{"end_break", "Back to work"}}}
	clockIn := Notification{Kind: "clock_in", Actions: []Action{{"clock_in", "Clock in"}}}

	var out []Notification
	add := func(n Notification, at time.Time, edit func(*Moment)) {
		m := base
		m.At = at
		if edit != nil {
			edit(&m)
		}
		n = Speak(n, m)
		if n.Kind == "idle" {
			n.Until = now.Add(m.HardIdle - m.SoftIdle)
		}
		if m.BreakFor > 0 {
			n.StartedAt = now.Add(-m.BreakFor)
		}
		out = append(out, n)
	}
	add(idle, day(15, 0), nil)
	add(idle, day(23, 30), nil)
	for i, took := range []time.Duration{0, 1, 2, 6} {
		add(long, day(15, i), func(m *Moment) { m.BreakFor = base.BreakReminder + took*base.Repeat })
	}
	add(long, day(16, 0), func(m *Moment) { m.BreakFor = 40 * time.Minute; m.Worked = m.Target })
	add(active, day(15, 0), func(m *Moment) { m.BreakFor = 3 * time.Minute })
	add(clockIn, day(8, 0), func(m *Moment) { m.Briefing = "2 planned today · 1 reminder" })
	add(clockIn, day(19, 0), nil)
	return out
}

// Preview shows every nudge Gwen can send, one after another, on the card or
// as desktop notifications; never on the phone. A new preview replaces one
// still running. It returns what will show, in order.
func (d *Dispatcher) Preview() []wire.Nudge {
	cfg := d.config()
	now := d.now()
	ns := previews(cfg, now)
	ctx, cancel := context.WithCancel(context.Background())
	d.mu.Lock()
	if d.stopPreview != nil {
		d.stopPreview()
	}
	d.stopPreview = cancel
	d.mu.Unlock()

	out := make([]wire.Nudge, len(ns))
	for i, n := range ns {
		n.At, n.Preview = now.Add(time.Duration(i)*previewStep), true
		out[i] = toWire(n)
	}
	if !cfg.Nudge.Desktop {
		return out
	}
	go func() { // not on the wait group: shutdown need not sit through it
		for i, n := range ns {
			if i > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(previewStep):
				}
			}
			// Its times count from the moment it shows.
			at := d.now()
			n.At, n.Preview, n.Expires = at, true, at.Add(previewStep+2*time.Second)
			if shift := at.Sub(now); shift > 0 {
				if !n.StartedAt.IsZero() {
					n.StartedAt = n.StartedAt.Add(shift)
				}
				if !n.Until.IsZero() {
					n.Until = n.Until.Add(shift)
				}
			}
			if d.show(n) {
				d.chime()
				continue
			}
			native := n
			native.Kind = previewKey // each replaces the last
			d.async(d.desktop, func(ctx context.Context) error { return d.desktop.Notify(ctx, native) })
		}
	}()
	return out
}
