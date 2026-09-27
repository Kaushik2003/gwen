package main

import (
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/packaging/icons/tray"
)

// maxProjects is how many projects the Switch project submenu lists after
// Unassigned; systray cannot remove items, so the pool is fixed.
const maxProjects = 10

// Menu is everything the tray shows for one state (docs/08-clients.md#tray).
type Menu struct {
	Icon    string // a tray.* icon name
	Tooltip string // also the disabled status line

	ClockIn, StartBreak, EndBreak, SwitchProject, Snooze, ClockOut, StartGwen bool

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
// ticking timer counts to; projects is the live, non-archived list by name.
func menuFor(status *wire.Status, daemonUp bool, projects []wire.Project, now time.Time) Menu {
	if !daemonUp || status == nil {
		return Menu{Icon: tray.Off, Tooltip: "Gwen isn't running", StartGwen: true}
	}
	m := Menu{Icon: tray.Off, Tooltip: "Not clocked in"}
	elapsed := "0s"
	if status.OpenSegment != nil {
		elapsed = client.FormatDuration(now.Sub(wire.Time(status.OpenSegment.StartedAt)))
	}
	name := wire.UnassignedName
	for _, p := range projects {
		if status.ProjectID != nil && p.ID == *status.ProjectID {
			name = p.Name
		}
	}
	switch status.State {
	case wire.StateOff:
		m.ClockIn = true
		return m
	case wire.StateWorking:
		m.Icon, m.Tooltip = tray.Working, "Working · "+elapsed+" · "+name
		m.StartBreak = true
	case wire.StateIdlePending:
		m.Icon, m.Tooltip = tray.Idle, "Idle · "+elapsed+" · "+name
		m.StartBreak = true
	case wire.StateBreakAuto:
		m.Icon, m.Tooltip = tray.Break, "On break · "+elapsed
		m.StartBreak, m.EndBreak = true, true
	case wire.StateBreakManual:
		m.Icon, m.Tooltip = tray.Break, "On break · "+elapsed
		m.EndBreak = true
	}
	m.SwitchProject, m.Snooze, m.ClockOut = true, true, true
	m.Projects = append(m.Projects, MenuProject{Name: wire.UnassignedName, Checked: status.ProjectID == nil})
	for _, p := range projects {
		if len(m.Projects) > maxProjects {
			break
		}
		id := p.ID
		m.Projects = append(m.Projects, MenuProject{ID: &id, Name: p.Name,
			Checked: status.ProjectID != nil && *status.ProjectID == p.ID})
	}
	return m
}
