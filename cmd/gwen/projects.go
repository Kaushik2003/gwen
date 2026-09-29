package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

func group(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
}

func (c *cli) projectCmd() *cobra.Command {
	project := group("project", "List and change projects")

	var archived, all bool
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List projects",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter := wire.ArchivedFalse
			switch {
			case archived && all:
				return usagef("give --archived or --all, not both")
			case archived:
				filter = wire.ArchivedTrue
			case all:
				filter = wire.ArchivedAll
			}
			list, err := c.api.ListProjects(cmd.Context(), filter)
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, p := range list.Projects {
					projectRow(tw, p)
				}
				return tw.Flush()
			})
		},
	}
	ls.Flags().BoolVar(&archived, "archived", false, "only archived projects")
	ls.Flags().BoolVar(&all, "all", false, "archived projects too")

	show := &cobra.Command{
		Use:   "show P",
		Short: "Show a project",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			p, err := c.project(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			got, err := c.api.GetProject(cmd.Context(), p.ID)
			if err != nil {
				return err
			}
			return c.printProject(got)
		},
	}

	var color string
	add := &cobra.Command{
		Use:   "add NAME",
		Short: "Create a project",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			req := wire.CreateProjectRequest{Name: a[0]}
			if cmd.Flags().Changed("color") {
				req.Color = &color
			}
			p, err := c.api.CreateProject(cmd.Context(), req)
			if err != nil {
				return err
			}
			return c.printProject(p)
		},
	}
	add.Flags().StringVar(&color, "color", "", "colour like #10b981 (default: the next palette colour)")

	var name, eColor string
	edit := &cobra.Command{
		Use:   "edit P",
		Short: "Rename or recolour a project",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			var req wire.PatchProjectRequest
			if cmd.Flags().Changed("name") {
				req.Name = &name
			}
			if cmd.Flags().Changed("color") {
				req.Color = &eColor
			}
			if req.Name == nil && req.Color == nil {
				return usagef("give --name or --color")
			}
			return c.patchProject(cmd.Context(), a[0], req)
		},
	}
	edit.Flags().StringVar(&name, "name", "", "new name")
	edit.Flags().StringVar(&eColor, "color", "", "new colour like #10b981")

	archive := &cobra.Command{
		Use:   "archive P",
		Short: "Archive a project",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			yes := true
			return c.patchProject(cmd.Context(), a[0], wire.PatchProjectRequest{Archived: &yes})
		},
	}
	unarchive := &cobra.Command{
		Use:   "unarchive P",
		Short: "Bring an archived project back",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			no := false
			return c.patchProject(cmd.Context(), a[0], wire.PatchProjectRequest{Archived: &no})
		},
	}
	rm := &cobra.Command{
		Use:   "rm P",
		Short: "Delete a project and its tasks",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			p, err := c.project(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			if err := c.api.DeleteProject(cmd.Context(), p.ID); err != nil {
				return err
			}
			if !c.json {
				fmt.Fprintf(c.stdout, "Deleted project %s.\n", p.Name)
			}
			return nil
		},
	}
	project.AddCommand(ls, show, add, edit, archive, unarchive, rm)
	return project
}

func (c *cli) patchProject(ctx context.Context, arg string, req wire.PatchProjectRequest) error {
	p, err := c.project(ctx, arg)
	if err != nil {
		return err
	}
	got, err := c.api.PatchProject(ctx, p.ID, req)
	if err != nil {
		return err
	}
	return c.printProject(got)
}

