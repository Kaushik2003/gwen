package planner

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/kzark/gwen/internal/planner/civil"
)

// MaxReminders is the most reminders a briefing carries.
const MaxReminders = 5

// Reminder is a nudge about a task's due day.
type Reminder struct {
	TaskID   string
	Title    string
	DueDay   civil.Day
	DaysLeft int
	Message  string
}

// ReminderInput is everything Reminders needs.
type ReminderInput struct {
	Today civil.Day
	// Tasks are the live open tasks; those with a due day are considered.
	Tasks   []Task
	Tracked map[string]time.Duration // all-time tracked work per task
	// PlannedToday holds the tasks with any live item on today.
	PlannedToday map[string]bool
	// DayCapacity[i] is capacity_minutes on today+i, for i from 0 to 13.
	DayCapacity []int
	Progress    map[string]GoalProgress // active goals
	// RolloverCounts are the tasks' rollover counts on today.
	RolloverCounts map[string]int
}

// Reminders computes the briefing's reminders
// (docs/06-planner.md#reminders): at most five, by urgency for today.
func Reminders(in ReminderInput) []Reminder {
	type candidate struct {
		task Task
		r    Reminder
	}
	var found []candidate
	for _, t := range in.Tasks {
		due, ok := t.due()
		if !ok || !t.live() || !t.open() || t.IsTemplate() {
			continue
		}
		left := in.Today.DaysUntil(due)
		remaining := RemainingMinutes(t, in.Tracked[t.ID])
		var msg string
		switch {
		case left < 0:
			msg = fmt.Sprintf("Overdue by %d %s", -left, plural(-left, "day", "days"))
		case left == 0:
			msg = "Due today"
		case left == 1:
			msg = "Due tomorrow"
		case left <= 14 && float64(remaining) > 0.25*float64(in.avail(left)):
			hours := math.Round(float64(remaining)/60*10) / 10
			msg = fmt.Sprintf("Start now — due in %d days, about %sh of work left", left,
				strconv.FormatFloat(hours, 'f', -1, 64))
		case left <= 7 && in.Tracked[t.ID] == 0 && !in.PlannedToday[t.ID]:
			msg = fmt.Sprintf("Not started — due in %d days", left)
		default:
			continue
		}
		found = append(found, candidate{t, Reminder{TaskID: t.ID, Title: t.Title, DueDay: due, DaysLeft: left, Message: msg}})
	}
	tasks := make([]Task, len(found))
	score := map[string]float64{}
	byID := map[string]Reminder{}
	for i, c := range found {
		tasks[i] = c.task
		score[c.task.ID] = Urgency(c.task, in.Today, in.Progress, in.RolloverCounts[c.task.ID])
		byID[c.task.ID] = c.r
	}
	sortByUrgency(tasks, score)
	out := []Reminder{}
	for _, t := range tasks[:min(len(tasks), MaxReminders)] {
		out = append(out, byID[t.ID])
	}
	return out
}

// avail is the capacity from today to the day before a due day daysLeft away.
func (in ReminderInput) avail(daysLeft int) int {
	sum := 0
	for i := 0; i < daysLeft && i < len(in.DayCapacity); i++ {
		sum += in.DayCapacity[i]
	}
	return sum
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
