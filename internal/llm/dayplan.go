package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Day plan limits (docs/07-integrations.md#day-plan).
const (
	MaxMessage      = 2000 // characters of one message, the user's or the reply
	MaxUserMessages = 20   // in one conversation
	MaxPlanTasks    = 60   // sent, most urgent first
	MaxPlanNotes    = 500  // characters of a task's notes, as sent
	MaxBlocks       = 30   // in one proposal
)

// Message roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// DayPlanRequest is everything a day plan sends, and nothing else. Times are
// HH:MM on Day.
type DayPlanRequest struct {
	Day      string        `json:"day"`
	Weekday  string        `json:"weekday"`
	Now      *string       `json:"now"`
	Methods  Methods       `json:"methods"`
	Hours    Hours         `json:"hours"`
	Busy     []BusyTime    `json:"busy"`
	Floating []Floating    `json:"floating"`
	Free     []Span        `json:"free"`
	Done     []DoneBlock   `json:"done"`
	Tasks    []PlanTask    `json:"tasks"`
	Plan     []Block       `json:"plan"`
	Messages []ChatMessage `json:"messages"`
}

// Methods are the planning methods the user turned on.
type Methods struct {
	EatTheFrog bool  `json:"eat_the_frog"`
	PrimeTime  *Span `json:"prime_time"` // biological prime time, or null
}

// Hours is the day's window and the minutes it has for tasks.
type Hours struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	WorkMinutes int    `json:"work_minutes"`
}

// BusyTime is a fixed commitment or calendar busy time.
type BusyTime struct {
	Title string `json:"title"`
	Start string `json:"start"`
	End   string `json:"end"`
}

// Floating is a commitment without a time.
type Floating struct {
	Title   string `json:"title"`
	Minutes int    `json:"minutes"`
}

// Span is free time.
type Span struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// DoneBlock is an item of the day already done.
type DoneBlock struct {
	Title   string  `json:"title"`
	Start   *string `json:"start"`
	Minutes int     `json:"minutes"`
}

// PlanTask is a task the plan may use, named by Ref.
type PlanTask struct {
	Ref              string   `json:"ref"`
	Title            string   `json:"title"`
	Notes            string   `json:"notes"`
	Project          *string  `json:"project"`
	Goal             *string  `json:"goal"`
	Pace             *string  `json:"pace"`
	Priority         int      `json:"priority"`
	DueDay           *string  `json:"due_day"`
	EstimateMinutes  *int     `json:"estimate_minutes"`
	RemainingMinutes int      `json:"remaining_minutes"`
	Urgency          float64  `json:"urgency"`
	Effort           *int     `json:"effort"`
	OpenSteps        []string `json:"open_steps"`
	Skipped          bool     `json:"skipped"`
}

// Block is a block of the current proposal; Start is nil when unslotted.
type Block struct {
	Ref     string  `json:"ref"`
	Start   *string `json:"start"`
	Minutes int     `json:"minutes"`
}

// ChatMessage is one turn of the conversation.
type ChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// DayPlanOutput is the model's reply: the whole plan for the day.
type DayPlanOutput struct {
	Reply string         `json:"reply"`
	Items []PlannedBlock `json:"items"`
	Hours *HoursChange   `json:"hours"`
}

// PlannedBlock is a block of a proposal.
type PlannedBlock struct {
	Ref     string `json:"ref"`
	Start   string `json:"start"`
	Minutes int    `json:"minutes"`
}

// HoursChange is the day hours a proposal sets; nil fields are unchanged.
type HoursChange struct {
	Start       *string `json:"start"`
	WorkMinutes *int    `json:"work_minutes"`
}

// DayPlan asks the model for the day's plan and validates it strictly. Any
// failure is an error; there is no rules fallback.
func (a *adapter) DayPlan(ctx context.Context, req DayPlanRequest) (DayPlanOutput, error) {
	text, err := a.ask(ctx, dayPlanJob, req)
	if err != nil {
		return DayPlanOutput{}, err
	}
	out, err := ParseDayPlan(text)
	if err != nil {
		return DayPlanOutput{}, err
	}
	if err := ValidateDayPlan(out, req); err != nil {
		return DayPlanOutput{}, err
	}
	return out, nil
}

