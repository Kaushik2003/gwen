package notify

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/wire"
)

// Gwen's panel card is the KDE panel widget showing nudges in its own popup
// (packaging/plasma/dev.gwen.panel), styled as the dashboard. It polls for
// the nudges showing; while it does, desktop nudges go to it alone, and the
// notification server shows them only once it stops (on other desktops, or
// with the widget removed).

// cardFresh is how long after its last poll the card counts as present.
const cardFresh = 10 * time.Second

// testShown is how long the test nudge stays on the card.
const testShown = 15 * time.Second

// ErrNoSuchNudge is an action on a nudge that is not showing, or a button it
// does not have.
var ErrNoSuchNudge = errors.New("no such nudge")

// windowFresh is how long after its last word the Gwen window counts as
// having your focus; it repeats itself while it does.
const windowFresh = 45 * time.Second

// WindowFocus records whether the Gwen window has your focus (POST
// /v1/window/focus): while it does, nudges show in the window alone.
func (d *Dispatcher) WindowFocus(focused bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if focused {
		d.windowAt = d.now()
	} else {
		d.windowAt = time.Time{}
	}
}

// showQuietly records n as showing for the window alone when the window has
// your focus, and reports whether it did. A preview always shows.
func (d *Dispatcher) showQuietly(n Notification) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n.Preview || d.windowAt.IsZero() || d.now().Sub(d.windowAt) >= windowFresh {
		return false
	}
	d.shown[n.Kind] = n
	d.quiet[n.Kind] = true
	return true
}

// show records n as showing and reports whether the card will show it.
func (d *Dispatcher) show(n Notification) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	key := n.Kind
	if n.Preview {
		key = previewKey
	}
	d.shown[key] = n
	delete(d.quiet, key)
	return !d.cardAt.IsZero() && d.now().Sub(d.cardAt) < cardFresh
}

// chimer is a desktop backend that can play the nudge sound on its own, for
// nudges the card shows.
type chimer interface {
	Chime(ctx context.Context) error
}

// chime plays the nudge sound for a nudge the card shows.
func (d *Dispatcher) chime() {
	if c, ok := d.desktop.(chimer); ok {
		d.async(d.desktop, c.Chime)
	}
}

// Nudges are the nudges showing, newest first. card says the caller is the
// panel card, which then shows desktop nudges in place of the notification
// server; it doesn't get the ones that came while you were in the Gwen
// window.
func (d *Dispatcher) Nudges(card bool) []wire.Nudge {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	if card {
		d.cardAt = now
	}
	out := []wire.Nudge{}
	for key, n := range d.shown {
		if !n.Expires.IsZero() && !n.Expires.After(now) {
			delete(d.shown, key)
			delete(d.quiet, key)
			continue
		}
		if card && d.quiet[key] {
			continue
		}
		out = append(out, toWire(n))
	}
	slices.SortFunc(out, func(a, b wire.Nudge) int { return cmp.Compare(b.At, a.At) })
	return out
}

func toWire(n Notification) wire.Nudge {
	w := wire.Nudge{Kind: n.Kind, Title: n.Title, Body: n.Body, Markup: n.Markup, Mood: n.Mood, At: wire.Millis(n.At),
		Actions: []wire.NudgeAction{}, Preview: n.Preview}
	if !n.Until.IsZero() {
		until := wire.Millis(n.Until)
		w.UntilAt = &until
	}
	if !n.StartedAt.IsZero() {
		started := wire.Millis(n.StartedAt)
		w.StartedAt = &started
	}
	for _, a := range n.Actions {
		w.Actions = append(w.Actions, wire.NudgeAction{ID: a.ID, Label: a.Label})
	}
	return w
}

// Act clicks a button of a showing nudge, as a click on the desktop
// notification would. A preview's buttons only close it.
func (d *Dispatcher) Act(kind, action string) error {
	d.mu.Lock()
	n, ok := d.shown[kind]
	if ok && kind == previewKey {
		delete(d.shown, kind)
	}
	d.mu.Unlock()
	if ok && kind == previewKey {
		return nil
	}
	if !ok || !slices.ContainsFunc(n.Actions, func(a Action) bool { return a.ID == action }) {
		return ErrNoSuchNudge
	}
	select {
	case d.actions <- ActionEvent{Kind: kind, Action: action}:
		return nil
	default:
		return errors.New("too many clicks waiting")
	}
}
