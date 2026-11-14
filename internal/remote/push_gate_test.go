package remote

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// pushRig is a Service that is only asked to push: with no phone subscribed a
// push goes to opts.Push, which records it.
type pushRig struct {
	svc   *Service
	clk   *clock.Fake
	away  bool
	phone bool

	mu   sync.Mutex
	sent []string
	got  chan struct{}
}

func newPushRig(t *testing.T, withPhoneAway bool) *pushRig {
	t.Helper()
	r := &pushRig{clk: clock.NewFake(testutil.At("09:00:00")), got: make(chan struct{}, 16)}
	cfg := config.Defaults()
	cfg.Remote.Push = true
	opts := Options{Config: func() config.Config { return cfg }, CredDir: t.TempDir(),
		Away: func() bool { return r.away },
		Push: func(_ context.Context, title, message, _ string) error {
			r.mu.Lock()
			r.sent = append(r.sent, title+": "+message)
			r.mu.Unlock()
			r.got <- struct{}{}
			return nil
		}}
	if withPhoneAway {
		opts.PhoneAway = func() bool { return r.phone }
	}
	r.svc = &Service{opts: opts, cfg: opts.Config, clk: r.clk}
	return r
}

// push pushes about sess and returns what was sent, "" when nothing was.
func (r *pushRig) push(sess *session) string {
	r.svc.mu.Lock()
	r.svc.push(sess)
	r.svc.mu.Unlock()
	select {
	case <-r.got:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.sent[len(r.sent)-1]
	case <-time.After(200 * time.Millisecond):
		return ""
	}
}

func pushSession(state string) *session {
	return &session{CodeSession: store.CodeSession{ID: "cs_push", Cwd: "/home/u/code/app", State: state}}
}

func TestPhoneWaitsUntilYouAreReallyAway(t *testing.T) {
	r := newPushRig(t, true)
	require.Empty(t, r.push(pushSession(store.CodeDone)), "at the laptop, even still for a few seconds: no phone push")

	r.phone = true
	require.Equal(t, "app: finished", r.push(pushSession(store.CodeDone)), "locked or idle two minutes")
}

func TestPhoneAwayIsIndependentOfTheAwayHold(t *testing.T) {
	r := newPushRig(t, true)
	r.away = true
	require.Empty(t, r.push(pushSession(store.CodeDone)), "Away holds permissions; it does not decide pushes")
	r.away, r.phone = false, true
	require.Equal(t, "app: finished", r.push(pushSession(store.CodeDone)))
}

func TestPhoneAwayFallsBackToAway(t *testing.T) {
	r := newPushRig(t, false)
	require.Empty(t, r.push(pushSession(store.CodeDone)))
	r.away = true
	require.Equal(t, "app: finished", r.push(pushSession(store.CodeDone)))
}

func TestPushIsOffWhenNotificationsAreOff(t *testing.T) {
	r := newPushRig(t, true)
	r.phone = true
	cfg := config.Defaults()
	cfg.Remote.Push = false
	r.svc.cfg = func() config.Config { return cfg }
	require.Empty(t, r.push(pushSession(store.CodeDone)))
}

func TestPushSaysEachStateOnceButNeverHidesAFollowingOne(t *testing.T) {
	r := newPushRig(t, true)
	r.phone = true
	sess := pushSession(store.CodeNeedsYou)
	// needOf reads the held request or the waiting kind; a permission card has one.
	sess.waiting = &wire.CodeWaiting{Kind: wire.WaitingPermission}
	require.Equal(t, "app: needs permission", r.push(sess))

	// The same words again within the half minute are one push, not two.
	r.clk.Advance(10 * time.Second)
	require.Empty(t, r.push(sess))

	// "Finished" a moment after "needs permission" is news: it was dropped before.
	r.clk.Advance(5 * time.Second)
	sess.State, sess.waiting = store.CodeDone, nil
	require.Equal(t, "app: finished", r.push(sess))

	// And the first words may be said again once the half minute is over.
	r.clk.Advance(pushEvery + time.Second)
	require.Equal(t, "app: finished", r.push(sess))
}

// payOwed runs one janitor pass of the held-back phone pushes and returns what
// went out, "" when nothing did.
func (r *pushRig) payOwed() string {
	r.svc.mu.Lock()
	r.svc.payPhone()
	r.svc.mu.Unlock()
	select {
	case <-r.got:
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.sent[len(r.sent)-1]
	case <-time.After(200 * time.Millisecond):
		return ""
	}
}

