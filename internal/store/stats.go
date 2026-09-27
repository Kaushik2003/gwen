package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// maxRange is the widest from/to range a query accepts, in days.
const maxRange = 366

// ProjectTotal is worked time on one project. ProjectID is nil for unassigned
// work, whose Name and Color are then empty; a deleted project keeps its name.
type ProjectTotal struct {
	ProjectID *string
	Name      string
	Color     string
	Worked    time.Duration
}

// DaySummary is the totals of one work day (docs/03-data-model.md#derived-totals).
type DaySummary struct {
	Day       string
	Target    time.Duration
	Worked    time.Duration
	Break     time.Duration
	TargetMet bool
	ByProject []ProjectTotal // by worked time, descending
}

// Summary is the totals over a range of days plus all-time streaks.
type Summary struct {
	From, To      string
	Worked, Break time.Duration
	DaysTracked   int
	DaysTargetMet int
	AvgWorked     time.Duration // over tracked days; 0 when none
	ByProject     []ProjectTotal
	CurrentStreak int
	LongestStreak int
}

// HeatmapDay is worked time on a day with some work.
type HeatmapDay struct {
	Day    string
	Worked time.Duration
}

// StatsRepo computes totals. The open segment counts up to now.
type StatsRepo interface {
	DaySummary(ctx context.Context, workDayID string, now time.Time) (DaySummary, error)
	// Days returns a summary for each day in [from, to] that has a work day, ascending.
	Days(ctx context.Context, from, to string, now time.Time) ([]DaySummary, error)
	// Summary totals [from, to]; streaks are all-time, counted back from
	// today, the day the current instant belongs to.
	Summary(ctx context.Context, from, to, today string, now time.Time) (Summary, error)
	// Heatmap returns the year's days with work, ascending.
	Heatmap(ctx context.Context, year int, now time.Time) ([]HeatmapDay, error)
}

type statsRepo struct{ db *DB }

// NewStatsRepo returns the SQLite StatsRepo.
func NewStatsRepo(db *DB) StatsRepo { return statsRepo{db} }

// dayTotals is one row of the per-day query in 03-data-model.md#derived-totals.
type dayTotals struct {
	id, day     string
	target      time.Duration
	worked, brk time.Duration
}

func (d dayTotals) met() bool { return d.worked >= d.target }

func (r statsRepo) dayTotals(ctx context.Context, from, to string, now time.Time) ([]dayTotals, error) {
	const qDayTotals = `SELECT d.id, d.day, d.target_seconds,
		COALESCE(SUM(CASE WHEN s.kind = 'work' THEN COALESCE(s.ended_at, ?) - s.started_at END), 0),
		COALESCE(SUM(CASE WHEN s.kind <> 'work' THEN COALESCE(s.ended_at, ?) - s.started_at END), 0)
		FROM work_days d
		LEFT JOIN segments s ON s.work_day_id = d.id AND s.deleted_at IS NULL
		WHERE d.day BETWEEN ? AND ? AND d.deleted_at IS NULL
		GROUP BY d.id
		ORDER BY d.day`
	n := now.UnixMilli()
	rows, err := r.db.sql.QueryContext(ctx, qDayTotals, n, n, from, to)
	if err != nil {
		return nil, fmt.Errorf("day totals: %w", err)
	}
	defer rows.Close()
	var out []dayTotals
	for rows.Next() {
		var d dayTotals
		var target, worked, brk int64
		if err := rows.Scan(&d.id, &d.day, &target, &worked, &brk); err != nil {
			return nil, fmt.Errorf("day totals: %w", err)
		}
		d.target = time.Duration(target) * time.Second
		d.worked, d.brk = time.Duration(worked)*time.Millisecond, time.Duration(brk)*time.Millisecond
		out = append(out, d)
	}
	return out, rows.Err()
}

// projectTotals sums work by project over the work days selected by where,
// keyed by work day id ("" when grouping the whole selection).
func (r statsRepo) projectTotals(ctx context.Context, perDay bool, where string, args ...any) (map[string][]ProjectTotal, error) {
	group := "''"
	if perDay {
		group = "s.work_day_id"
	}
	q := `SELECT ` + group + `, s.project_id, p.name, p.color, SUM(COALESCE(s.ended_at, ?) - s.started_at)
		FROM segments s
		JOIN work_days d ON d.id = s.work_day_id AND d.deleted_at IS NULL
		LEFT JOIN projects p ON p.id = s.project_id
		WHERE s.kind = 'work' AND s.deleted_at IS NULL AND ` + where + `
		GROUP BY 1, s.project_id`
	rows, err := r.db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("project totals: %w", err)
	}
	defer rows.Close()
	out := map[string][]ProjectTotal{}
	for rows.Next() {
		var (
			key         string
			id          sql.NullString
			name, color sql.NullString
			ms          int64
		)
		if err := rows.Scan(&key, &id, &name, &color, &ms); err != nil {
			return nil, fmt.Errorf("project totals: %w", err)
		}
		out[key] = append(out[key], ProjectTotal{ProjectID: stringPtr(id), Name: name.String, Color: color.String,
			Worked: time.Duration(ms) * time.Millisecond})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, totals := range out {
		sortTotals(totals)
	}
	return out, nil
}

