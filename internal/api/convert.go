package api

import (
	"context"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
)

// BuildStatus assembles the engine snapshot with the open rows and today's
// totals at now (docs/04-api-contract.md#status).
func BuildStatus(ctx context.Context, repos store.Repos, s timeengine.Snapshot, now time.Time,
	backend string, warnings []string) (wire.Status, error) {
	st := wire.Status{
		State:           string(s.State),
		StateSinceAt:    wire.Millis(s.Since),
		ProjectID:       s.ProjectID,
		TaskID:          s.TaskID,
		IdleSinceAt:     wire.MillisPtr(s.IdleSince),
		SnoozedUntilAt:  wire.MillisPtr(s.SnoozedUntil),
		ServerNowAt:     wire.Millis(now),
		ActivityBackend: backend,
		Warnings:        append([]string{}, warnings...),
	}
	wd, err := repos.WorkDays.Current(ctx)
	if err != nil {
		return wire.Status{}, fmt.Errorf("status: %w", err)
	}
	if wd != nil {
		w := workDayWire(*wd)
		st.WorkDay = &w
		sum, err := repos.Stats.DaySummary(ctx, wd.ID, now)
		if err != nil {
			return wire.Status{}, fmt.Errorf("status: %w", err)
		}
		today := daySummaryWire(sum)
		st.Today = &today
	}
	seg, err := repos.Segments.Current(ctx)
	if err != nil {
		return wire.Status{}, fmt.Errorf("status: %w", err)
	}
	if seg != nil {
		g := segmentWire(*seg)
		st.OpenSegment = &g
	}
	return st, nil
}

func workDayWire(w model.WorkDay) wire.WorkDay {
	return wire.WorkDay{
		ID: w.ID, Day: w.Day, TZ: w.TZ,
		ClockedInAt: wire.Millis(w.ClockedInAt), ClockedOutAt: wire.MillisPtr(w.ClockedOutAt),
		TargetSeconds: int(w.Target / time.Second), Note: w.Note,
		CreatedAt: wire.Millis(w.CreatedAt), UpdatedAt: wire.Millis(w.UpdatedAt), Rev: w.Rev,
	}
}

func segmentWire(g model.Segment) wire.Segment {
	return wire.Segment{
		ID: g.ID, WorkDayID: g.WorkDayID, Kind: g.Kind, Source: g.Source,
		ProjectID: g.ProjectID, TaskID: g.TaskID,
		StartedAt: wire.Millis(g.StartedAt), EndedAt: wire.MillisPtr(g.EndedAt), Truncated: g.Truncated,
		CreatedAt: wire.Millis(g.CreatedAt), UpdatedAt: wire.Millis(g.UpdatedAt), Rev: g.Rev,
	}
}

func segmentsWire(gs []model.Segment) []wire.Segment {
	out := make([]wire.Segment, len(gs))
	for i, g := range gs {
		out[i] = segmentWire(g)
	}
	return out
}

func projectTotalsWire(ts []store.ProjectTotal) []wire.ProjectTotal {
	out := make([]wire.ProjectTotal, len(ts))
	for i, t := range ts {
		out[i] = wire.ProjectTotal{ProjectID: t.ProjectID, Name: t.Name, Color: t.Color, WorkedMs: t.Worked.Milliseconds()}
		if t.ProjectID == nil {
			out[i].Name, out[i].Color = wire.UnassignedName, wire.UnassignedColor
		}
	}
	return out
}

func daySummaryWire(d store.DaySummary) wire.DaySummary {
	return wire.DaySummary{
		Day: d.Day, TargetSeconds: int(d.Target / time.Second),
		WorkedMs: d.Worked.Milliseconds(), BreakMs: d.Break.Milliseconds(), TargetMet: d.TargetMet,
		ByProject: projectTotalsWire(d.ByProject),
	}
}

func projectWire(p model.Project) wire.Project {
	return wire.Project{
		ID: p.ID, Name: p.Name, Color: p.Color, ArchivedAt: wire.MillisPtr(p.ArchivedAt),
		CreatedAt: wire.Millis(p.CreatedAt), UpdatedAt: wire.Millis(p.UpdatedAt), Rev: p.Rev,
	}
}

func taskWire(t model.Task, tracked time.Duration) wire.Task {
	var estimate *int
	if t.Estimate != nil {
		m := int(*t.Estimate / time.Minute)
		estimate = &m
	}
	return wire.Task{
		ID: t.ID, ProjectID: t.ProjectID, Title: t.Title, Notes: t.Notes, Status: t.Status,
		Priority: t.Priority, DueDay: t.DueDay, EstimateMinutes: estimate, DoneAt: wire.MillisPtr(t.DoneAt),
		CreatedAt: wire.Millis(t.CreatedAt), UpdatedAt: wire.Millis(t.UpdatedAt), Rev: t.Rev,
		TrackedMs: tracked.Milliseconds(),
	}
}
