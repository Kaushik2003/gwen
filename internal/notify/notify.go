// Package notify sends nudges to the desktop and the phone: the Notifier port
// of docs/02-architecture.md and the backends and routing of
// docs/07-integrations.md#notifications.
package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
)

// Notification is one nudge, with its text already rendered. Body is plain
// text; Markup, when set, is the same body with <b>, <i>, and <br/> for
// desktops that render it. Mood names Gwen's face to show (faces/). At is
// when it fired; Until, when set, is when it stops applying (an idle nudge's
// moment of becoming a break); StartedAt is when the break it is about began.
// Expires, when set, is when the card drops it on its own (a test or a
// preview); a Preview's buttons do nothing.
type Notification struct {
	Kind, Title, Body string
	Markup, Mood      string
	Actions           []Action
	At, Until         time.Time
	StartedAt         time.Time
	Expires           time.Time
	Preview           bool
}

// Action is a notification button.
type Action struct{ ID, Label string }

// ActionEvent is a button the user clicked.
type ActionEvent struct{ Kind, Action string }

// Notifier is one notification backend.
type Notifier interface {
	Notify(ctx context.Context, n Notification) error
	// Withdraw closes the kind's notification if still shown; a no-op where
	// the backend cannot retract.
	Withdraw(ctx context.Context, kind string) error
	Name() string
}

// KindTest is the kind of the notification POST /v1/notify/test sends.
const KindTest = "test"

// phoneKinds are the nudges that also go to the phone (docs/05-time-engine.md#nudges).
var phoneKinds = map[string]bool{"idle": true, "break_long": true}

// backendTimeout bounds each backend per notification.
const backendTimeout = 10 * time.Second

// configurable backends follow the configuration.
type configurable interface{ SetConfig(config.Config) }

// Dispatcher fans each notification out to the desktop and the phone
// concurrently. Neither blocks the other or the caller; each is bounded by a
// timeout, and failures are logged at Warn.
//
// It also keeps the nudges showing for Gwen's panel card (card.go): while the
// card is polling, desktop nudges go to it instead of the notification server.
type Dispatcher struct {
	desktop Notifier
	phone   Notifier
	actions chan ActionEvent
	log     *slog.Logger
	timeout time.Duration
	now     func() time.Time

	mu     sync.Mutex
	cfg    config.Config
	shown  map[string]Notification // kind, or previewKey → the nudge showing
	cardAt time.Time               // when the card last polled
	wg     sync.WaitGroup

	stopPreview context.CancelFunc // ends a preview still running
}

// NewDispatcher routes to desktop and phone per cfg. actions delivers the
// desktop's button clicks; it may be nil.
func NewDispatcher(desktop, phone Notifier, actions <-chan ActionEvent, cfg config.Config) *Dispatcher {
	d := &Dispatcher{desktop: desktop, phone: phone, actions: make(chan ActionEvent, 16), log: slog.Default(),
		timeout: backendTimeout, now: time.Now, shown: map[string]Notification{}}
	if actions != nil {
		go func() {
			for a := range actions {
				d.actions <- a
			}
		}()
	}
	d.SetConfig(cfg)
	return d
}

// SetConfig applies a new configuration immediately.
func (d *Dispatcher) SetConfig(cfg config.Config) {
	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()
	for _, b := range []Notifier{d.desktop, d.phone} {
		if c, ok := b.(configurable); ok {
			c.SetConfig(cfg)
		}
	}
}

func (d *Dispatcher) config() config.Config {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.cfg
}

// Actions delivers notification and card button clicks.
func (d *Dispatcher) Actions() <-chan ActionEvent { return d.actions }

// Send delivers a nudge to every backend its routing allows, returning at
// once: desktop when nudge.desktop is on; phone when nudge.phone is on, a topic
// is set, and the kind goes to the phone.
func (d *Dispatcher) Send(n Notification) {
	cfg := d.config()
	if n.At.IsZero() {
		n.At = d.now()
	}
	if cfg.Nudge.Desktop {
		if d.show(n) {
			d.chime()
		} else {
			d.async(d.desktop, func(ctx context.Context) error { return d.desktop.Notify(ctx, n) })
		}
	}
	if cfg.Nudge.Phone && cfg.Ntfy.Topic != "" && phoneKinds[n.Kind] {
		d.async(d.phone, func(ctx context.Context) error { return d.phone.Notify(ctx, n) })
	}
}

// Withdraw closes the kind's desktop notification, returning at once.
func (d *Dispatcher) Withdraw(kind string) {
	d.mu.Lock()
	delete(d.shown, kind)
	d.mu.Unlock()
	d.async(d.desktop, func(ctx context.Context) error { return d.desktop.Withdraw(ctx, kind) })
}

func (d *Dispatcher) async(b Notifier, fn func(context.Context) error) {
	d.wg.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- fn(ctx) }()
		select {
		case err := <-done:
			if err != nil {
				d.log.Warn("notification failed", "backend", b.Name(), "err", err)
			}
		case <-ctx.Done():
			d.log.Warn("notification abandoned", "backend", b.Name(), "after", d.timeout)
		}
	})
}

// Wait blocks until every notification in flight has finished or been
// abandoned.
func (d *Dispatcher) Wait() { d.wg.Wait() }

// Test sends the test notification through each backend, ignoring snooze and
// nudge state, and reports each outcome (docs/04-api-contract.md#notifications).
func (d *Dispatcher) Test(ctx context.Context) wire.NotifyTestResult {
	cfg := d.config()
	now := d.now()
	n := Notification{Kind: KindTest, Title: "It's me, Gwen!", Body: "If you can see my face, notifications are working.", Mood: "winking",
		At: now, Expires: now.Add(testShown)}
	res := wire.NotifyTestResult{Desktop: wire.NotifyDisabled, Phone: wire.NotifyDisabled}
	var wg sync.WaitGroup
	if cfg.Nudge.Desktop {
		if d.show(n) {
			res.Desktop = wire.NotifyOK
			d.chime()
		} else {
			wg.Go(func() { res.Desktop = d.try(ctx, d.desktop, n) })
		}
	}
	if cfg.Nudge.Phone && cfg.Ntfy.Topic != "" {
		wg.Go(func() { res.Phone = d.try(ctx, d.phone, n) })
	}
	wg.Wait()
	return res
}

func (d *Dispatcher) try(ctx context.Context, b Notifier, n Notification) string {
	ctx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- b.Notify(ctx, n) }()
	select {
	case err := <-done:
		if err != nil {
			return err.Error()
		}
		return wire.NotifyOK
	case <-ctx.Done():
		return "timed out"
	}
}
