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
	mu                                           sync.Mutex
	lines                                        []*item
	clockIn, startBreak, endBreak, switchProject *item
	snooze, clockOut, startGwen                  *item
	projectItems                                 []*item
	projectIDs                                   []*string
	energy                                       *item
	icon, tooltip                                string
	built                                        bool
	pending                                      *Menu
}

func (v *systrayView) build(c *controller, quit context.CancelFunc) {
	v.mu.Lock()
	defer v.mu.Unlock()
	systray.SetTitle("Gwen")
	for range maxLines {
		line := addItem("", "")
		line.mi.Disable()
		v.lines = append(v.lines, line)
	}
	v.clockIn = addItem("Clock in", "Start tracking the day")
	v.startBreak = addItem("Start break", "")
	v.endBreak = addItem("End break", "")
	v.switchProject = addItem("Switch project", "")
	for range maxProjects + 1 {
		v.projectItems = append(v.projectItems, &item{mi: v.switchProject.mi.AddSubMenuItemCheckbox("", "", false)})
		v.projectIDs = append(v.projectIDs, nil)
	}
	v.snooze = addItem("Snooze nudges", "")
	v.clockOut = addItem("Clock out", "End the work day")
	systray.AddSeparator()
	v.energy = addItem("Log energy", "How is your energy right now? Gwen finds your prime time from these")
	var levels []*systray.MenuItem
	for i := len(EnergyLevels) - 1; i >= 0; i-- {
		levels = append(levels, v.energy.mi.AddSubMenuItem(EnergyLevels[i], ""))
	}
	ask := systray.AddMenuItem("Ask the assistant…", "Talk to your AI assistant")
	capture := systray.AddMenuItem("Capture to inbox…", "Jot down a task to sort out later")
	board := systray.AddMenuItem("Task board", "Your tasks by stage")
	open := systray.AddMenuItem("Open dashboard", "")
	v.startGwen = addItem("Start Gwen", "Start the Gwen daemon")
	quitItem := systray.AddMenuItem("Quit tray", "Close the tray; tracking goes on")

	on := func(item *systray.MenuItem, fn func()) {
		go func() {
			for range item.ClickedCh {
				fn()
			}
		}()
	}
	on(v.clockIn.mi, c.clockIn)
	on(v.startBreak.mi, c.startBreak)
	on(v.endBreak.mi, c.endBreak)
	on(v.snooze.mi, c.snooze)
	on(v.clockOut.mi, c.clockOut)
	on(open, c.openDashboard)
	on(ask, func() { c.openScreen("assistant") })
	on(capture, func() { c.openScreen("inbox") })
	on(board, func() { c.openScreen("board") })
	for i, mi := range levels {
		on(mi, func() { c.logEnergy(len(EnergyLevels) - i) })
	}
	on(v.startGwen.mi, c.startGwen)
	on(quitItem, func() { quit(); systray.Quit() })
	for i, item := range v.projectItems {
		on(item.mi, func() {
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

// apply updates what changed; v.mu must be held.
func (v *systrayView) apply(m Menu) {
	if m.Icon != v.icon {
		if b, err := tray.Icon(m.Icon, 44); err == nil {
			systray.SetIcon(b)
			v.icon = m.Icon
		} else {
			slog.Warn("tray icon", "err", err)
		}
	}
	if m.Tooltip != v.tooltip {
		systray.SetTooltip(m.Tooltip)
		v.tooltip = m.Tooltip
	}
	for i, line := range v.lines {
		if i < len(m.Lines) {
			line.setTitle(m.Lines[i])
		}
		line.setShown(i < len(m.Lines))
	}
	for item, shown := range map[*item]bool{
		v.clockIn: m.ClockIn, v.startBreak: m.StartBreak, v.endBreak: m.EndBreak,
		v.switchProject: m.SwitchProject, v.snooze: m.Snooze, v.clockOut: m.ClockOut, v.startGwen: m.StartGwen,
		v.energy: m.Energy,
	} {
		item.setShown(shown)
	}
	if m.EnergyTitle != "" {
		v.energy.setTitle(m.EnergyTitle)
	}
	for i, item := range v.projectItems {
		if i >= len(m.Projects) {
			item.setShown(false)
			continue
		}
		p := m.Projects[i]
		v.projectIDs[i] = p.ID
		item.setTitle(p.Name)
		item.setChecked(p.Checked)
		item.setShown(true)
	}
}

// item is a menu item and what it shows, so a render sends only what changed:
// every systray call is a D-Bus signal, and the timer re-renders every second.
type item struct {
	mi              *systray.MenuItem
	title           string
	hidden, checked bool
}

// addItem adds a top-level item, which systray shows unchecked.
func addItem(title, tooltip string) *item {
	return &item{mi: systray.AddMenuItem(title, tooltip), title: title}
}

func (i *item) setTitle(title string) {
	if title != i.title {
		i.mi.SetTitle(title)
		i.title = title
	}
}

func (i *item) setShown(shown bool) {
	if shown == !i.hidden {
		return
	}
	if shown {
		i.mi.Show()
	} else {
		i.mi.Hide()
	}
	i.hidden = !shown
}

func (i *item) setChecked(checked bool) {
	if checked == i.checked {
		return
	}
	if checked {
		i.mi.Check()
	} else {
		i.mi.Uncheck()
	}
	i.checked = checked
}
