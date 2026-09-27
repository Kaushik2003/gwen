package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

func (c *cli) statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show what is being tracked now",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.Status(cmd.Context())
			if err != nil {
				return err
			}
			return c.printStatus(cmd.Context(), st)
		},
	}
}

func (c *cli) healthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check that the daemon answers",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			h, err := c.api.Health(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(h, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "gwend %s · schema %d · pid %d\n", h.Version, h.SchemaVersion, h.PID)
				return err
			})
		},
	}
}

func (c *cli) inCmd() *cobra.Command {
	var project, task string
	cmd := &cobra.Command{
		Use:   "in",
		Short: "Clock in",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p, t, err := c.attribution(ctx, project, task)
			if err != nil {
				return err
			}
			st, err := c.api.ClockIn(ctx, wire.ClockInRequest{ProjectID: p, TaskID: t})
			if err != nil {
				return err
			}
			return c.printStatus(ctx, st)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id, short id, or name; none for unassigned")
	cmd.Flags().StringVar(&task, "task", "", "task id or short id")
	return cmd
}

func (c *cli) switchCmd() *cobra.Command {
	var project, task string
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Change what the time is attributed to",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p, t, err := c.attribution(ctx, project, task)
			if err != nil {
				return err
			}
			st, err := c.api.Switch(ctx, wire.SwitchRequest{ProjectID: p, TaskID: t})
			if err != nil {
				return err
			}
			return c.printStatus(ctx, st)
		},
	}
	cmd.Flags().StringVar(&project, "project", "", "project id, short id, or name; none for unassigned")
	cmd.Flags().StringVar(&task, "task", "", "task id or short id")
	return cmd
}

func (c *cli) attribution(ctx context.Context, project, task string) (*string, *string, error) {
	p, err := c.projectFlag(ctx, project)
	if err != nil {
		return nil, nil, err
	}
	t, err := c.taskFlag(ctx, task)
	if err != nil {
		return nil, nil, err
	}
	return p, t, nil
}

// simple builds a command that takes no arguments and prints the status.
func (c *cli) simple(use, short string, call func(ctx context.Context) (*wire.Status, error)) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := call(cmd.Context())
			if err != nil {
				return err
			}
			return c.printStatus(cmd.Context(), st)
		},
	}
}

func (c *cli) outCmd() *cobra.Command {
	return c.simple("out", "Clock out", func(ctx context.Context) (*wire.Status, error) { return c.api.ClockOut(ctx) })
}

func (c *cli) breakCmd() *cobra.Command {
	return c.simple("break", "Start a break", func(ctx context.Context) (*wire.Status, error) { return c.api.BreakStart(ctx) })
}

func (c *cli) backCmd() *cobra.Command {
	return c.simple("back", "End the break and get back to work", func(ctx context.Context) (*wire.Status, error) { return c.api.BreakEnd(ctx) })
}

func (c *cli) snoozeCmd() *cobra.Command {
	return c.simple("snooze", "Hold nudges for a while", func(ctx context.Context) (*wire.Status, error) { return c.api.Snooze(ctx) })
}

// printStatus renders a Status, the reference for human formatting:
//
//	Working · 3h 12m on Internship (task 9f3a1c2e)
//	Today   5h 47m worked · 38m break · target 8h (72%)
func (c *cli) printStatus(ctx context.Context, st *wire.Status) error {
	if c.json {
		return c.emit(st, nil)
	}
	names := map[string]string{}
	if st.ProjectID != nil {
		var err error
		if names, err = c.projectNames(ctx); err != nil {
			return err
		}
	}
	w := c.stdout
	now := wire.Time(st.ServerNowAt)
	elapsed := func() string {
		if st.OpenSegment == nil {
			return "0s"
		}
		return client.FormatDuration(now.Sub(wire.Time(st.OpenSegment.StartedAt)))
	}
	switch st.State {
	case wire.StateOff:
		fmt.Fprintln(w, "Not clocked in.")
	case wire.StateWorking:
		fmt.Fprintf(w, "Working · %s on %s\n", elapsed(), attributionLabel(st.ProjectID, st.TaskID, names))
	case wire.StateIdlePending:
		fmt.Fprintf(w, "Idle since %s (still counting)\n", client.FormatTime(wire.Time(*st.IdleSinceAt).In(c.loc)))
	case wire.StateBreakAuto:
		fmt.Fprintf(w, "On break · %s (automatic)\n", elapsed())
	case wire.StateBreakManual:
		fmt.Fprintf(w, "On break · %s\n", elapsed())
	}
	if st.Today != nil {
		fmt.Fprintf(w, "Today   %s\n", totalsLine(*st.Today))
	}
	if st.SnoozedUntilAt != nil && *st.SnoozedUntilAt > st.ServerNowAt {
		fmt.Fprintf(w, "Nudges snoozed until %s\n", client.FormatTime(wire.Time(*st.SnoozedUntilAt).In(c.loc)))
	}
	for _, warning := range st.Warnings {
		fmt.Fprintf(w, "Warning: %s\n", warning)
	}
	return nil
}

func attributionLabel(project, task *string, names map[string]string) string {
	label := wire.UnassignedName
	if project != nil {
		label = names[*project]
		if label == "" {
			label = client.ShortID(*project)
		}
	}
	if task != nil {
		label += " (task " + client.ShortID(*task) + ")"
	}
	return label
}

// totalsLine is "5h 47m worked · 38m break · target 8h (72%)".
func totalsLine(d wire.DaySummary) string {
	s := fmt.Sprintf("%s worked · %s break · target %s", client.FormatMillis(d.WorkedMs),
		client.FormatMillis(d.BreakMs), client.FormatDuration(time.Duration(d.TargetSeconds)*time.Second))
	if d.TargetSeconds > 0 {
		s += fmt.Sprintf(" (%d%%)", d.WorkedMs/int64(d.TargetSeconds*10))
	}
	return s
}

func (c *cli) watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Print each event from the daemon as it happens",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := client.Follower{
				API:   c.api,
				Clock: c.clk,
				OnEvent: func(ev wire.Event) {
					if c.json {
						b, _ := json.Marshal(ev)
						fmt.Fprintf(c.stdout, "%s\n", b)
						return
					}
					fmt.Fprintf(c.stdout, "%s %s %s\n", c.clk.Now().In(c.loc).Format("15:04:05"), ev.Name, ev.Data)
				},
				OnDisconnect: func(err error) {
					fmt.Fprintf(c.stderr, "gwen: disconnected (%v); reconnecting\n", err)
				},
			}.Run(cmd.Context())
			if errors.Is(err, context.Canceled) {
				return nil // interrupted: the normal way to stop watching
			}
			return err
		},
	}
}
