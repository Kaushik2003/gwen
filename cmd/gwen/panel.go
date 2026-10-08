package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"slices"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// panelLine is what the KDE panel widget shows: the state, the day's worked
// and break time as of now, the project and task by name, and the plan block
// on now or next.
type panelLine struct {
	State    string `json:"state"` // a wire state, or "down" when the daemon is not running
	WorkedMs int64  `json:"worked_ms"`
	BreakMs  int64  `json:"break_ms"`
	TargetMs int64  `json:"target_ms"`
	// SinceMs is how long the state has lasted: the break, on a break.
	SinceMs int64       `json:"since_ms"`
	Project string      `json:"project"`
	Color   string      `json:"color"`
	Task    string      `json:"task"`
	Next    *panelBlock `json:"next"` // nil when nothing more is planned today
	// More counts the planned blocks after Next.
	More int `json:"more"`
	// ProjectID is the tracked project, "" for none; Projects are the live
	// ones by name, to switch to.
	ProjectID string         `json:"project_id"`
	Projects  []panelProject `json:"projects"`
	// Energy is the latest check-in of the last day, or nil.
	Energy *wire.EnergyLog `json:"energy"`
}

type panelProject struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// panelBlock is a planned block of a task.
type panelBlock struct {
	TaskID string `json:"task_id"`
	// ProjectID is the task's project, "none" for unassigned: starting the
	// task takes both, as `gwen switch` does not look the project up.
	ProjectID string `json:"project_id"`
	Title     string `json:"title"`
	StartAt   int64  `json:"start_at"` // Unix ms
	Minutes   int    `json:"minutes"`
	Now       bool   `json:"now"` // the block is on now, rather than next
}

// panelCmd prints one panelLine as JSON for the panel widget, which polls it.
// It is not for people, so it is hidden from help.
func (c *cli) panelCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "panel",
		Short:  "Print the panel widget's line as JSON",
		Args:   args(0),
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			st, err := c.api.Status(ctx)
			if errors.Is(err, client.ErrDaemonNotRunning) {
				return json.NewEncoder(c.stdout).Encode(panelLine{State: "down"})
			}
			if err != nil {
				return err
			}
			line := panelLine{State: st.State, Project: wire.UnassignedName, SinceMs: st.ServerNowAt - st.StateSinceAt, Projects: []panelProject{}}
			if d := st.Today; d != nil {
				line.WorkedMs, line.BreakMs, line.TargetMs = d.WorkedMs, d.BreakMs, int64(d.TargetSeconds)*1000
			}
			if list, err := c.api.ListProjects(ctx, wire.ArchivedFalse); err == nil && list != nil {
				for _, p := range list.Projects {
					line.Projects = append(line.Projects, panelProject{ID: p.ID, Name: p.Name, Color: p.Color})
				}
			}
			if st.ProjectID != nil {
				line.ProjectID = *st.ProjectID
				if p, err := c.api.GetProject(ctx, *st.ProjectID); err == nil {
					line.Project, line.Color = p.Name, p.Color
				}
			}
			if st.TaskID != nil {
				if t, err := c.api.GetTask(ctx, *st.TaskID); err == nil {
					line.Task = t.Title
				}
			}
			if day, err := c.today(ctx); err == nil {
				if plan, err := c.api.GetPlan(ctx, day); err == nil && plan != nil {
					line.Next, line.More = nextBlock(plan.Items, st.ServerNowAt)
				}
			}
			if report, err := c.api.EnergyReport(ctx, 1); err == nil && report != nil {
				for _, e := range report.Logs {
					if line.Energy == nil || e.At > line.Energy.At {
						line.Energy = &e
					}
				}
			}
			return json.NewEncoder(c.stdout).Encode(line)
		},
	}
}

// nextBlock is the planned block on at now, else the next to start, and how
// many planned blocks are still to come after it.
func nextBlock(items []wire.PlanItem, now int64) (*panelBlock, int) {
	var ahead []wire.PlanItem
	for _, it := range items {
		if it.Status == "planned" && it.StartAt != nil && *it.StartAt+int64(it.PlannedMinutes)*60_000 > now {
			ahead = append(ahead, it)
		}
	}
	if len(ahead) == 0 {
		return nil, 0
	}
	slices.SortFunc(ahead, func(a, b wire.PlanItem) int { return cmp.Compare(*a.StartAt, *b.StartAt) })
	it := ahead[0]
	project := "none"
	if it.Task.ProjectID != nil {
		project = *it.Task.ProjectID
	}
	return &panelBlock{TaskID: it.Task.ID, ProjectID: project, Title: it.Task.Title, StartAt: *it.StartAt, Minutes: it.PlannedMinutes, Now: *it.StartAt <= now}, len(ahead) - 1
}
