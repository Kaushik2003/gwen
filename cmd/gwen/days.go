package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

func (c *cli) todayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "Show today's segments and totals",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			day, err := c.today(cmd.Context())
			if err != nil {
				return err
			}
			return c.showDay(cmd.Context(), day)
		},
	}
}

func (c *cli) showDay(ctx context.Context, day string) error {
	d, err := c.api.GetDay(ctx, day)
	if err != nil {
		return err
	}
	names, err := c.projectNames(ctx)
	if err != nil {
		return err
	}
	return c.emit(d, func(w io.Writer) error {
		fmt.Fprintf(w, "%s · %s\n", d.WorkDay.Day, totalsLine(d.Summary))
		if d.WorkDay.Note != "" {
			fmt.Fprintf(w, "Note: %s\n", d.WorkDay.Note)
		}
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, g := range d.Segments {
			c.segmentRow(tw, g, names)
		}
		tw.Flush()
		for _, p := range d.Summary.ByProject {
			fmt.Fprintf(tw, "%s\t%s\n", p.Name, client.FormatMillis(p.WorkedMs))
		}
		return tw.Flush()
	})
}

// segmentRow is "id  09:00–10:30  work  Internship  task 9f3a1c2e  1h 30m".
func (c *cli) segmentRow(w io.Writer, g wire.Segment, names map[string]string) {
	end, length := "now", ""
	if g.EndedAt != nil {
		end = client.FormatTime(wire.Time(*g.EndedAt).In(c.loc))
		length = client.FormatMillis(*g.EndedAt - g.StartedAt)
	}
	what := g.Kind
	if g.Kind == "work" {
		what += "\t" + attributionLabel(g.ProjectID, g.TaskID, names)
	} else {
		what += "\t"
	}
	flag := ""
	if g.Truncated {
		flag = " (truncated)"
	}
	fmt.Fprintf(w, "%s\t%s–%s\t%s\t%s\t%s%s\n", client.ShortID(g.ID),
		client.FormatTime(wire.Time(g.StartedAt).In(c.loc)), end, what, g.Source, length, flag)
}

func (c *cli) logCmd() *cobra.Command {
	var from, to string
	cmd := &cobra.Command{
		Use:   "log",
		Short: "List worked time per day (default: the last 7 days)",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			f, t, err := c.rangeFlags(ctx, from, to, 7)
			if err != nil {
				return err
			}
			list, err := c.api.ListDays(ctx, f, t)
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if len(list.Days) == 0 {
					_, err := fmt.Fprintf(w, "Nothing tracked from %s to %s.\n", f, t)
					return err
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, d := range list.Days {
					met := ""
					if d.TargetMet {
						met = "✓"
					}
					fmt.Fprintf(tw, "%s\t%s worked\t%s break\t%s\t%s\n", d.Day, client.FormatMillis(d.WorkedMs),
						client.FormatMillis(d.BreakMs), percent(d), met)
				}
				return tw.Flush()
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "first day, YYYY-MM-DD")
	cmd.Flags().StringVar(&to, "to", "", "last day, YYYY-MM-DD (default today)")
	return cmd
}

func percent(d wire.DaySummary) string {
	if d.TargetSeconds == 0 {
		return "-"
	}
	return fmt.Sprintf("%d%%", d.WorkedMs/int64(d.TargetSeconds*10))
}

// rangeFlags defaults to the last n days ending today.
func (c *cli) rangeFlags(ctx context.Context, from, to string, n int) (string, string, error) {
	t, err := c.day(ctx, to)
	if err != nil {
		return "", "", err
	}
	f := addDays(t, -(n - 1))
	if from != "" {
		if f, err = c.day(ctx, from); err != nil {
			return "", "", err
		}
	}
	return f, t, nil
}

func (c *cli) dayCmd() *cobra.Command {
	day := &cobra.Command{Use: "day", Short: "Show or change one work day", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	show := &cobra.Command{
		Use:   "show DAY",
		Short: "Show a day's segments and totals",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			d, err := c.day(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			return c.showDay(cmd.Context(), d)
		},
	}
	var target, note string
	set := &cobra.Command{
		Use:   "set DAY",
		Short: "Change a day's target or note",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			d, err := c.day(ctx, a[0])
			if err != nil {
				return err
			}
			var req wire.PatchDayRequest
			if cmd.Flags().Changed("target") {
				dur, err := time.ParseDuration(target)
				if err != nil || dur < 0 {
					return usagef("--target %q is not a duration like 7h30m", target)
				}
				secs := int(dur / time.Second)
				req.TargetSeconds = &secs
			}
			if cmd.Flags().Changed("note") {
				req.Note = &note
			}
			if req.TargetSeconds == nil && req.Note == nil {
				return usagef("give --target or --note")
			}
			wd, err := c.api.PatchDay(ctx, d, req)
			if err != nil {
				return err
			}
			return c.emit(wd, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "%s · target %s\n", wd.Day, client.FormatDuration(time.Duration(wd.TargetSeconds)*time.Second))
				return err
			})
		},
	}
	set.Flags().StringVar(&target, "target", "", "the day's target, like 7h30m")
	set.Flags().StringVar(&note, "note", "", "a note for the day")
	day.AddCommand(show, set)
	return day
}

