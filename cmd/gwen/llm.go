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
	cmd := &cobra.Command{
		Use:   "breakdown G",
		Short: "Ask the LLM to propose tasks for a goal; accept them with gwen llm accept",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			g, err := c.goal(cmd.Context(), a[0])
			if err != nil {
				return err
			}
			run, err := c.api.GoalBreakdown(cmd.Context(), g.ID, wire.BreakdownRequest{Instructions: instructions})
			if err != nil {
				return err
			}
			return c.printRun(run)
		},
	}
	cmd.Flags().StringVar(&instructions, "instructions", "", "what to focus on, in your own words")
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
		Use:   "accept RUN --pick 0,2,5",
		Short: "Create the picked tasks of a breakdown",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			if pick == "" {
				return usagef("--pick is required, such as --pick 0,2")
			}
			var indexes []int
			for p := range strings.SplitSeq(pick, ",") {
				n, err := strconv.Atoi(strings.TrimSpace(p))
				if err != nil {
					return usagef("--pick takes numbers like 0,2,5, not %q", p)
				}
				indexes = append(indexes, n)
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
	accept.Flags().StringVar(&pick, "pick", "", "the numbers of the tasks to create, as gwen llm show lists them")
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