// ParseDayPlan decodes a day plan reply, rejecting unknown fields.
func ParseDayPlan(text string) (DayPlanOutput, error) {
	var raw struct {
		Reply *string           `json:"reply"`
		Items []json.RawMessage `json:"items"`
		Hours *HoursChange      `json:"hours"`
	}
	if err := strictDecode(text, &raw); err != nil {
		return DayPlanOutput{}, fmt.Errorf("the reply is not the day plan JSON: %w", err)
	}
	if raw.Reply == nil {
		return DayPlanOutput{}, fmt.Errorf("the reply has no reply text")
	}
	out := DayPlanOutput{Reply: *raw.Reply, Items: make([]PlannedBlock, len(raw.Items)), Hours: raw.Hours}
	for i, r := range raw.Items {
		var b struct {
			Ref     *string `json:"ref"`
			Start   *string `json:"start"`
			Minutes *int    `json:"minutes"`
		}
		if err := strictDecode(string(r), &b); err != nil {
			return DayPlanOutput{}, fmt.Errorf("block %d is not valid: %w", i, err)
		}
		if b.Ref == nil || b.Start == nil || b.Minutes == nil {
			return DayPlanOutput{}, fmt.Errorf("block %d lacks ref, start, or minutes", i)
		}
		out.Items[i] = PlannedBlock{Ref: *b.Ref, Start: *b.Start, Minutes: *b.Minutes}
	}
	if h := out.Hours; h != nil && h.Start == nil && h.WorkMinutes == nil {
		out.Hours = nil
	}
	return out, nil
}

// ValidateDayPlan applies the rules of docs/07-integrations.md#day-plan.
func ValidateDayPlan(out DayPlanOutput, req DayPlanRequest) error {
	if n := len([]rune(strings.TrimSpace(out.Reply))); n < 1 || n > MaxMessage {
		return fmt.Errorf("the reply must be 1 to %d characters", MaxMessage)
	}
	if len(out.Items) > MaxBlocks {
		return fmt.Errorf("a plan has at most %d blocks, got %d", MaxBlocks, len(out.Items))
	}
	start, err := Clock(req.Hours.Start)
	if err != nil {
		return fmt.Errorf("the sent window start: %w", err)
	}
	end, err := Clock(req.Hours.End)
	if err != nil {
		return fmt.Errorf("the sent window end: %w", err)
	}
	if h := out.Hours; h != nil {
		if h.Start != nil {
			s, err := Clock(*h.Start)
			if err != nil || s >= end {
				return fmt.Errorf("hours.start %q is not a time before %s", *h.Start, req.Hours.End)
			}
			start = s
		}
		if h.WorkMinutes != nil && (*h.WorkMinutes < 0 || *h.WorkMinutes > 24*60) {
			return fmt.Errorf("hours.work_minutes %d is outside 0 to 1440", *h.WorkMinutes)
		}
	}
	free := make([][2]int, len(req.Free))
	for i, f := range req.Free {
		s, err1 := Clock(f.Start)
		e, err2 := Clock(f.End)
		if err1 != nil || err2 != nil {
			return fmt.Errorf("free interval %d is not two times", i)
		}
		free[i] = [2]int{s, e}
	}
	refs := map[string]bool{}
	for _, t := range req.Tasks {
		refs[t.Ref] = true
	}
	spans := make([][2]int, len(out.Items))
	for i, b := range out.Items {
		if !refs[b.Ref] {
			return fmt.Errorf("block %d: %q is not one of the tasks", i, b.Ref)
		}
		s, err := Clock(b.Start)
		if err != nil {
			return fmt.Errorf("block %d: %w", i, err)
		}
		if b.Minutes < 5 || b.Minutes > 480 || b.Minutes%5 != 0 {
			return fmt.Errorf("block %d: minutes %d is not a multiple of 5 from 5 to 480", i, b.Minutes)
		}
		e := s + b.Minutes
		if s < start {
			return fmt.Errorf("block %d: %s is before the day starts at %s", i, b.Start, minuteClock(start))
		}
		if !slices.ContainsFunc(free, func(f [2]int) bool { return f[0] <= s && e <= f[1] }) {
			return fmt.Errorf("block %d: %s to %s is not inside the free time", i, b.Start, minuteClock(e))
		}
		spans[i] = [2]int{s, e}
	}
	for i := range spans {
		for j := range i {
			if spans[i][0] < spans[j][1] && spans[j][0] < spans[i][1] {
				return fmt.Errorf("blocks %d and %d overlap", j, i)
			}
		}
	}
	return nil
}

// Clock parses HH:MM, 24-hour, as minutes after midnight.
func Clock(s string) (int, error) {
	digit := func(i int) int { return int(s[i] - '0') }
	if len(s) != 5 || s[2] != ':' || strings.IndexFunc(s[:2]+s[3:], func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, fmt.Errorf("%q is not HH:MM", s)
	}
	h, m := digit(0)*10+digit(1), digit(3)*10+digit(4)
	if h > 23 || m > 59 {
		return 0, fmt.Errorf("%q is not HH:MM", s)
	}
	return h*60 + m, nil
}

// minuteClock writes minutes after midnight as HH:MM.
func minuteClock(m int) string { return fmt.Sprintf("%02d:%02d", m/60, m%60) }
