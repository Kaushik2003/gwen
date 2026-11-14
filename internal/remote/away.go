package remote

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// pushEvery is the least time between two pushes about one session.
const pushEvery = 30 * time.Second

// lowBattery is the charge below which Gwen stops keeping the laptop awake
// on battery.
const lowBattery = 15

// inhibitor is a logind block inhibitor held while a session works
// (plan.md 3.14.11).
type inhibitor struct {
	mu   sync.Mutex
	fd   int
	what string
}

// set takes or releases the inhibitor: held while busy, in the mode the
// configuration names, and never on a battery that is nearly empty.
func (i *inhibitor) set(busy bool, mode string) {
	want := ""
	if busy && !batteryLow() {
		switch mode {
		case config.KeepAwakeWorking:
			want = "sleep:idle"
		case config.KeepAwakeWorkingLid:
			want = "sleep:idle:handle-lid-switch"
		}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if want == i.what {
		return
	}
	if i.what != "" {
		_ = syscall.Close(i.fd)
		i.what = ""
	}
	if want == "" {
		return
	}
	conn, err := dbus.SystemBus()
	if err != nil {
		return
	}
	var fd dbus.UnixFD
	err = conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		Call("org.freedesktop.login1.Manager.Inhibit", 0, want, "Gwen", "A Claude session is working", "block").Store(&fd)
	if err != nil {
		slog.Debug("keep awake", "err", err)
		return
	}
	i.fd, i.what = int(fd), want
}

func (i *inhibitor) release() { i.set(false, config.KeepAwakeNever) }

// batteryLow reports a battery that is discharging below lowBattery percent.
func batteryLow() bool {
	supplies, _ := filepath.Glob("/sys/class/power_supply/BAT*")
	for _, dir := range supplies {
		status, err := os.ReadFile(filepath.Join(dir, "status"))
		if err != nil || strings.TrimSpace(string(status)) != "Discharging" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, "capacity"))
		if err != nil {
			continue
		}
		if pct, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && pct < lowBattery {
			return true
		}
	}
	return false
}

// pushWanted reports that a phone notification should go out now: they are
// turned on, and you are away from the laptop (locked or idle two minutes).
// Being at it, even idle for a few seconds, is left to the desktop.
func (s *Service) pushWanted() bool {
	if !s.cfg().Remote.Push {
		return false
	}
	if s.opts.PhoneAway != nil {
		return s.opts.PhoneAway()
	}
	return s.away()
}

// push tells the phone that a session needs you, finished, or failed, unless
// you are at the laptop. The message never carries a prompt, command, path,
// or code: ntfy.sh is a public server (plan.md 3.14.11). s.mu is held.
func (s *Service) push(sess *session) {
	message := "finished"
	switch sess.State {
	case store.CodeError:
		message = "stopped with an error"
	case store.CodeNeedsYou:
		switch s.needOf(sess) {
		case store.NeedPermission:
			message = "needs permission"
		case store.NeedQuestion:
			message = "has a question"
		default:
			message = "needs you"
		}
	}
	s.pushAs(sess, message, sess.State == store.CodeNeedsYou)
}

// DesktopAlert is a desktop notification about a session: Detail is what it
// asks (the question, or the command to allow) and Choices are the buttons
// that answer it, in the keys Answer takes.
type DesktopAlert struct {
	Title, Message, Session, Detail string
	Choices                         []wire.CodeOption
}

// maxAlertChoices is how many answer buttons a notification carries.
const maxAlertChoices = 4

// alertNow shows the desktop alert for sess with what is known of it. s.mu is
// held.
func (s *Service) alertNow(sess *session, message string) {
	sess.alertMsg = ""
	if s.opts.Desktop == nil {
		return
	}
	a := DesktopAlert{Title: filepath.Base(sess.Cwd), Message: message, Session: sess.ID}
	if w := sess.waiting; w != nil {
		a.Detail = w.Question
		if a.Detail == "" {
			a.Detail = w.Summary
		}
		for _, o := range w.Options {
			if len(a.Choices) < maxAlertChoices && o.Key != "" && o.Label != "" {
				a.Choices = append(a.Choices, o)
			}
		}
	}
	s.opts.Desktop(a)
}

