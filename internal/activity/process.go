package activity

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/kzark/gwen/internal/clock"
)

// pollInterval is how often the polling backends sample the idle time.
const pollInterval = 10 * time.Second

// runner starts external programs; tests replace it.
type runner interface {
	LookPath(name string) error
	// Output runs a program to completion and returns its standard output.
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
	// Stream starts a long-running program and returns its standard output,
	// which ends when the program exits or ctx is cancelled.
	Stream(ctx context.Context, name string, args ...string) (io.ReadCloser, error)
}

type execRunner struct{}

func (execRunner) LookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}

func (execRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func (execRunner) Stream(ctx context.Context, name string, args ...string) (io.ReadCloser, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return waitCloser{ReadCloser: out, wait: cmd.Wait}, nil
}

type waitCloser struct {
	io.ReadCloser
	wait func() error
}

func (w waitCloser) Close() error {
	w.ReadCloser.Close()
	return w.wait()
}

// swayidleProc spawns swayidle, which reports each threshold by running the
// echo commands Gwen gives it.
type swayidleProc struct {
	clk  clock.Clock
	exec runner
}

func probeSwayidle(_ context.Context, clk clock.Clock, r runner) (idleBackend, error) {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return nil, errors.New("not a Wayland session")
	}
	if err := r.LookPath("swayidle"); err != nil {
		return nil, err
	}
	return swayidleProc{clk: clk, exec: r}, nil
}

func (swayidleProc) name() string { return NameSwayidle }

// swayidleArgs asks swayidle to print "idle <ms>" and "active <ms>" per
// threshold. swayidle counts whole seconds, so thresholds round up.
func swayidleArgs(thresholds []time.Duration) []string {
	args := []string{"-w"}
	for _, th := range thresholds {
		secs := int64((th + time.Second - 1) / time.Second)
		ms := th.Milliseconds()
		args = append(args, "timeout", strconv.FormatInt(max(secs, 1), 10),
			fmt.Sprintf("echo idle %d", ms), "resume", fmt.Sprintf("echo active %d", ms))
	}
	return args
}

// parseSwayidle reads one line of swayidle's output.
func parseSwayidle(line string) (idle bool, threshold time.Duration, ok bool) {
	fields := strings.Fields(line)
	if len(fields) != 2 || (fields[0] != "idle" && fields[0] != "active") {
		return false, 0, false
	}
	ms, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || ms <= 0 {
		return false, 0, false
	}
	return fields[0] == "idle", time.Duration(ms) * time.Millisecond, true
}

func (s swayidleProc) run(ctx context.Context, thresholds []time.Duration, emit func(Event)) error {
	out, err := s.exec.Stream(ctx, "swayidle", swayidleArgs(thresholds)...)
	if err != nil {
		return fmt.Errorf("start swayidle: %w", err)
	}
	defer out.Close()
	t := &tracker{clk: s.clk, emit: emit}
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		idle, th, ok := parseSwayidle(sc.Text())
		switch {
		case !ok:
		case idle:
			t.idled(th)
		default:
			t.resumed()
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read swayidle: %w", err)
	}
	return errors.New("swayidle exited")
}

// poller turns idle-time samples into events. A sample gives the last input
// instant, now − idle, so events carry the instant the idle time says, not
// the poll time.
type poller struct {
	thresholds []time.Duration // ascending
	fired      map[time.Duration]bool
	lastInput  time.Time
	anyFired   bool
}

// inputJitter absorbs rounding in reported idle times when deciding whether
// input happened between two samples.
const inputJitter = time.Second

func newPoller(thresholds []time.Duration) *poller {
	return &poller{thresholds: thresholds, fired: map[time.Duration]bool{}}
}

