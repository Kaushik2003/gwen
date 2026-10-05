package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// resolve finds the item among xs whose id matches s, a full id or a unique
// suffix of at least 8 characters.
func resolve[T any](s, what string, xs []T, id func(T) string) (T, error) {
	var zero T
	ids := make([]string, len(xs))
	for i, x := range xs {
		ids[i] = id(x)
	}
	got, err := client.ResolveID(s, ids)
	if err != nil {
		if errors.Is(err, client.ErrUnknownID) {
			return zero, fmt.Errorf("no %s matches %q", what, s)
		}
		return zero, err
	}
	for _, x := range xs {
		if id(x) == got {
			return x, nil
		}
	}
	return zero, fmt.Errorf("no %s matches %q", what, s)
}

func (c *cli) goal(ctx context.Context, s string) (wire.Goal, error) {
	all, err := c.api.ListGoals(ctx, wire.GoalsAll)
	if err != nil {
		return wire.Goal{}, err
	}
	return resolve(s, "goal", all.Goals, func(g wire.Goal) string { return g.ID })
}

func (c *cli) goalFlag(ctx context.Context, s string) (*string, error) {
	if s == "" || strings.EqualFold(s, "none") {
		return nil, nil
	}
	g, err := c.goal(ctx, s)
	if err != nil {
		return nil, err
	}
	return &g.ID, nil
}

func (c *cli) commitment(ctx context.Context, s string) (wire.Commitment, error) {
	all, err := c.api.ListCommitments(ctx)
	if err != nil {
		return wire.Commitment{}, err
	}
	return resolve(s, "commitment", all.Commitments, func(m wire.Commitment) string { return m.ID })
}

// planItem resolves an item among the plan of day.
func (c *cli) planItem(ctx context.Context, day, s string) (wire.PlanItem, error) {
	p, err := c.api.GetPlan(ctx, day)
	if err != nil {
		return wire.PlanItem{}, err
	}
	return resolve(s, "plan item on "+p.Day, p.Items, func(it wire.PlanItem) string { return it.ID })
}

func minutesFlag(flag, s string) (int, error) {
	d, err := duration(flag, s)
	if err != nil {
		return 0, err
	}
	if d < time.Minute {
		return 0, usagef("--%s must be at least a minute", flag)
	}
	return int(d / time.Minute), nil
}

