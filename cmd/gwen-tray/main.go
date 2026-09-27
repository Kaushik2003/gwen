// Command gwen-tray is Gwen's tray icon: the current state at a glance and the
// everyday commands (docs/08-clients.md#tray).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"

	"fyne.io/systray"
	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/packaging/icons/tray"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("gwen-tray", version)
		return
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		fmt.Fprintln(os.Stderr, "gwen-tray:", err)
		os.Exit(1)
	}
	unlock, err := singleInstance(filepath.Join(paths.RuntimeDir, "tray.lock"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gwen-tray:", err)
		os.Exit(0) // another tray is already showing: not an error
	}
	defer unlock()

	ctx, cancel := context.WithCancel(context.Background())
	sv := &systrayView{}
	c := &controller{api: client.New(paths.SocketPath()), clk: clock.Real(), view: sv, exec: execRunner{}}
	systray.Run(func() {
		sv.build(c, cancel)
		go c.run(ctx)
	}, cancel)
}

// singleInstance holds an exclusive lock so a second tray exits: gwen setup
// starts one even when one runs already.
func singleInstance(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("the tray is already running")
	}
	return func() { f.Close() }, nil
}

type execRunner struct{}

func (execRunner) start(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap it
	return nil
}

// systrayView is the fyne.io/systray menu. Items are created once and shown or
// hidden, since systray cannot remove them.
type systrayView struct {
	mu                                                   sync.Mutex
	status, clockIn, startBreak, endBreak, switchProject *systray.MenuItem
	snooze, clockOut, open, startGwen, quit              *systray.MenuItem
	projectItems                                         []*systray.MenuItem
	projectIDs                                           []*string
	icon                                                 string
	built                                                bool
	pending                                              *Menu
}

func (v *systrayView) build(c *controller, quit context.CancelFunc) {
	v.mu.Lock()
	defer v.mu.Unlock()
	systray.SetTitle("Gwen")
	v.status = systray.AddMenuItem("", "")
	v.status.Disable()
	v.clockIn = systray.AddMenuItem("Clock in", "Start tracking the day")
	v.startBreak = systray.AddMenuItem("Start break", "")
	v.endBreak = systray.AddMenuItem("End break", "")
	v.switchProject = systray.AddMenuItem("Switch project", "")
	for range maxProjects + 1 {
		v.projectItems = append(v.projectItems, v.switchProject.AddSubMenuItemCheckbox("", "", false))
		v.projectIDs = append(v.projectIDs, nil)
	}
	v.snooze = systray.AddMenuItem("Snooze nudges", "")
	v.clockOut = systray.AddMenuItem("Clock out", "End the work day")
	systray.AddSeparator()
	v.open = systray.AddMenuItem("Open dashboard", "")
	v.startGwen = systray.AddMenuItem("Start Gwen", "Start the Gwen daemon")
	v.quit = systray.AddMenuItem("Quit tray", "Close the tray; tracking goes on")

	on := func(item *systray.MenuItem, fn func()) {
		go func() {
			for range item.ClickedCh {
				fn()
			}
		}()
	}
	on(v.clockIn, c.clockIn)
	on(v.startBreak, c.startBreak)
	on(v.endBreak, c.endBreak)
	on(v.snooze, c.snooze)
	on(v.clockOut, c.clockOut)
	on(v.open, c.openDashboard)
	on(v.startGwen, c.startGwen)
	on(v.quit, func() { quit(); systray.Quit() })
	for i, item := range v.projectItems {
		on(item, func() {
			v.mu.Lock()
			id := v.projectIDs[i]
			v.mu.Unlock()
			c.switchTo(id)
		})
	}
	v.built = true
	if v.pending != nil {
		v.apply(*v.pending)
	}
}

func (v *systrayView) show(m Menu) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.built {
		v.pending = &m
		return
	}
	v.apply(m)
}

// apply updates every item; v.mu must be held.
func (v *systrayView) apply(m Menu) {
	if m.Icon != v.icon {
		if b, err := tray.Icon(m.Icon, 44); err == nil {
			systray.SetIcon(b)
			v.icon = m.Icon
		} else {
			slog.Warn("tray icon", "err", err)
		}
	}
	systray.SetTooltip(m.Tooltip)
	v.status.SetTitle(m.Tooltip)
	for item, visible := range map[*systray.MenuItem]bool{
		v.clockIn: m.ClockIn, v.startBreak: m.StartBreak, v.endBreak: m.EndBreak,
		v.switchProject: m.SwitchProject, v.snooze: m.Snooze, v.clockOut: m.ClockOut, v.startGwen: m.StartGwen,
	} {
		if visible {
			item.Show()
		} else {
			item.Hide()
		}
	}
	for i, item := range v.projectItems {
		if i >= len(m.Projects) {
			item.Hide()
			continue
		}
		p := m.Projects[i]
		v.projectIDs[i] = p.ID
		item.SetTitle(p.Name)
		if p.Checked {
			item.Check()
		} else {
			item.Uncheck()
		}
		item.Show()
	}
}