func (c *cli) segCmd() *cobra.Command {
	seg := &cobra.Command{Use: "seg", Short: "Add, fix, split, or remove segments", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}

	var day, kind, start, end, project, task string
	add := &cobra.Command{
		Use:   "add",
		Short: "Add a segment you forgot to track",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			d, err := c.day(ctx, day)
			if err != nil {
				return err
			}
			if kind == "" || start == "" || end == "" {
				return usagef("--kind, --start, and --end are required")
			}
			s, err := c.instant(ctx, d, start)
			if err != nil {
				return err
			}
			e, err := c.instant(ctx, d, end)
			if err != nil {
				return err
			}
			p, t, err := c.attribution(ctx, project, task)
			if err != nil {
				return err
			}
			endMs := wire.Millis(e)
			g, err := c.api.CreateSegment(ctx, wire.CreateSegmentRequest{Day: d, Kind: kind, ProjectID: p, TaskID: t,
				StartedAt: wire.Millis(s), EndedAt: &endMs})
			if err != nil {
				return err
			}
			return c.printSegments(ctx, g)
		},
	}
	add.Flags().StringVar(&day, "day", "", "work day, YYYY-MM-DD (default today)")
	add.Flags().StringVar(&kind, "kind", "", "work, break_manual, or break_auto")
	add.Flags().StringVar(&start, "start", "", "start time, HH:MM")
	add.Flags().StringVar(&end, "end", "", "end time, HH:MM")
	add.Flags().StringVar(&project, "project", "", "project id, short id, or name")
	add.Flags().StringVar(&task, "task", "", "task id or short id")

	var eDay, eKind, eStart, eEnd, eProject, eTask string
	edit := &cobra.Command{
		Use:   "edit ID",
		Short: "Change a segment",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			d, err := c.day(ctx, eDay)
			if err != nil {
				return err
			}
			g, err := c.segment(ctx, d, a[0])
			if err != nil {
				return err
			}
			var req wire.PatchSegmentRequest
			changed := cmd.Flags().Changed
			if changed("kind") {
				req.Kind = &eKind
			}
			if changed("start") {
				s, err := c.instant(ctx, d, eStart)
				if err != nil {
					return err
				}
				ms := wire.Millis(s)
				req.StartedAt = &ms
			}
			if changed("end") {
				e, err := c.instant(ctx, d, eEnd)
				if err != nil {
					return err
				}
				ms := wire.Millis(e)
				req.EndedAt = &ms
			}
			if changed("project") {
				p, err := c.projectFlag(ctx, eProject)
				if err != nil {
					return err
				}
				req.ProjectID = wire.FromPtr(p)
			}
			if changed("task") {
				t, err := c.taskFlag(ctx, eTask)
				if err != nil {
					return err
				}
				req.TaskID = wire.FromPtr(t)
			}
			out, err := c.api.PatchSegment(ctx, g.ID, req)
			if err != nil {
				return err
			}
			return c.printSegments(ctx, out)
		},
	}
	edit.Flags().StringVar(&eDay, "day", "", "work day the segment is on (default today)")
	edit.Flags().StringVar(&eKind, "kind", "", "work, break_manual, or break_auto")
	edit.Flags().StringVar(&eStart, "start", "", "start time, HH:MM")
	edit.Flags().StringVar(&eEnd, "end", "", "end time, HH:MM")
	edit.Flags().StringVar(&eProject, "project", "", "project id, short id, or name; none to clear")
	edit.Flags().StringVar(&eTask, "task", "", "task id or short id; none to clear")

	var sDay string
	split := &cobra.Command{
		Use:   "split ID HH:MM",
		Short: "Split a segment in two",
		Args:  args(2),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			d, err := c.day(ctx, sDay)
			if err != nil {
				return err
			}
			g, err := c.segment(ctx, d, a[0])
			if err != nil {
				return err
			}
			at, err := c.instant(ctx, d, a[1])
			if err != nil {
				return err
			}
			halves, err := c.api.SplitSegment(ctx, g.ID, wire.SplitSegmentRequest{At: wire.Millis(at)})
			if err != nil {
				return err
			}
			if c.json {
				return c.emit(halves, nil)
			}
			return c.printSegments(ctx, &halves.Segments[0], &halves.Segments[1])
		},
	}
	split.Flags().StringVar(&sDay, "day", "", "work day the segment is on (default today)")

	var rDay string
	rm := &cobra.Command{
		Use:   "rm ID",
		Short: "Remove a segment",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			d, err := c.day(ctx, rDay)
			if err != nil {
				return err
			}
			g, err := c.segment(ctx, d, a[0])
			if err != nil {
				return err
			}
			if err := c.api.DeleteSegment(ctx, g.ID); err != nil {
				return err
			}
			if !c.json {
				fmt.Fprintf(c.stdout, "Removed segment %s.\n", client.ShortID(g.ID))
			}
			return nil
		},
	}
	rm.Flags().StringVar(&rDay, "day", "", "work day the segment is on (default today)")

	seg.AddCommand(add, edit, split, rm)
	return seg
}

// printSegments renders segments as rows, or one as JSON.
func (c *cli) printSegments(ctx context.Context, segs ...*wire.Segment) error {
	if c.json {
		return c.emit(segs[0], nil)
	}
	names, err := c.projectNames(ctx)
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(c.stdout, 0, 0, 2, ' ', 0)
	for _, g := range segs {
		c.segmentRow(tw, *g, names)
	}
	return tw.Flush()
}