func (c *cli) goalCmd() *cobra.Command {
	goal := group("goal", "List and change goals")

	var status string
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List goals",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.ListGoals(cmd.Context(), status)
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, g := range list.Goals {
					goalRow(tw, g)
				}
				return tw.Flush()
			})
		},
	}
	ls.Flags().StringVar(&status, "status", "", "active (default), done, abandoned, or all")

	show := &cobra.Command{
		Use:   "show G",
		Short: "Show a goal and its progress",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			g, err := c.goal(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			got, err := c.api.GetGoal(cmd.Context(), g.ID)
			if err != nil {
				return err
			}
			return c.emit(got, func(w io.Writer) error {
				if err := printGoal(w, *got); err != nil {
					return err
				}
				p := got.Progress
				_, err := fmt.Fprintf(w, "Needs %s a day · doing %s a day · started %s\n",
					perDay(p.RequiredPerDay), perDay(p.ActualPerDay), got.StartDay)
				return err
			})
		},
	}

	var f goalFlags
	add := &cobra.Command{
		Use:   "add TITLE",
		Short: "Create a goal: a quantity with --quantity, else a set of tasks",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			if f.due == "" {
				return usagef("--due is required")
			}
			req := wire.CreateGoalRequest{Title: a[0], Kind: wire.GoalTasks}
			var err error
			if req.DueDay, err = c.day(ctx, f.due); err != nil {
				return err
			}
			if req.StartDay, err = c.day(ctx, f.start); err != nil {
				return err
			}
			if req.ProjectID, err = c.projectFlag(ctx, f.project); err != nil {
				return err
			}
			changed := cmd.Flags().Changed
			if changed("quantity") {
				if !changed("per-unit") {
					return usagef("--quantity needs --per-unit, the time one unit takes")
				}
				m, err := minutesFlag("per-unit", f.perUnit)
				if err != nil {
					return err
				}
				req.Kind, req.TargetQuantity, req.MinutesPerUnit = wire.GoalQuantity, &f.quantity, &m
				if changed("daily") {
					d, err := minutesFlag("daily", f.daily)
					if err != nil {
						return err
					}
					req.DailyMinutes = &d
				}
			} else if changed("per-unit") || changed("unit") || changed("daily") {
				return usagef("--unit, --per-unit, and --daily need --quantity")
			}
			if changed("unit") {
				req.Unit = &f.unit
			}
			g, err := c.api.CreateGoal(ctx, req)
			if err != nil {
				return err
			}
			return c.printGoal(g)
		},
	}
	f.register(add, false)

	var e goalFlags
	edit := &cobra.Command{
		Use:   "edit G",
		Short: "Change a goal",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			g, err := c.goal(ctx, a[0])
			if err != nil {
				return err
			}
			var req wire.PatchGoalRequest
			changed := cmd.Flags().Changed
			if changed("title") {
				req.Title = &e.title
			}
			if changed("due") {
				d, err := c.day(ctx, e.due)
				if err != nil {
					return err
				}
				req.DueDay = &d
			}
			if changed("start") {
				d, err := c.day(ctx, e.start)
				if err != nil {
					return err
				}
				req.StartDay = &d
			}
			if changed("project") {
				p, err := c.projectFlag(ctx, e.project)
				if err != nil {
					return err
				}
				req.ProjectID = wire.FromPtr(p)
			}
			if changed("quantity") {
				req.TargetQuantity = wire.Some(e.quantity)
			}
			if changed("unit") {
				req.Unit = &e.unit
			}
			if changed("per-unit") {
				m, err := minutesFlag("per-unit", e.perUnit)
				if err != nil {
					return err
				}
				req.MinutesPerUnit = wire.Some(m)
			}
			if changed("daily") {
				if e.daily == "none" {
					req.DailyMinutes = wire.Null[int]()
				} else {
					m, err := minutesFlag("daily", e.daily)
					if err != nil {
						return err
					}
					req.DailyMinutes = wire.Some(m)
				}
			}
			if changed("status") {
				req.Status = &e.status
			}
			got, err := c.api.PatchGoal(ctx, g.ID, req)
			if err != nil {
				return err
			}
			return c.printGoal(got)
		},
	}
	e.register(edit, true)

	var withTasks bool
	rm := &cobra.Command{
		Use:   "rm G",
		Short: "Delete a goal and its session template",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			g, err := c.goal(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			if err := c.api.DeleteGoal(cmd.Context(), g.ID, withTasks); err != nil {
				return err
			}
			if !c.json {
				fmt.Fprintf(c.stdout, "Deleted goal %s.\n", g.Title)
			}
			return nil
		},
	}
	rm.Flags().BoolVar(&withTasks, "with-tasks", false, "also delete its open tasks and lined-up items")
	goal.AddCommand(ls, show, add, edit, rm, c.goalBreakdownCmd())
	return goal
}

// goalFlags are the fields goal add and goal edit share.
type goalFlags struct {
	title, due, start, unit, perUnit, daily, project, status string
	quantity                                                 int
}

func (f *goalFlags) register(cmd *cobra.Command, edit bool) {
	cmd.Flags().StringVar(&f.due, "due", "", "last day, YYYY-MM-DD")
	cmd.Flags().StringVar(&f.start, "start", "", "first day, YYYY-MM-DD (default: today)")
	cmd.Flags().IntVar(&f.quantity, "quantity", 0, "how many units, such as 300 problems")
	cmd.Flags().StringVar(&f.unit, "unit", "", "what a unit is called, such as problems")
	cmd.Flags().StringVar(&f.perUnit, "per-unit", "", "time one unit takes, such as 30m")
	cmd.Flags().StringVar(&f.daily, "daily", "", "time a day you can give it, such as 2h; none with edit clears")
	cmd.Flags().StringVar(&f.project, "project", "", "project id, short id, or name; none for no project")
	if edit {
		cmd.Flags().StringVar(&f.title, "title", "", "new title")
		cmd.Flags().StringVar(&f.status, "status", "", "active, done, or abandoned")
	}
}

func perDay(x float64) string { return strconv.FormatFloat(x, 'f', 1, 64) }

