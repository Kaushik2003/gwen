package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/planner"
	"github.com/kzark/gwen/internal/planner/civil"
)

// The data lane's spans: what a query reads when it names no time, and what
// insights look at.
const (
	dataDays    = 7
	insightDays = 28
	// maxDataDays bounds a window to what the stats read at once.
	maxDataDays = 366
	// maxTitles is how many finished tasks a summary names.
	maxTitles = 12
)

// Refs of the data lane's summaries.
const (
	dataWorked = "worked"
	dataDaily  = "worked_by_day"
	dataTasks  = "tasks_done"
	dataGoals  = "goals"
	dataEnergy = "energy"
)

// dataLane answers questions about the user's tracked data by fixed queries
// (recall.DataLane): time worked against target, tasks done, goals and their
// pace, and energy check-ins.
type dataLane struct{ s *Service }

// dataTopics are the words that ask about each summary. A message that asks
// about none of them gets no summary, so the data does not crowd every note.
var dataTopics = map[string]*regexp.Regexp{
	dataWorked: regexp.MustCompile(`(?i)\b(hours?|worked|tracked|productive|overtime|targets?)\b|` +
		`\bhow (much|long)\b.*\b(work|working|study|studying)\b`),
	dataTasks: regexp.MustCompile(`(?i)\b(tasks?|to-?dos?|finish(ed)?|complete(d)?|accomplish(ed)?)\b|` +
		`\b(got|get|gotten|getting)\b.*\bdone\b`),
	dataGoals:  regexp.MustCompile(`(?i)\b(goals?|pace|progress|on track|behind)\b`),
	dataEnergy: regexp.MustCompile(`(?i)\b(energy|tired|exhausted|drained|burn(ed|t)? out)\b`),
}

// Search summarises the tracked data of w, or of the last 7 days: the
// summaries the query asks about, or all of them for a query of intent data.
func (d *dataLane) Search(ctx context.Context, q memory.Query, w *memory.TimeRange) ([]memory.Hit, error) {
	want := map[string]bool{}
	for name, re := range dataTopics {
		if q.Intent == memory.IntentData || re.MatchString(q.Text) {
			want[name] = true
		}
	}
	if len(want) == 0 {
		return nil, nil
	}
	want[dataDaily] = want[dataWorked]
	from, to := d.span(w, dataDays)
	hits, err := d.hits(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := hits[:0]
	for _, h := range hits {
		if _, name, _ := memory.ParseRef(h.Ref); want[name] {
			out = append(out, h)
		}
	}
	return out, nil
}

// summaries are the last 28 days' summaries, for insights.
func (d *dataLane) summaries(ctx context.Context) ([]string, error) {
	from, to := d.span(nil, insightDays)
	hits, err := d.hits(ctx, from, to)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.Text
	}
	return out, nil
}

// span is the days of w, the last n days when w is nil; a span is at most a
// year and never ends after today.
func (d *dataLane) span(w *memory.TimeRange, n int) (from, to civil.Day) {
	today := civil.MustParse(d.s.today())
	if w == nil {
		return today.AddDays(1 - n), today
	}
	from = civil.MustParse(d.s.o.Day(w.From))
	to = civil.MustParse(d.s.o.Day(w.To.Add(-time.Millisecond)))
	if to.After(today) {
		to = today
	}
	if to.Before(from) {
		from = to
	}
	if from.DaysUntil(to) >= maxDataDays {
		from = to.AddDays(1 - maxDataDays)
	}
	return from, to
}

