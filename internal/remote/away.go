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

// push tells the phone that a session needs you, finished, or failed, when
// you are away. The message never carries a prompt, command, path, or code:
// ntfy.sh is a public server (plan.md 3.14.11). s.mu is held.
func (s *Service) push(sess *session) {
	if s.opts.Push == nil || !s.cfg().Remote.Push || !s.away() {
		return
	}
	now := s.clk.Now()
	if now.Sub(sess.pushedAt) < pushEvery {
		return
	}
	sess.pushedAt = now
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
	title, id, port := filepath.Base(sess.Cwd), sess.ID, s.cfg().Remote.Port
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		click := ""
		if host := tailscaleStatus(ctx).Host; host != "" {
			click = fmt.Sprintf("https://%s:%d/s/%s", host, port, id)
		}
		if err := s.opts.Push(ctx, title, message, click); err != nil {
			slog.Debug("push", "err", err)
		}
	}()
}