func sortTotals(t []ProjectTotal) {
	sort.SliceStable(t, func(i, j int) bool {
		if t[i].Worked != t[j].Worked {
			return t[i].Worked > t[j].Worked
		}
		return t[i].Name < t[j].Name
	})
}

func (r statsRepo) DaySummary(ctx context.Context, workDayID string, now time.Time) (DaySummary, error) {
	w, err := getWorkDayTx(ctx, r.db.sql, workDayID)
	if err != nil {
		return DaySummary{}, fmt.Errorf("day summary: %w", err)
	}
	days, err := r.Days(ctx, w.Day, w.Day, now)
	if err != nil {
		return DaySummary{}, err
	}
	return days[0], nil
}

func (r statsRepo) Days(ctx context.Context, from, to string, now time.Time) ([]DaySummary, error) {
	if err := checkRange(from, to); err != nil {
		return nil, err
	}
	totals, err := r.dayTotals(ctx, from, to, now)
	if err != nil {
		return nil, err
	}
	byDay, err := r.projectTotals(ctx, true, `d.day BETWEEN ? AND ?`, now.UnixMilli(), from, to)
	if err != nil {
		return nil, err
	}
	out := make([]DaySummary, 0, len(totals))
	for _, d := range totals {
		projects := byDay[d.id]
		if projects == nil {
			projects = []ProjectTotal{}
		}
		out = append(out, DaySummary{Day: d.day, Target: d.target, Worked: d.worked, Break: d.brk,
			TargetMet: d.met(), ByProject: projects})
	}
	return out, nil
}

func (r statsRepo) Summary(ctx context.Context, from, to, today string, now time.Time) (Summary, error) {
	if err := checkRange(from, to); err != nil {
		return Summary{}, err
	}
	s := Summary{From: from, To: to, ByProject: []ProjectTotal{}}
	totals, err := r.dayTotals(ctx, from, to, now)
	if err != nil {
		return Summary{}, err
	}
	for _, d := range totals {
		s.Worked += d.worked
		s.Break += d.brk
		s.DaysTracked++
		if d.met() {
			s.DaysTargetMet++
		}
	}
	if s.DaysTracked > 0 {
		s.AvgWorked = s.Worked / time.Duration(s.DaysTracked)
	}
	all, err := r.projectTotals(ctx, false, `d.day BETWEEN ? AND ?`, now.UnixMilli(), from, to)
	if err != nil {
		return Summary{}, err
	}
	if p := all[""]; p != nil {
		s.ByProject = p
	}
	history, err := r.dayTotals(ctx, "0000-01-01", "9999-12-31", now)
	if err != nil {
		return Summary{}, err
	}
	s.CurrentStreak, s.LongestStreak = streaks(history, today)
	return s, nil
}

// streaks counts consecutive calendar days on which the target was met. The
// current run starts at today if today's target is met, otherwise at
// yesterday; a day without a work day breaks a run.
func streaks(history []dayTotals, today string) (current, longest int) {
	met := map[string]bool{}
	for _, d := range history {
		if d.met() {
			met[d.day] = true
		}
	}
	day := today
	if !met[day] {
		day = addDays(day, -1)
	}
	for met[day] {
		current++
		day = addDays(day, -1)
	}
	run, prev := 0, ""
	for _, d := range history {
		switch {
		case !d.met():
			run = 0
		case prev != "" && addDays(prev, 1) == d.day && run > 0:
			run++
		default:
			run = 1
		}
		prev = d.day
		longest = max(longest, run)
	}
	return current, longest
}

func addDays(day string, n int) string {
	t, err := time.Parse(model.DayLayout, day)
	if err != nil {
		return day
	}
	return time.Date(t.Year(), t.Month(), t.Day()+n, 0, 0, 0, 0, time.UTC).Format(model.DayLayout)
}

func (r statsRepo) Heatmap(ctx context.Context, year int, now time.Time) ([]HeatmapDay, error) {
	if year < 1 || year > 9999 {
		return nil, FailField(ErrInvalid, "year", "year must be between 1 and 9999")
	}
	from, to := fmt.Sprintf("%04d-01-01", year), fmt.Sprintf("%04d-12-31", year)
	totals, err := r.dayTotals(ctx, from, to, now)
	if err != nil {
		return nil, err
	}
	out := []HeatmapDay{}
	for _, d := range totals {
		if d.worked > 0 {
			out = append(out, HeatmapDay{Day: d.day, Worked: d.worked})
		}
	}
	return out, nil
}

// checkRange validates an inclusive from/to range of at most 366 days.
func checkRange(from, to string) error {
	if !validDay(from) {
		return FailField(ErrInvalid, "from", "from must be a date like 2026-09-15")
	}
	if !validDay(to) {
		return FailField(ErrInvalid, "to", "to must be a date like 2026-09-15")
	}
	if to < from {
		return FailField(ErrInvalid, "to", "to must not be before from")
	}
	if addDays(from, maxRange) < to {
		return FailField(ErrInvalid, "to", "a range may span at most %d days", maxRange)
	}
	return nil
}