func projectRow(w io.Writer, p wire.Project) {
	state := ""
	if p.ArchivedAt != nil {
		state = "archived"
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", client.ShortID(p.ID), p.Name, p.Color, state)
}

func (c *cli) printProject(p *wire.Project) error {
	return c.emit(p, func(w io.Writer) error {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		projectRow(tw, *p)
		return tw.Flush()
	})
}

// taskFlags are the fields task add and task edit share.
type taskFlags struct {
	project, due, estimate, notes, title, goal, rrule string
	priority, quantity                                int
}

func (f *taskFlags) register(cmd *cobra.Command, withTitle bool) {
	cmd.Flags().StringVar(&f.project, "project", "", "project id, short id, or name; none for no project")
	cmd.Flags().IntVar(&f.priority, "priority", 2, "1 (low) to 4 (high)")
	cmd.Flags().StringVar(&f.due, "due", "", "due date, YYYY-MM-DD; none to clear")
	cmd.Flags().StringVar(&f.estimate, "estimate", "", "estimated effort like 1h30m; none to clear")
	cmd.Flags().StringVar(&f.notes, "notes", "", "notes")
	cmd.Flags().StringVar(&f.goal, "goal", "", "goal id or short id; none to clear")
	cmd.Flags().StringVar(&f.rrule, "rrule", "", "make it recur, such as FREQ=WEEKLY;BYDAY=MO; none to clear")
	cmd.Flags().IntVar(&f.quantity, "quantity", 0, "units of its goal this task covers; 0 with edit clears")
	if withTitle {
		cmd.Flags().StringVar(&f.title, "title", "", "new title")
	}
}

func estimateMinutes(s string) (int, error) {
	d, err := duration("estimate", s)
	if err != nil {
		return 0, err
	}
	if d < time.Minute {
		return 0, usagef("--estimate must be at least a minute")
	}
	return int(d / time.Minute), nil
}

func (c *cli) taskCmd() *cobra.Command {
	task := group("task", "List and change tasks")

	var lsProject, lsGoal, status, dueBefore string
	var templates bool
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List tasks",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			q := wire.TaskQuery{Status: status, Templates: templates}
			if lsGoal != "" {
				g, err := c.goal(ctx, lsGoal)
				if err != nil {
					return err
				}
				q.GoalID = g.ID
			}
			if lsProject != "" {
				p, err := c.projectFlag(ctx, lsProject)
				if err != nil {
					return err
				}
				if p != nil {
					q.ProjectID = *p
				}
			}
			if dueBefore != "" {
				d, err := c.day(ctx, dueBefore)
				if err != nil {
					return err
				}
				q.DueBefore = d
			}
			list, err := c.api.ListTasks(ctx, q)
			if err != nil {
				return err
			}
			names := map[string]string{}
			if !c.json {
				if names, err = c.projectNames(ctx); err != nil {
					return err
				}
			}
			return c.emit(list, func(w io.Writer) error {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, t := range list.Tasks {
					taskRow(tw, t, names)
				}
				return tw.Flush()
			})
		},
	}
	ls.Flags().StringVar(&lsProject, "project", "", "only this project")
	ls.Flags().StringVar(&status, "status", "", "open (default), done, or all")
	ls.Flags().StringVar(&dueBefore, "due-before", "", "only tasks due before this date")
	ls.Flags().StringVar(&lsGoal, "goal", "", "only tasks of this goal")
	ls.Flags().BoolVar(&templates, "templates", false, "list recurring templates too")

	show := &cobra.Command{
		Use:   "show T",
		Short: "Show a task",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			t, err := c.task(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			got, err := c.api.GetTask(cmd.Context(), t.ID)
			if err != nil {
				return err
			}
			return c.printTask(cmd.Context(), got)
		},
	}

	var addFlags taskFlags
	add := &cobra.Command{
		Use:   "add TITLE",
		Short: "Create a task",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			req := wire.CreateTaskRequest{Title: a[0]}
			p, err := c.projectFlag(ctx, addFlags.project)
			if err != nil {
				return err
			}
			req.ProjectID = p
			if cmd.Flags().Changed("priority") {
				req.Priority = &addFlags.priority
			}
			if addFlags.due != "" {
				d, err := c.day(ctx, addFlags.due)
				if err != nil {
					return err
				}
				req.DueDay = &d
			}
			if addFlags.estimate != "" {
				m, err := estimateMinutes(addFlags.estimate)
				if err != nil {
					return err
				}
				req.EstimateMinutes = &m
			}
			if cmd.Flags().Changed("notes") {
				req.Notes = &addFlags.notes
			}
			if req.GoalID, err = c.goalFlag(ctx, addFlags.goal); err != nil {
				return err
			}
			if addFlags.rrule != "" {
				req.RRule = &addFlags.rrule
			}
			if cmd.Flags().Changed("quantity") {
				req.Quantity = &addFlags.quantity
			}
			t, err := c.api.CreateTask(ctx, req)
			if err != nil {
				return err
			}
			return c.printTask(ctx, t)
		},
	}
	addFlags.register(add, false)

	var editFlags taskFlags
	edit := &cobra.Command{
		Use:   "edit T",
		Short: "Change a task",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			t, err := c.task(ctx, a[0])
			if err != nil {
				return err
			}
			var req wire.PatchTaskRequest
			changed := cmd.Flags().Changed
			if changed("project") {
				p, err := c.projectFlag(ctx, editFlags.project)
				if err != nil {
					return err
				}
				req.ProjectID = wire.FromPtr(p)
			}
			if changed("title") {
				req.Title = &editFlags.title
			}
			if changed("notes") {
				req.Notes = &editFlags.notes
			}
			if changed("priority") {
				req.Priority = &editFlags.priority
			}
			if changed("due") {
				req.DueDay = wire.Null[string]()
				if !strings.EqualFold(editFlags.due, "none") {
					d, err := c.day(ctx, editFlags.due)
					if err != nil {
						return err
					}
					req.DueDay = wire.Some(d)
				}
			}
			if changed("estimate") {
				req.EstimateMinutes = wire.Null[int]()
				if !strings.EqualFold(editFlags.estimate, "none") {
					m, err := estimateMinutes(editFlags.estimate)
					if err != nil {
						return err
					}
					req.EstimateMinutes = wire.Some(m)
				}
			}
			if changed("goal") {
				g, err := c.goalFlag(ctx, editFlags.goal)
				if err != nil {
					return err
				}
				req.GoalID = wire.FromPtr(g)
			}
			if changed("rrule") {
				req.RRule = wire.Null[string]()
				if !strings.EqualFold(editFlags.rrule, "none") {
					req.RRule = wire.Some(editFlags.rrule)
				}
			}
			if changed("quantity") {
				req.Quantity = wire.Null[int]()
				if editFlags.quantity != 0 {
					req.Quantity = wire.Some(editFlags.quantity)
				}
			}
			got, err := c.api.PatchTask(ctx, t.ID, req)
			if err != nil {
				return err
			}
			return c.printTask(ctx, got)
		},
	}
	editFlags.register(edit, true)

	byID := func(use, short string, call func(ctx context.Context, id string) (*wire.Task, error)) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  args(1),
			RunE: func(cmd *cobra.Command, a []string) error {
				t, err := c.task(cmd.Context(), a[0])
				if err != nil {
					return err
				}
				got, err := call(cmd.Context(), t.ID)
				if err != nil {
					return err
				}
				return c.printTask(cmd.Context(), got)
			},
		}
	}
	var qty int
	var done *cobra.Command
	done = byID("done T", "Mark a task done", func(ctx context.Context, id string) (*wire.Task, error) {
		var req wire.CompleteTaskRequest
		if done.Flags().Changed("qty") {
			req.QuantityDone = &qty
		}
		return c.api.CompleteTask(ctx, id, req)
	})
	done.Flags().IntVar(&qty, "qty", 0, "units done, for a task with a quantity (default: its quantity)")
	reopen := byID("reopen T", "Reopen a done task", func(ctx context.Context, id string) (*wire.Task, error) {
		return c.api.ReopenTask(ctx, id) // c.api is set only once a command runs
	})
	rm := &cobra.Command{
		Use:   "rm T",
		Short: "Delete a task",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			t, err := c.task(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			if err := c.api.DeleteTask(cmd.Context(), t.ID); err != nil {
				return err
			}
			if !c.json {
				fmt.Fprintf(c.stdout, "Deleted task %s.\n", client.ShortID(t.ID))
			}
			return nil
		},
	}
	task.AddCommand(ls, show, add, edit, done, reopen, rm)
	return task
}

// taskRow is "id  open  P2  due 2026-09-20  Internship  Title  1h 5m".
func taskRow(w io.Writer, t wire.Task, names map[string]string) {
	due := ""
	if t.DueDay != nil {
		due = "due " + *t.DueDay
	}
	project := ""
	if t.ProjectID != nil {
		project = names[*t.ProjectID]
	}
	fmt.Fprintf(w, "%s\t%s\tP%s\t%s\t%s\t%s\t%s", client.ShortID(t.ID), t.Status, strconv.Itoa(t.Priority),
		due, project, t.Title, client.FormatMillis(t.TrackedMs))
	if t.Quantity != nil {
		fmt.Fprintf(w, "\t×%d", *t.Quantity)
	}
	if t.RRule != nil {
		fmt.Fprintf(w, "\trepeats %s", *t.RRule)
	}
	fmt.Fprintln(w)
}

func (c *cli) printTask(ctx context.Context, t *wire.Task) error {
	if c.json {
		return c.emit(t, nil)
	}
	names, err := c.projectNames(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	taskRow(tw, *t, names)
	return tw.Flush()
}