// goalRow is "id  active  Title  12/300 problems  behind  due 2026-12-15  finish 2027-01-02".
func goalRow(w io.Writer, g wire.Goal) {
	p := g.Progress
	unit := g.Unit
	if g.Kind == wire.GoalTasks {
		unit = "tasks"
	}
	amount := fmt.Sprintf("%d/%d", p.DoneQuantity, p.DoneQuantity+p.RemainingQuantity)
	if unit != "" {
		amount += " " + unit
	}
	finish := ""
	if p.ProjectedFinishDay != nil {
		finish = "finish " + *p.ProjectedFinishDay
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\tdue %s\t%s\n", client.ShortID(g.ID), g.Status, g.Title, amount, p.Pace,
		g.DueDay, finish)
}

func printGoal(w io.Writer, g wire.Goal) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	goalRow(tw, g)
	return tw.Flush()
}

func (c *cli) printGoal(g *wire.Goal) error {
	return c.emit(g, func(w io.Writer) error { return printGoal(w, *g) })
}

func (c *cli) commitCmd() *cobra.Command {
	commit := group("commit", "List and change commitments: recurring time that is not planned work")

	ls := &cobra.Command{
		Use:   "ls",
		Short: "List commitments",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.ListCommitments(cmd.Context())
			if err != nil {
				return err
			}
			names := map[string]string{}
			if !c.json {
				if names, err = c.projectNames(cmd.Context()); err != nil {
					return err
				}
			}
			return c.emit(list, func(w io.Writer) error {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, m := range list.Commitments {
					commitmentRow(tw, m, names)
				}
				return tw.Flush()
			})
		},
	}

	var f commitFlags
	add := &cobra.Command{
		Use:   "add TITLE",
		Short: "Create a commitment",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			if f.rrule == "" || f.duration == "" {
				return usagef("--rrule and --duration are required")
			}
			req := wire.CreateCommitmentRequest{Title: a[0], RRule: f.rrule}
			var err error
			if req.DurationMinutes, err = minutesFlag("duration", f.duration); err != nil {
				return err
			}
			if f.at != "" {
				if req.StartMinute, err = startMinute(f.at); err != nil {
					return err
				}
			}
			if req.ProjectID, err = c.projectFlag(ctx, f.project); err != nil {
				return err
			}
			if f.noCount {
				no := false
				req.CountsTowardTarget = &no
			}
			if req.ActiveFrom, err = c.day(ctx, f.from); err != nil {
				return err
			}
			if f.until != "" {
				d, err := c.day(ctx, f.until)
				if err != nil {
					return err
				}
				req.ActiveUntil = &d
			}
			m, err := c.api.CreateCommitment(ctx, req)
			if err != nil {
				return err
			}
			return c.printCommitment(ctx, m)
		},
	}
	f.register(add, false)

	var e commitFlags
	edit := &cobra.Command{
		Use:   "edit C",
		Short: "Change a commitment; --at none makes it floating",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			m, err := c.commitment(ctx, a[0])
			if err != nil {
				return err
			}
			var req wire.PatchCommitmentRequest
			changed := cmd.Flags().Changed
			if changed("title") {
				req.Title = &e.title
			}
			if changed("rrule") {
				req.RRule = &e.rrule
			}
			if changed("duration") {
				d, err := minutesFlag("duration", e.duration)
				if err != nil {
					return err
				}
				req.DurationMinutes = &d
			}
			if changed("at") {
				req.StartMinute = wire.Null[int]()
				if !strings.EqualFold(e.at, "none") {
					s, err := startMinute(e.at)
					if err != nil {
						return err
					}
					req.StartMinute = wire.Some(*s)
				}
			}
			if changed("project") {
				p, err := c.projectFlag(ctx, e.project)
				if err != nil {
					return err
				}
				req.ProjectID = wire.FromPtr(p)
			}
			switch {
			case e.noCount && e.count:
				return usagef("give --count or --no-count, not both")
			case e.noCount:
				req.CountsTowardTarget = new(bool)
			case e.count:
				yes := true
				req.CountsTowardTarget = &yes
			}
			if changed("from") {
				d, err := c.day(ctx, e.from)
				if err != nil {
					return err
				}
				req.ActiveFrom = &d
			}
			if changed("until") {
				req.ActiveUntil = wire.Null[string]()
				if !strings.EqualFold(e.until, "none") {
					d, err := c.day(ctx, e.until)
					if err != nil {
						return err
					}
					req.ActiveUntil = wire.Some(d)
				}
			}
			got, err := c.api.PatchCommitment(ctx, m.ID, req)
			if err != nil {
				return err
			}
			return c.printCommitment(ctx, got)
		},
	}
	e.register(edit, true)

	rm := &cobra.Command{
		Use:   "rm C",
		Short: "Delete a commitment",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			m, err := c.commitment(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			if err := c.api.DeleteCommitment(cmd.Context(), m.ID); err != nil {
				return err
			}
			if !c.json {
				fmt.Fprintf(c.stdout, "Deleted commitment %s.\n", m.Title)
			}
			return nil
		},
	}
	commit.AddCommand(ls, add, edit, rm)
	return commit
}