func (p *poller) observe(idle time.Duration, now time.Time) []Event {
	lastInput := now.Add(-idle)
	var out []Event
	if !p.lastInput.IsZero() && lastInput.Sub(p.lastInput) > inputJitter {
		// There was input since the previous sample: a new idle period.
		if p.anyFired {
			out = append(out, Event{Kind: Active, At: lastInput})
		}
		p.fired, p.anyFired = map[time.Duration]bool{}, false
	}
	if p.lastInput.IsZero() || lastInput.Sub(p.lastInput) > inputJitter {
		p.lastInput = lastInput
	}
	for _, th := range p.thresholds {
		if !p.fired[th] && idle >= th {
			p.fired[th], p.anyFired = true, true
			out = append(out, Event{Kind: Idle, Threshold: th, At: p.lastInput.Add(th)})
		}
	}
	return out
}

// pollLoop samples every pollInterval, the first time at once.
func pollLoop(ctx context.Context, clk clock.Clock, thresholds []time.Duration, emit func(Event),
	sample func(context.Context) (time.Duration, error)) error {
	p := newPoller(thresholds)
	timer := clk.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C():
		}
		idle, err := sample(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		for _, ev := range p.observe(idle, clk.Now()) {
			emit(ev)
		}
		timer.Reset(pollInterval)
	}
}

// xprintidle polls the xprintidle program, which prints idle milliseconds.
type xprintidle struct {
	clk  clock.Clock
	exec runner
}

func probeXprintidle(ctx context.Context, clk clock.Clock, r runner) (idleBackend, error) {
	if os.Getenv("DISPLAY") == "" {
		return nil, errors.New("not an X11 session")
	}
	x := xprintidle{clk: clk, exec: r}
	if _, err := x.sample(ctx); err != nil {
		return nil, err
	}
	return x, nil
}

func (xprintidle) name() string { return NameXprintidle }

func (x xprintidle) run(ctx context.Context, thresholds []time.Duration, emit func(Event)) error {
	return pollLoop(ctx, x.clk, thresholds, emit, x.sample)
}

func (x xprintidle) sample(ctx context.Context) (time.Duration, error) {
	out, err := x.exec.Output(ctx, "xprintidle")
	if err != nil {
		return 0, fmt.Errorf("xprintidle: %w", err)
	}
	return parseXprintidle(string(out))
}

func parseXprintidle(s string) (time.Duration, error) {
	ms, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || ms < 0 {
		return 0, fmt.Errorf("xprintidle printed %q", strings.TrimSpace(s))
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// idleTimeSource answers org.freedesktop.ScreenSaver.GetSessionIdleTime.
type idleTimeSource interface {
	IdleTime(ctx context.Context) (time.Duration, error)
}

// dbusScreensaver polls the session's screensaver for the idle time. KDE on
// X11 answers in milliseconds; KDE on Wayland answers NotSupported, which
// fails the probe.
type dbusScreensaver struct {
	clk clock.Clock
	src idleTimeSource
}

func probeScreensaver(ctx context.Context, clk clock.Clock) (idleBackend, error) {
	src, err := newScreensaverBus()
	if err != nil {
		return nil, err
	}
	return probeScreensaverWith(ctx, clk, src)
}

func probeScreensaverWith(ctx context.Context, clk clock.Clock, src idleTimeSource) (idleBackend, error) {
	if _, err := src.IdleTime(ctx); err != nil {
		return nil, err
	}
	return dbusScreensaver{clk: clk, src: src}, nil
}

func (dbusScreensaver) name() string { return NameDBusScreensaver }

func (d dbusScreensaver) run(ctx context.Context, thresholds []time.Duration, emit func(Event)) error {
	return pollLoop(ctx, d.clk, thresholds, emit, d.src.IdleTime)
}

type screensaverBus struct{ obj dbus.BusObject }

func newScreensaverBus() (screensaverBus, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return screensaverBus{}, fmt.Errorf("session bus: %w", err)
	}
	return screensaverBus{obj: conn.Object("org.freedesktop.ScreenSaver", "/org/freedesktop/ScreenSaver")}, nil
}

func (s screensaverBus) IdleTime(ctx context.Context) (time.Duration, error) {
	var ms uint32
	if err := s.obj.CallWithContext(ctx, "org.freedesktop.ScreenSaver.GetSessionIdleTime", 0).Store(&ms); err != nil {
		return 0, fmt.Errorf("GetSessionIdleTime: %w", err)
	}
	return time.Duration(ms) * time.Millisecond, nil
}
