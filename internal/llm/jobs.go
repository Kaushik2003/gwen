package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/planner/civil"
)

// Retro generators (output.generated_by).
const (
	GeneratedByLLM   = "llm"
	GeneratedByRules = "rules"
)

// MaxRetro is the longest retro, in characters.
const MaxRetro = 4000

// BreakdownRequest is everything a breakdown sends, and nothing else.
type BreakdownRequest struct {
	Title          string   `json:"title"`
	Kind           string   `json:"kind"`
	Unit           string   `json:"unit"`
	TargetQuantity *int     `json:"target_quantity"`
	MinutesPerUnit *int     `json:"minutes_per_unit"`
	StartDay       string   `json:"start_day"`
	DueDay         string   `json:"due_day"`
	ExistingTasks  []string `json:"existing_tasks"`
	Instructions   string   `json:"instructions"`
	Today          string   `json:"today"`
}

// ProposedTask is one task of a breakdown.
type ProposedTask struct {
	Title           string `json:"title"`
	Notes           string `json:"notes"`
	EstimateMinutes int    `json:"estimate_minutes"`
	DueDay          string `json:"due_day"`
	Priority        int    `json:"priority"`
	Quantity        *int   `json:"quantity"`
}

// BreakdownOutput is llm_runs.output of an ok breakdown.
type BreakdownOutput struct {
	Tasks []ProposedTask `json:"tasks"`
}

// RetroDay is one day of a retro.
type RetroDay struct {
	Day       string `json:"day"`
	WorkedMin int    `json:"worked_minutes"`
	BreakMin  int    `json:"break_minutes"`
	TargetMin int    `json:"target_minutes"`
	TargetMet bool   `json:"target_met"`
}

// RetroProject is a project's time over the week.
type RetroProject struct {
	Name      string `json:"name"`
	WorkedMin int    `json:"worked_minutes"`
}

// RetroGoal is an active goal's progress.
type RetroGoal struct {
	Title          string  `json:"title"`
	Done           int     `json:"done"`
	Remaining      int     `json:"remaining"`
	Unit           string  `json:"unit"`
	RequiredPerDay float64 `json:"required_per_day"`
	ActualPerDay   float64 `json:"actual_per_day"`
	Pace           string  `json:"pace"`
	DueDay         string  `json:"due_day"`
}

// RetroRequest is everything a retro sends: the 7 days from WeekStart.
type RetroRequest struct {
	WeekStart      string         `json:"week_start"`
	Days           []RetroDay     `json:"days"`
	Projects       []RetroProject `json:"projects"` // by time, descending
	Completed      []string       `json:"completed_tasks"`
	Goals          []RetroGoal    `json:"goals"`
	PlannedMinutes int            `json:"planned_minutes"`
	DoneMinutes    int            `json:"done_minutes"`
}

// RetroOutput is llm_runs.output of a retro.
type RetroOutput struct {
	Markdown    string `json:"markdown"`
	GeneratedBy string `json:"generated_by"`
}

// Breakdown asks the model for tasks and validates them strictly. Any
// failure is an error; there is no rules fallback for a breakdown.
func (a *adapter) Breakdown(ctx context.Context, req BreakdownRequest) (BreakdownOutput, error) {
	if req.ExistingTasks == nil {
		req.ExistingTasks = []string{}
	}
	text, err := a.ask(ctx, breakdownPrompt, req)
	if err != nil {
		return BreakdownOutput{}, err
	}
	out, err := ParseBreakdown(text)
	if err != nil {
		return BreakdownOutput{}, err
	}
	if err := ValidateBreakdown(out, req.Today, req.DueDay); err != nil {
		return BreakdownOutput{}, err
	}
	return out, nil
}

// ParseBreakdown decodes a breakdown reply, rejecting unknown fields.
func ParseBreakdown(text string) (BreakdownOutput, error) {
	var raw struct {
		Tasks []json.RawMessage `json:"tasks"`
	}
	if err := strictDecode(text, &raw); err != nil {
		return BreakdownOutput{}, fmt.Errorf("the reply is not the breakdown JSON: %w", err)
	}
	out := BreakdownOutput{Tasks: make([]ProposedTask, len(raw.Tasks))}
	for i, r := range raw.Tasks {
		var t struct {
			Title           *string `json:"title"`
			Notes           *string `json:"notes"`
			EstimateMinutes *int    `json:"estimate_minutes"`
			DueDay          *string `json:"due_day"`
			Priority        *int    `json:"priority"`
			Quantity        *int    `json:"quantity"`
		}
		if err := strictDecode(string(r), &t); err != nil {
			return BreakdownOutput{}, fmt.Errorf("task %d is not valid: %w", i, err)
		}
		if t.Title == nil || t.EstimateMinutes == nil || t.DueDay == nil || t.Priority == nil {
			return BreakdownOutput{}, fmt.Errorf("task %d lacks title, estimate_minutes, due_day, or priority", i)
		}
		out.Tasks[i] = ProposedTask{Title: *t.Title, EstimateMinutes: *t.EstimateMinutes, DueDay: *t.DueDay,
			Priority: *t.Priority, Quantity: t.Quantity}
		if t.Notes != nil {
			out.Tasks[i].Notes = *t.Notes
		}
	}
	return out, nil
}