// commitFlags are the fields commit add and commit edit share.
type commitFlags struct {
	title, rrule, duration, at, project, from, until string
	noCount, count                                   bool
}

func (f *commitFlags) register(cmd *cobra.Command, edit bool) {
	cmd.Flags().StringVar(&f.rrule, "rrule", "", "recurrence, such as FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR")
	cmd.Flags().StringVar(&f.duration, "duration", "", "how long it takes, such as 5h")
	cmd.Flags().StringVar(&f.at, "at", "", "start time HH:MM (default: floating, no fixed time)")
	cmd.Flags().StringVar(&f.project, "project", "", "project id, short id, or name; none for no project")
	cmd.Flags().BoolVar(&f.noCount, "no-count", false, "do not count it toward the daily target")
	cmd.Flags().StringVar(&f.from, "from", "", "first day, YYYY-MM-DD (default: today)")
	cmd.Flags().StringVar(&f.until, "until", "", "last day, YYYY-MM-DD")
	if edit {
		cmd.Flags().StringVar(&f.title, "title", "", "new title")
		cmd.Flags().BoolVar(&f.count, "count", false, "count it toward the daily target")
	}
}

func startMinute(hhmm string) (*int, error) {
	tod, err := config.ParseTimeOfDay(hhmm)
	if err != nil {
		return nil, usagef("--at %q is not a time like 09:30", hhmm)
	}
	m := int(tod)
	return &m, nil
}

// commitmentRow is "id  10:00–15:00  5h  Internship  Title  FREQ=…  counts  from D".
func commitmentRow(w io.Writer, m wire.Commitment, names map[string]string) {
	when := "floating"
	if m.StartMinute != nil {
		end := *m.StartMinute + m.DurationMinutes
		when = config.TimeOfDay(*m.StartMinute).String() + "–" + config.TimeOfDay(end%1440).String()
	}
	project := ""
	if m.ProjectID != nil {
		project = names[*m.ProjectID]
	}
	counts := "counts"
	if !m.CountsTowardTarget {
		counts = "extra"
	}
	active := "from " + m.ActiveFrom
	if m.ActiveUntil != nil {
		active += " until " + *m.ActiveUntil
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", client.ShortID(m.ID), when,
		client.FormatDuration(time.Duration(m.DurationMinutes)*time.Minute), project, m.Title, m.RRule, counts, active)
}