// hits are the summaries of [from, to], days inclusive.
func (d *dataLane) hits(ctx context.Context, from, to civil.Day) ([]memory.Hit, error) {
	s := d.s
	now := s.now()
	when := spanWords(from, to)
	hit := func(name, text string) memory.Hit {
		return memory.Hit{Ref: memory.Ref(memory.RefData, name), Kind: "data", Text: text, At: now,
			Source: "your tracked data", Trust: model.TrustObserved, Sensitivity: model.SensitivityPersonal}
	}
	var out []memory.Hit

	days, err := s.o.Repos.Stats.Days(ctx, from.String(), to.String(), now)
	if err != nil {
		return nil, fmt.Errorf("data lane: %w", err)
	}
	var worked, target time.Duration
	met := 0
	daily := make([]string, 0, len(days))
	for _, day := range days {
		worked += day.Worked
		target += day.Target
		if day.TargetMet {
			met++
		}
		daily = append(daily, fmt.Sprintf("%s %s of %s", dayWords(civil.MustParse(day.Day)), duration(day.Worked),
			duration(day.Target)))
	}
	if len(days) == 0 {
		out = append(out, hit(dataWorked, fmt.Sprintf("%s they tracked no work.", capitalize(when))))
	} else {
		out = append(out, hit(dataWorked, fmt.Sprintf("%s they worked %s against a target of %s, on %d %s; they met "+
			"the day's target on %d of them.", capitalize(when), duration(worked), duration(target), len(days),
			plural(len(days), "day", "days"), met)))
		out = append(out, hit(dataDaily, fmt.Sprintf("Time worked by day, %s: %s.", strings.TrimPrefix(when, "from "),
			strings.Join(daily, ", "))))
	}

	titles, err := s.o.Repos.LLMRuns.Completed(ctx, from.Midnight(s.loc), to.AddDays(1).Midnight(s.loc))
	if err != nil {
		return nil, fmt.Errorf("data lane: %w", err)
	}
	if len(titles) > 0 {
		named := make([]string, 0, min(len(titles), maxTitles))
		for _, t := range titles[:min(len(titles), maxTitles)] {
			named = append(named, "“"+t+"”")
		}
		more := ""
		if len(titles) > maxTitles {
			more = fmt.Sprintf(", and %d more", len(titles)-maxTitles)
		}
		out = append(out, hit(dataTasks, fmt.Sprintf("%s they finished %d %s: %s%s.", capitalize(when), len(titles),
			plural(len(titles), "task", "tasks"), strings.Join(named, ", "), more)))
	} else {
		out = append(out, hit(dataTasks, fmt.Sprintf("%s they finished no tasks.", capitalize(when))))
	}

	goals, err := s.o.Repos.Goals.List(ctx, model.GoalActive)
	if err != nil {
		return nil, fmt.Errorf("data lane: %w", err)
	}
	if len(goals) > 0 {
		progress, err := s.o.Repos.Plans.Progress(ctx, goals, api.NewPlanEnv(s.o.Config(), s.loc, now))
		if err != nil {
			return nil, fmt.Errorf("data lane: %w", err)
		}
		lines := make([]string, len(goals))
		for i, g := range goals {
			lines[i] = goalWords(g, progress[g.ID])
		}
		out = append(out, hit(dataGoals, "Their active goals: "+strings.Join(lines, "; ")+"."))
	}

	logs, err := s.o.Repos.Energy.List(ctx, from.Midnight(s.loc), to.AddDays(1).Midnight(s.loc))
	if err != nil {
		return nil, fmt.Errorf("data lane: %w", err)
	}
	if len(logs) > 0 {
		sum, low, high := 0, logs[0].Level, logs[0].Level
		for _, l := range logs {
			sum += l.Level
			low, high = min(low, l.Level), max(high, l.Level)
		}
		out = append(out, hit(dataEnergy, fmt.Sprintf("%s they checked in their energy %d %s, %.1f of 5 on average "+
			"(lowest %d, highest %d).", capitalize(when), len(logs), plural(len(logs), "time", "times"),
			float64(sum)/float64(len(logs)), low, high)))
	}
	return out, nil
}

// goalWords is a goal and its pace today in words.
func goalWords(g model.Goal, p planner.GoalProgress) string {
	unit := g.Unit
	if unit == "" {
		unit = "tasks"
	}
	out := fmt.Sprintf("“%s”, %d of %d %s done", g.Title, p.Done, p.Target, unit)
	switch p.Pace {
	case planner.PaceAhead:
		out += ", ahead of pace"
	case planner.PaceBehind:
		out += ", behind pace"
	case planner.PaceOnTrack:
		out += ", on track"
	}
	if d, err := civil.Parse(g.DueDay); err == nil {
		out += ", due " + dayWords(d)
	}
	return out
}

// spanWords is a span of days in words: "on Mon 5 Oct", "from 5 to 11 Oct",
// "from 28 Sep to 4 Oct", or with years when they differ.
func spanWords(from, to civil.Day) string {
	switch {
	case from == to:
		return "on " + dayWords(from)
	case from.Y != to.Y:
		return fmt.Sprintf("from %d %s %d to %d %s %d", from.D, from.M.String()[:3], from.Y, to.D, to.M.String()[:3], to.Y)
	case from.M != to.M:
		return fmt.Sprintf("from %d %s to %d %s", from.D, from.M.String()[:3], to.D, to.M.String()[:3])
	}
	return fmt.Sprintf("from %d to %d %s", from.D, to.D, to.M.String()[:3])
}

// dayWords is a day as "Mon 5 Oct".
func dayWords(d civil.Day) string {
	return fmt.Sprintf("%s %d %s", d.Weekday().String()[:3], d.D, d.M.String()[:3])
}

// duration is a span of time as "7h 32m", "8h", or "45m".
func duration(d time.Duration) string {
	d = d.Round(time.Minute)
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
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

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