// alert shows the alert that waited for readDialog. s.mu is held.
func (s *Service) alert(sess *session) {
	if sess.alertMsg != "" {
		s.alertNow(sess, sess.alertMsg)
	}
}

// flushAlert is readDialog's end: the alert it held back goes out now.
func (s *Service) flushAlert(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[id]; sess != nil && sess.State == store.CodeNeedsYou {
		s.alert(sess)
	}
}

// resolved withdraws the desktop alert once no session needs you. s.mu is held.
func (s *Service) resolved() {
	if s.opts.Resolved == nil {
		return
	}
	for _, sess := range s.sessions {
		if sess.State == store.CodeNeedsYou {
			return
		}
	}
	s.opts.Resolved()
}

// pushAs tells you message about sess: a desktop notification (the dispatcher
// keeps it quiet while you are in the Gwen window) and, unless you are at the
// laptop, the phone. The same words about the same session are sent once in a
// while; a different one ("needs permission", then "finished") always goes.
// s.mu is held.
func (s *Service) pushAs(sess *session, message string, needsYou bool) {
	now := s.clk.Now()
	if message == sess.pushedMsg && now.Sub(sess.pushedAt) < pushEvery {
		return
	}
	sess.pushedAt, sess.pushedMsg = now, message
	title := filepath.Base(sess.Cwd)
	switch {
	case !needsYou:
		s.alertNow(sess, message)
	case sess.held != nil || sess.waiting != nil && (sess.waiting.Question != "" || len(sess.waiting.Options) > 0):
		s.alertNow(sess, message) // what it asks is known already
	default:
		// readDialog is reading the laptop's dialog; the alert goes out with
		// what it asks and the choices, as soon as it has.
		sess.alertMsg = message
	}
	if s.pushWanted() {
		s.sendPhone(pushMessage{Title: title, Body: message, URL: "/s/" + sess.ID, Tag: "session:" + sess.ID})
		sess.phoneOwed = ""
	} else {
		// Still at the laptop (idle under two minutes): the phone is owed
		// this until you are away and the session still waits.
		sess.phoneOwed = message
	}
}

// payPhone sends the phone pushes that were held back while you were at the
// laptop, now that you are away and the session still needs you (or ended
// in a state worth telling). s.mu is held.
func (s *Service) payPhone() {
	if !s.pushWanted() {
		return
	}
	for _, sess := range s.sessions {
		if sess.phoneOwed == "" {
			continue
		}
		msg := sess.phoneOwed
		sess.phoneOwed = ""
		if sess.State == store.CodeWorking || sess.State == store.CodeEnded {
			continue
		}
		s.sendPhone(pushMessage{Title: filepath.Base(sess.Cwd), Body: msg, URL: "/s/" + sess.ID, Tag: "session:" + sess.ID})
	}
}

// PushPhone tells the phone something needs you, unless you are at the
// laptop: title and body in a few plain words (never a prompt, command, or
// code), path the place in the phone app a tap opens, and tag what it is
// about, so a newer push about the same thing replaces it.
func (s *Service) PushPhone(title, body, path, tag string) {
	if !s.pushWanted() {
		return
	}
	s.sendPhone(pushMessage{Title: title, Body: body, URL: path, Tag: tag})
}

// sendPhone sends m to the phones that turned notifications on in the phone
// app, or through ntfy when none did. It returns at once.
func (s *Service) sendPhone(m pushMessage) {
	port := s.cfg().Remote.Port
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if s.webPush(ctx, m, "") > 0 || s.opts.Push == nil {
			return
		}
		click := ""
		if host := tailscaleStatus(ctx).Host; host != "" {
			click = fmt.Sprintf("https://%s:%d%s", host, port, m.URL)
		}
		if err := s.opts.Push(ctx, m.Title, m.Body, click); err != nil {
			slog.Debug("push", "err", err)
		}
	}()
}