func (c *cli) printCommitment(ctx context.Context, m *wire.Commitment) error {
	if c.json {
		return c.emit(m, nil)
	}
	names, err := c.projectNames(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	commitmentRow(tw, *m, names)
	return tw.Flush()
}

func (c *cli) planCmd() *cobra.Command {
	plan := &cobra.Command{
		Use:   "plan [DAY]",
		Short: "Show the plan for a day (default: today, generated on first look)",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			day := ""
			if len(a) == 1 {
				var err error
				if day, err = c.day(cmd.Context(), a[0]); err != nil {
					return err
				}
			}
			p, err := c.api.GetPlan(cmd.Context(), day)
			if err != nil {
				return err
			}
			return c.printPlan(p)
		},
	}
	gen := &cobra.Command{
		Use:   "gen [DAY]",
		Short: "Regenerate the plan for today or a later day, keeping pinned items",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			day := ""
			if len(a) == 1 {
				day = a[0]
			}
			d, err := c.day(cmd.Context(), day)
			if err != nil {
				return err
			}
			p, err := c.api.GeneratePlan(cmd.Context(), wire.GeneratePlanRequest{Day: d})
			if err != nil {
				return err
			}
			return c.printPlan(p)
		},
	}

	var moveDay, at string
	var position, minutes int
	move := &cobra.Command{
		Use:   "move ITEM",
		Short: "Move, reorder, or resize a plan item, which pins it",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			day, err := c.day(ctx, moveDay)
			if err != nil {
				return err
			}
			var req wire.PatchPlanItemRequest
			changed := cmd.Flags().Changed
			if changed("at") {
				t, err := c.instant(ctx, day, at)
				if err != nil {
					return err
				}
				req.StartAt = wire.Some(wire.Millis(t))
			}
			if changed("position") {
				req.Position = &position
			}
			if changed("minutes") {
				req.PlannedMinutes = &minutes
			}
			if !req.StartAt.Set && req.Position == nil && req.PlannedMinutes == nil {
				return usagef("give --at, --position, or --minutes")
			}
			return c.patchPlanItem(ctx, day, a[0], req)
		},
	}
	move.Flags().StringVar(&moveDay, "day", "", "the plan's day (default: today)")
	move.Flags().StringVar(&at, "at", "", "new start time HH:MM")
	move.Flags().IntVar(&position, "position", 0, "new position, counting from 0")
	move.Flags().IntVar(&minutes, "minutes", 0, "new length in minutes")

	status := func(use, short, to string) *cobra.Command {
		var day string
		cmd := &cobra.Command{
			Use:   use,
			Short: short,
			Args:  args(1),
			RunE: func(cmd *cobra.Command, a []string) error {
				d, err := c.day(cmd.Context(), day)
				if err != nil {
					return err
				}
				return c.patchPlanItem(cmd.Context(), d, a[0], wire.PatchPlanItemRequest{Status: &to})
			},
		}
		cmd.Flags().StringVar(&day, "day", "", "the plan's day (default: today)")
		return cmd
	}
	plan.AddCommand(gen, move, c.planHoursCmd(), c.planChatCmd(),
		status("skip ITEM", "Skip a plan item", wire.PlanSkipped),
		status("unskip ITEM", "Plan a skipped item again", wire.PlanPlanned))
	return plan
}

// planHoursCmd is gwen plan hours: a day's own start and time for tasks
// (docs/06-planner.md#day-hours). A flag left out keeps the day's value.
func (c *cli) planHoursCmd() *cobra.Command {
	var from, work string
	var usual bool
	cmd := &cobra.Command{
		Use:   "hours [DAY]",
		Short: "Set when a day's work starts and how much time it has for tasks, then replan it",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			changed := cmd.Flags().Changed
			switch {
			case usual && (changed("from") || changed("work")):
				return usagef("--usual clears both, so give it alone")
			case !usual && !changed("from") && !changed("work"):
				return usagef("give --from, --work, or --usual")
			}
			day := ""
			if len(a) == 1 {
				day = a[0]
			}
			d, err := c.day(ctx, day)
			if err != nil {
				return err
			}
			req := wire.SetDayHoursRequest{Day: d}
			if !usual {
				p, err := c.api.GetPlan(ctx, d)
				if err != nil {
					return err
				}
				if p.Hours != nil {
					req.StartMinute, req.WorkMinutes = p.Hours.StartMinute, p.Hours.WorkMinutes
				}
			}
			if changed("from") {
				req.StartMinute = nil
				if !strings.EqualFold(from, "usual") {
					tod, err := config.ParseTimeOfDay(from)
					if err != nil {
						return usagef("--from %q is not a time like 19:00", from)
					}
					m := int(tod)
					req.StartMinute = &m
				}
			}
			if changed("work") {
				req.WorkMinutes = nil
				if !strings.EqualFold(work, "usual") {
					dur, err := time.ParseDuration(work)
					if err != nil || dur < 0 {
						return usagef("--work %q is not a duration like 3h", work)
					}
					m := int(dur / time.Minute)
					req.WorkMinutes = &m
				}
			}
			p, err := c.api.SetDayHours(ctx, req)
			if err != nil {
				return err
			}
			return c.printPlan(p)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "when the day's work starts, HH:MM, or usual")
	cmd.Flags().StringVar(&work, "work", "", "time for tasks, such as 3h, or usual")
	cmd.Flags().BoolVar(&usual, "usual", false, "go back to the usual hours")
	return cmd
}