func strictDecode(s string, v any) error {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("trailing data after the JSON object")
	}
	return nil
}

// ValidateBreakdown applies the rules of docs/07-integrations.md#breakdown.
func ValidateBreakdown(out BreakdownOutput, today, goalDue string) error {
	if n := len(out.Tasks); n < 1 || n > 50 {
		return fmt.Errorf("a breakdown needs 1 to 50 tasks, got %d", n)
	}
	for i, t := range out.Tasks {
		if n := len([]rune(strings.TrimSpace(t.Title))); n < 1 || n > 200 {
			return fmt.Errorf("task %d: the title must be 1 to 200 characters", i)
		}
		if t.EstimateMinutes < 5 || t.EstimateMinutes > 480 {
			return fmt.Errorf("task %d: estimate_minutes %d is outside 5 to 480", i, t.EstimateMinutes)
		}
		if _, err := civil.Parse(t.DueDay); err != nil {
			return fmt.Errorf("task %d: due_day %q is not a date", i, t.DueDay)
		}
		if t.DueDay < today || t.DueDay > goalDue {
			return fmt.Errorf("task %d: due_day %s is outside %s to %s", i, t.DueDay, today, goalDue)
		}
		if t.Priority < 1 || t.Priority > 4 {
			return fmt.Errorf("task %d: priority %d is outside 1 to 4", i, t.Priority)
		}
		if t.Quantity != nil && *t.Quantity < 1 {
			return fmt.Errorf("task %d: quantity must be positive or null", i)
		}
	}
	return nil
}

// Retro asks the model for the week's retrospective and falls back to the
// rules summary on any failure, so it always succeeds; the error it returns
// is the reason the model's reply was not used, for logging.
func (a *adapter) Retro(ctx context.Context, req RetroRequest) (RetroOutput, error) {
	text, err := a.ask(ctx, retroPrompt, req)
	if err == nil {
		var out RetroOutput
		switch err = strictDecode(text, &out); {
		case err != nil:
			err = fmt.Errorf("the reply is not the retro JSON: %w", err)
		case strings.TrimSpace(out.Markdown) == "":
			err = errors.New("the retro is empty")
		case len([]rune(out.Markdown)) > MaxRetro:
			err = fmt.Errorf("the retro is longer than %d characters", MaxRetro)
		default:
			return RetroOutput{Markdown: out.Markdown, GeneratedBy: GeneratedByLLM}, nil
		}
	}
	return RulesRetro(req), err
}

// RulesRetro is the deterministic summary used when the model fails: totals,
// the top three projects, target-met days, and goals behind pace.
func RulesRetro(req RetroRequest) RetroOutput {
	var b bytes.Buffer
	worked, brk, met, tracked := 0, 0, 0, 0
	for _, d := range req.Days {
		worked += d.WorkedMin
		brk += d.BreakMin
		if d.WorkedMin > 0 || d.BreakMin > 0 {
			tracked++
		}
		if d.TargetMet {
			met++
		}
	}
	fmt.Fprintf(&b, "## Week of %s\n\n", req.WeekStart)
	fmt.Fprintf(&b, "Worked %s over %d %s, with %s on breaks. The target was met on %d of 7 days.\n",
		hours(worked), tracked, plural(tracked, "day", "days"), hours(brk), met)
	if req.PlannedMinutes > 0 {
		fmt.Fprintf(&b, "Of %s planned, %s was done (%d%%).\n", hours(req.PlannedMinutes), hours(req.DoneMinutes),
			int(math.Round(100*float64(req.DoneMinutes)/float64(req.PlannedMinutes))))
	}
	if len(req.Projects) > 0 {
		b.WriteString("\n### Top projects\n\n")
		for _, p := range req.Projects[:min(3, len(req.Projects))] {
			fmt.Fprintf(&b, "- %s: %s\n", p.Name, hours(p.WorkedMin))
		}
	}
	if len(req.Completed) > 0 {
		fmt.Fprintf(&b, "\nCompleted %d %s.\n", len(req.Completed), plural(len(req.Completed), "task", "tasks"))
	}
	behind := slices.DeleteFunc(slices.Clone(req.Goals), func(g RetroGoal) bool { return g.Pace != "behind" })
	if len(behind) > 0 {
		b.WriteString("\n### Goals behind pace\n\n")
		for _, g := range behind {
			fmt.Fprintf(&b, "- %s: %d done, %d left by %s; needs %.1f a day, doing %.1f.\n",
				g.Title, g.Done, g.Remaining, g.DueDay, g.RequiredPerDay, g.ActualPerDay)
		}
	}
	md := strings.TrimSpace(b.String())
	if r := []rune(md); len(r) > MaxRetro {
		md = string(r[:MaxRetro])
	}
	return RetroOutput{Markdown: md, GeneratedBy: GeneratedByRules}
}

func hours(minutes int) string {
	d := time.Duration(minutes) * time.Minute
	h, m := int(d.Hours()), minutes%60
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
