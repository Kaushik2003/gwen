package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/packaging/icons/tray"
)

// maxProjects is how many projects the Switch project submenu lists after
// Unassigned; systray cannot remove items, so the pool is fixed.
const maxProjects = 10

// maxLines is the status block atop the menu: the headline and two details.
const maxLines = 3

// Menu is everything the tray shows for one state (docs/08-clients.md#tray).
type Menu struct {
	Icon    string   // a tray.* icon name
	Tooltip string   // Lines, one per line
	Lines   []string // the disabled status block atop the menu, headline first

	ClockIn, StartBreak, EndBreak, SwitchProject, Snooze, ClockOut, StartGwen bool

	// Energy shows the energy check-in submenu, titled EnergyTitle; the
	// daemon must be running.
	Energy      bool
	EnergyTitle string

	// Projects is the submenu: Unassigned, then up to ten live projects by name.
	Projects []MenuProject
}

// MenuProject is one entry of the Switch project submenu.
type MenuProject struct {
	ID      *string // nil for Unassigned
	Name    string
	Checked bool
}

// menuFor decides the tray for a status. now is the skew-corrected instant the
// ticking timers count to; projects is the live, non-archived list by name.
func menuFor(status *wire.Status, daemonUp bool, projects []wire.Project, now time.Time) Menu {
	if !daemonUp || status == nil {
		return withLines(Menu{Icon: tray.Off, StartGwen: true}, "Gwen isn't running")
	}
	if status.State == wire.StateOff {
		return withLines(Menu{Icon: tray.Off, ClockIn: true}, "Not clocked in")
	}
	working := status.State == wire.StateWorking || status.State == wire.StateIdlePending
	// The day's worked time goes on from its total at server_now_at, so it never
	// restarts after a break or a switch; a break counts from its own start.
	var worked, target time.Duration
	if d := status.Today; d != nil {
		worked = time.Duration(d.WorkedMs) * time.Millisecond
		target = time.Duration(d.TargetSeconds) * time.Second
		if working {
			worked += now.Sub(wire.Time(status.ServerNowAt))
		}
	}
	segment := time.Duration(0)
	if status.OpenSegment != nil {
		segment = now.Sub(wire.Time(status.OpenSegment.StartedAt))
	}
	name := wire.UnassignedName
	for _, p := range projects {
		if status.ProjectID != nil && p.ID == *status.ProjectID {
			name = p.Name
		}
	}
	m := Menu{SwitchProject: true, Snooze: true, ClockOut: true}
	var lines []string
	switch status.State {
	case wire.StateWorking:
		m.Icon, m.StartBreak = tray.Working, true
		lines = []string{"Working · " + client.FormatDuration(worked) + " today", name}
	case wire.StateIdlePending:
		m.Icon, m.StartBreak = tray.Idle, true
		lines = []string{"Idle · " + client.FormatDuration(worked) + " today", name}
	case wire.StateBreakAuto:
		m.Icon, m.StartBreak, m.EndBreak = tray.Break, true, true
		lines = []string{"On break · " + client.FormatDuration(segment), "Worked " + client.FormatDuration(worked) + " today"}
	case wire.StateBreakManual:
		m.Icon, m.EndBreak = tray.Break, true
		lines = []string{"On break · " + client.FormatDuration(segment), "Worked " + client.FormatDuration(worked) + " today"}
	}
	if target > 0 {
		lines = append(lines, targetLine(worked, target))
	}
	m.Projects = append(m.Projects, MenuProject{Name: wire.UnassignedName, Checked: status.ProjectID == nil})
	for _, p := range projects {
		if len(m.Projects) > maxProjects {
			break
		}
		id := p.ID
		m.Projects = append(m.Projects, MenuProject{ID: &id, Name: p.Name,
			Checked: status.ProjectID != nil && *status.ProjectID == p.ID})
	}
	return withLines(m, lines...)
}

// EnergyLevels are the check-in submenu, level 1 first.
var EnergyLevels = []string{"Drained", "Low", "Okay", "Good", "Peak"}

// energyTitle is "Log energy", or "Energy: Good at 10:42 am" after a check-in.
func energyTitle(last *wire.EnergyLog) string {
	if last == nil || last.Level < 1 || last.Level > len(EnergyLevels) {
		return "Log energy"
	}
	return fmt.Sprintf("Energy: %s at %s", EnergyLevels[last.Level-1], client.FormatTime(wire.Time(last.At)))
}

// targetLine is "72% of 8h · 2h 13m left", or "Target of 8h met".
func targetLine(worked, target time.Duration) string {
	if worked >= target {
		return "Target of " + client.FormatDuration(target) + " met"
	}
	return fmt.Sprintf("%d%% of %s · %s left", worked*100/target, client.FormatDuration(target), client.FormatDuration(target-worked))
}

func withLines(m Menu, lines ...string) Menu {
	m.Lines, m.Tooltip = lines, strings.Join(lines, "\n")
	return m
}