func (c *cli) patchPlanItem(ctx context.Context, day, arg string, req wire.PatchPlanItemRequest) error {
	it, err := c.planItem(ctx, day, arg)
	if err != nil {
		return err
	}
	got, err := c.api.PatchPlanItem(ctx, it.ID, req)
	if err != nil {
		return err
	}
	return c.emit(got, func(w io.Writer) error {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		c.planRow(tw, *got)
		return tw.Flush()
	})
}

// planRow is "id  09:00  1h 30m  planned  pinned  Title".
func (c *cli) planRow(w io.Writer, it wire.PlanItem) {
	start := "--:--"
	if it.StartAt != nil {
		start = client.FormatTime(wire.Time(*it.StartAt).In(c.loc))
	}
	pinned := ""
	if it.Pinned {
		pinned = "pinned"
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", client.ShortID(it.ID), start,
		client.FormatDuration(time.Duration(it.PlannedMinutes)*time.Minute), it.Status, pinned, it.Task.Title)
}

func (c *cli) printPlan(p *wire.Plan) error {
	return c.emit(p, func(w io.Writer) error {
		fmt.Fprintf(w, "%s · %s planned of %s capacity\n", p.Day,
			client.FormatDuration(time.Duration(p.PlannedMinutes)*time.Minute),
			client.FormatDuration(time.Duration(p.CapacityMinutes)*time.Minute))
		if h := hoursLine(p.Hours); h != "" {
			fmt.Fprintf(w, "Hours: %s\n", h)
		}
		if len(p.Items) == 0 {
			_, err := fmt.Fprintln(w, "Nothing planned.")
			return err
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, it := range p.Items {
			c.planRow(tw, it)
		}
		return tw.Flush()
	})
}

// hoursLine is "from 19:00 · 3h for tasks", or "" for no hours.
func hoursLine(h *wire.DayHours) string {
	if h == nil {
		return ""
	}
	var parts []string
	if h.StartMinute != nil {
		parts = append(parts, fmt.Sprintf("from %02d:%02d", *h.StartMinute/60, *h.StartMinute%60))
	}
	switch {
	case h.WorkMinutes == nil:
	case *h.WorkMinutes == 0:
		parts = append(parts, "no time for tasks")
	default:
		parts = append(parts, client.FormatDuration(time.Duration(*h.WorkMinutes)*time.Minute)+" for tasks")
	}
	return strings.Join(parts, " · ")
}

func (c *cli) briefCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "brief",
		Short: "What to start today: pending work, today's plan, reminders, and goals",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := c.api.Briefing(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(b, func(w io.Writer) error { return c.printBriefing(w, b) })
		},
	}
}

func (c *cli) printBriefing(w io.Writer, b *wire.Briefing) error {
	fmt.Fprintf(w, "Briefing for %s\n", b.Day)
	if len(b.Pending)+len(b.Today)+len(b.Reminders)+len(b.Goals) == 0 {
		_, err := fmt.Fprintln(w, "Nothing planned, pending, or due.")
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if len(b.Pending) > 0 {
		fmt.Fprintln(tw, "\nPending from earlier")
		for _, it := range b.Pending {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", client.ShortID(it.TaskID), it.Day, it.Task.Title)
		}
	}
	if len(b.Today) > 0 {
		fmt.Fprintln(tw, "\nToday")
		for _, it := range b.Today {
			fmt.Fprint(tw, "  ")
			c.planRow(tw, it)
		}
	}
	if len(b.Reminders) > 0 {
		fmt.Fprintln(tw, "\nReminders")
		for _, r := range b.Reminders {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", client.ShortID(r.TaskID), r.Title, r.Message)
		}
	}
	if len(b.Goals) > 0 {
		fmt.Fprintln(tw, "\nGoals")
		for _, g := range b.Goals {
			fmt.Fprint(tw, "  ")
			goalRow(tw, g)
		}
	}
	return tw.Flush()
}
