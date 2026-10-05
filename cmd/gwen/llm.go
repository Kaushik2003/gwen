package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/planner/civil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// goalBreakdownCmd is gwen goal breakdown, added to the goal group.
func (c *cli) goalBreakdownCmd() *cobra.Command {
	var instructions string
	var sessions int
	cmd := &cobra.Command{
		Use:   "breakdown G",
		Short: "Ask the LLM to propose tasks for a goal; accept them with gwen llm accept",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			g, err := c.goal(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			run, err := c.api.GoalBreakdown(cmd.Context(), g.ID,
				wire.BreakdownRequest{Instructions: instructions, Sessions: sessions})
			if err != nil {
				return err
			}
			return c.printRun(run)
		},
	}
	cmd.Flags().StringVar(&instructions, "instructions", "", "what to focus on, in your own words")
	cmd.Flags().IntVar(&sessions, "sessions", 0, "for a quantity goal, how many coming sessions to fill, 1 to 14 (default 7)")
	return cmd
}

// planChatCmd is gwen plan chat, added to the plan group: one message of a
// day plan conversation (docs/07-integrations.md#day-plan).
func (c *cli) planChatCmd() *cobra.Command {
	var day, run string
	cmd := &cobra.Command{
		Use:   "chat MESSAGE",
		Short: "Plan a day by talking it through with the LLM; apply it with gwen llm accept",
		Args:  minArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			d, err := c.day(cmd.Context(), day)
			if err != nil {
				return err
			}
			req := wire.PlanChatRequest{Day: d, Message: strings.Join(a, " ")}
			if run != "" {
				req.RunID = &run
			}
			r, err := c.api.PlanChat(cmd.Context(), req)
			if err != nil {
				return err
			}
			return c.printRun(r)
		},
	}
	cmd.Flags().StringVar(&day, "day", "", "the day to plan (default: today)")
	cmd.Flags().StringVar(&run, "run", "", "the run to go on from, to keep talking")
	return cmd
}

func (c *cli) llmCmd() *cobra.Command {
	group := group("llm", "Show, accept, or reject what the LLM proposed")
	show := &cobra.Command{
		Use:   "show RUN",
		Short: "Show a run",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			run, err := c.api.GetLLMRun(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			return c.printRun(run)
		},
	}
	var pick string
	accept := &cobra.Command{
		Use:   "accept RUN [--pick 0,2,5]",
		Short: "Create the picked tasks of a breakdown, or apply a day plan",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			indexes := []int{}
			for p := range strings.SplitSeq(pick, ",") {
				if pick == "" {
					break
				}
				n, err := strconv.Atoi(strings.TrimSpace(p))
				if err != nil {
					return usagef("--pick takes numbers like 0,2,5, not %q", p)
				}
				indexes = append(indexes, n)
			}
			if pick == "" {
				run, err := c.api.GetLLMRun(cmd.Context(), a[0])
				if err != nil {
					return err
				}
				if run.Kind != wire.RunDayPlan {
					return usagef("--pick is required for a breakdown, such as --pick 0,2")
				}
				var out wire.DayPlanOutput
				if err := json.Unmarshal(run.Output, &out); err != nil {
					return err
				}
				for i := range out.Items {
					indexes = append(indexes, i)
				}
			}
			tasks, err := c.api.AcceptLLMRun(cmd.Context(), a[0], wire.AcceptRunRequest{Indexes: indexes})
			if err != nil {
				return err
			}
			return c.emit(tasks, func(w io.Writer) error {
				names, err := c.projectNames(cmd.Context())
				if err != nil {
					return err
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, t := range tasks.Tasks {
					taskRow(tw, t, names)
				}
				return tw.Flush()
			})
		},
	}
	accept.Flags().StringVar(&pick, "pick", "", "the numbers to take, as gwen llm show lists them (default for a day plan: all)")
	reject := &cobra.Command{
		Use:   "reject RUN",
		Short: "Discard a run's proposal",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			run, err := c.api.RejectLLMRun(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			return c.printRun(run)
		},
	}
	group.AddCommand(show, accept, reject)
	return group
}