func TestPhoneIsToldOnceYouTurnOutToBeAway(t *testing.T) {
	r := newPushRig(t, true)
	sess := pushSession(store.CodeNeedsYou)
	sess.waiting = &wire.CodeWaiting{Kind: wire.WaitingQuestion}
	sess.lastTool = "AskUserQuestion"
	r.svc.sessions = map[string]*session{sess.ID: sess}

	require.Empty(t, r.push(sess), "idle only seconds: no push yet")
	require.Empty(t, r.payOwed(), "still at the laptop")

	r.phone = true
	require.Equal(t, "app: has a question", r.payOwed(), "away two minutes and Claude still waits")
	require.Empty(t, r.payOwed(), "told once")
}

func TestPhoneIsNotToldAboutWhatYouAnsweredMeanwhile(t *testing.T) {
	r := newPushRig(t, true)
	sess := pushSession(store.CodeNeedsYou)
	r.svc.sessions = map[string]*session{sess.ID: sess}
	require.Empty(t, r.push(sess))

	r.svc.mu.Lock()
	r.svc.setState(sess, store.CodeWorking)
	r.svc.mu.Unlock()
	r.phone = true
	require.Empty(t, r.payOwed(), "Claude carried on, nothing to tell")
}

// alertRig records the desktop alerts and withdrawals of a pushRig.
func (r *pushRig) alerts() (*[]DesktopAlert, *int) {
	var got []DesktopAlert
	resolved := 0
	r.svc.opts.Desktop = func(a DesktopAlert) { got = append(got, a) }
	r.svc.opts.Resolved = func() { resolved++ }
	return &got, &resolved
}

func TestAlertSaysTheQuestionAndOffersItsChoices(t *testing.T) {
	r := newPushRig(t, true)
	got, _ := r.alerts()
	sess := pushSession(store.CodeNeedsYou)
	r.svc.sessions = map[string]*session{sess.ID: sess}

	// Nothing is known yet: the alert waits for readDialog.
	r.svc.mu.Lock()
	r.svc.push(sess)
	r.svc.mu.Unlock()
	require.Empty(t, *got)

	sess.waiting = &wire.CodeWaiting{Kind: wire.WaitingQuestion, Question: "Ship it?", Options: []wire.CodeOption{
		{Key: "option:1", Label: "Yes"}, {Key: "option:2", Label: "No"}}}
	r.svc.flushAlert(sess.ID)
	require.Equal(t, []DesktopAlert{{Title: "app", Message: "needs you", Session: "cs_push", Detail: "Ship it?",
		Choices: sess.waiting.Options}}, *got)

	r.svc.flushAlert(sess.ID)
	require.Len(t, *got, 1, "told once")
}

func TestAlertForAHeldPermissionGoesAtOnceWithAllowAndDeny(t *testing.T) {
	r := newPushRig(t, true)
	got, _ := r.alerts()
	sess := pushSession(store.CodeNeedsYou)
	sess.held = &hold{}
	sess.waiting = &wire.CodeWaiting{Kind: wire.WaitingPermission, Summary: "Bash: go test", Options: []wire.CodeOption{
		{Key: "allow", Label: "Allow once"}, {Key: "deny", Label: "Deny"}}}
	r.svc.mu.Lock()
	r.svc.push(sess)
	r.svc.mu.Unlock()
	require.Len(t, *got, 1)
	require.Equal(t, "Bash: go test", (*got)[0].Detail)
	require.Equal(t, "allow", (*got)[0].Choices[0].Key)
}

func TestAlertIsWithdrawnWhenNothingNeedsYouAnyMore(t *testing.T) {
	r := newPushRig(t, true)
	_, resolved := r.alerts()
	a, b := pushSession(store.CodeNeedsYou), pushSession(store.CodeNeedsYou)
	b.ID = "cs_other"
	r.svc.sessions = map[string]*session{a.ID: a, b.ID: b}
	r.svc.mu.Lock()
	r.svc.setState(a, store.CodeWorking)
	require.Zero(t, *resolved, "another session still needs you")
	r.svc.setState(b, store.CodeWorking)
	r.svc.mu.Unlock()
	require.Equal(t, 1, *resolved)
}