func (c *cli) retroCmd() *cobra.Command {
	var week string
	cmd := &cobra.Command{
		Use:   "retro",
		Short: "Write a retrospective of a week (default: last week)",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			start, err := c.retroWeek(cmd.Context(), week)
			if err != nil {
				return err
			}
			run, err := c.api.Retro(cmd.Context(), wire.RetroRequest{WeekStart: start})
			if err != nil {
				return err
			}
			return c.printRun(run)
		},
	}
	cmd.Flags().StringVar(&week, "week", "", "the week's first day, YYYY-MM-DD (default: the Monday of last week)")
	return cmd
}

// retroWeek is the given day, or the Monday of the week before today's.
func (c *cli) retroWeek(ctx context.Context, week string) (string, error) {
	if week != "" {
		return c.day(ctx, week)
	}
	today, err := c.today(ctx)
	if err != nil {
		return "", err
	}
	return civil.MustParse(today).Monday().AddDays(-7).String(), nil
}

func (c *cli) printRun(run *wire.LlmRun) error {
	return c.emit(run, func(w io.Writer) error {
		fmt.Fprintf(w, "Run %s · %s · %s\n", run.ID, run.Kind, run.Status)
		if run.Status == wire.RunFailed {
			var e wire.RunError
			json.Unmarshal(run.Output, &e)
			_, err := fmt.Fprintf(w, "Failed: %s\n", e.Error)
			return err
		}
		switch run.Kind {
		case wire.RunBreakdown:
			var out wire.BreakdownOutput
			if err := json.Unmarshal(run.Output, &out); err != nil {
				return err
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for i, t := range out.Tasks {
				qty := ""
				if t.Quantity != nil {
					qty = fmt.Sprintf("×%d", *t.Quantity)
				}
				fmt.Fprintf(tw, "%d\tP%d\tdue %s\t%s\t%s\t%s\n", i, t.Priority, t.DueDay,
					client.FormatDuration(time.Duration(t.EstimateMinutes)*time.Minute), qty, t.Title)
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if run.Status == wire.RunOK {
				fmt.Fprintf(w, "Accept with: gwen llm accept %s --pick 0,1\n", run.ID)
			}
		case wire.RunDayPlan:
			var out wire.DayPlanOutput
			if err := json.Unmarshal(run.Output, &out); err != nil {
				return err
			}
			if n := len(out.Messages); n > 0 {
				fmt.Fprintf(w, "\n%s\n\n", out.Messages[n-1].Text)
			}
			tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
			for i, b := range out.Items {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", i, client.FormatTime(wire.Time(b.StartAt).In(c.loc)),
					client.FormatDuration(time.Duration(b.PlannedMinutes)*time.Minute), b.Title)
			}
			if len(out.Items) == 0 {
				fmt.Fprintln(tw, "No blocks: nothing planned.")
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			if h := hoursLine(out.Hours); h != "" {
				fmt.Fprintf(w, "Sets the day's hours: %s\n", h)
			}
			if run.Status == wire.RunOK || run.Status == wire.RunAccepted {
				if run.Status == wire.RunOK {
					fmt.Fprintf(w, "Apply with: gwen llm accept %s\n", run.ID)
				}
				fmt.Fprintf(w, "Reply with: gwen plan chat --day %s --run %s MESSAGE\n", run.SubjectID, run.ID)
			}
		case wire.RunRetro:
			var out wire.RetroOutput
			if err := json.Unmarshal(run.Output, &out); err != nil {
				return err
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, out.Markdown)
			if out.GeneratedBy == "rules" {
				fmt.Fprintln(w, "\n(Written from your numbers; the LLM did not answer.)")
			}
		}
		return nil
	})
}
