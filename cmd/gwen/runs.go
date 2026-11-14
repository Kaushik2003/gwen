package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// runCmd is `gwen run`: the runs Gwen makes (plan.md 3.7, W19). Approvals
// are not decided here: only a click in the window decides one (section 7).
func (c *cli) runCmd() *cobra.Command {
	run := group("run", "List, show, cancel, and resume Gwen's runs")

	var limit int
	ls := &cobra.Command{
		Use:   "ls",
		Short: "List recent runs",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.ListRuns(cmd.Context(), nil, limit)
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if len(list.Runs) == 0 {
					_, err := fmt.Fprintln(w, "No runs yet.")
					return err
				}
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, r := range list.Runs {
					fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID[:8], r.Status, r.Kind, r.Title, r.Reason)
				}
				return tw.Flush()
			})
		},
	}
	ls.Flags().IntVar(&limit, "limit", 20, "how many runs, newest first")

	show := &cobra.Command{
		Use:   "show ID",
		Short: "Show a run with its steps and log",
		Args:  args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			id, err := c.runID(cmd, a[0])
			if err != nil {
				return err
			}
			d, err := c.api.GetRun(cmd.Context(), id)
			if err != nil {
				return err
			}
			return c.emit(d, func(w io.Writer) error {
				fmt.Fprintf(w, "%s  (%s, %s)\n", d.Run.Title, d.Run.Kind, d.Run.Status)
				if d.Run.Reason != "" {
					fmt.Fprintf(w, "  %s\n", d.Run.Reason)
				}
				for _, s := range d.Steps {
					line := fmt.Sprintf("  %d. [%s] %s", s.Position+1, s.Status, s.Title)
					if s.Error != "" {
						line += ": " + s.Error
					}
					fmt.Fprintln(w, line)
				}
				for _, ev := range d.Log {
					fmt.Fprintf(w, "    %s %s\n", time.UnixMilli(ev.At).Format("15:04:05"), ev.Kind)
				}
				return nil
			})
		},
	}

	act := func(use, short string, do func(cmd *cobra.Command, id string) (*wire.Run, error)) *cobra.Command {
		return &cobra.Command{Use: use + " ID", Short: short, Args: args(1),
			RunE: func(cmd *cobra.Command, a []string) error {
				id, err := c.runID(cmd, a[0])
				if err != nil {
					return err
				}
				r, err := do(cmd, id)
				if err != nil {
					return err
				}
				return c.emit(r, func(w io.Writer) error {
					_, err := fmt.Fprintf(w, "%s is %s.\n", r.Title, r.Status)
					return err
				})
			}}
	}
	cancel := act("cancel", "Cancel a run", func(cmd *cobra.Command, id string) (*wire.Run, error) { return c.api.CancelRun(cmd.Context(), id) })
	resume := act("resume", "Resume a run that a restart interrupted", func(cmd *cobra.Command, id string) (*wire.Run, error) {
		return c.api.ResumeRun(cmd.Context(), id)
	})

	approvals := &cobra.Command{
		Use:   "approvals",
		Short: "List approvals waiting for you (decide them in the window)",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.ListApprovals(cmd.Context(), "pending")
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if len(list.Approvals) == 0 {
					_, err := fmt.Fprintln(w, "Nothing is waiting for you.")
					return err
				}
				for _, a := range list.Approvals {
					fmt.Fprintf(w, "%s  %s (%s)\n", a.ID[:8], a.Summary, a.Risk)
				}
				_, err := fmt.Fprintln(w, "Approve or reject them in Gwen › Settings › What Gwen can do.")
				return err
			})
		},
	}

	grants := &cobra.Command{
		Use:   "grants",
		Short: "List what you told Gwen to remember",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.ListGrants(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, g := range list.Grants {
					fmt.Fprintf(tw, "%s\t%s\t%s %s\t%s\n", g.ID[:8], g.Decision, g.Capability, g.Pattern, g.Scope)
				}
				return tw.Flush()
			})
		},
	}

	run.AddCommand(ls, show, cancel, resume, approvals, grants)
	return run
}

// runID finds a run by its id or the start of it, among the recent runs.
func (c *cli) runID(cmd *cobra.Command, prefix string) (string, error) {
	if len(prefix) == 36 {
		return prefix, nil
	}
	list, err := c.api.ListRuns(cmd.Context(), nil, 200)
	if err != nil {
		return "", err
	}
	found := ""
	for _, r := range list.Runs {
		if len(r.ID) >= len(prefix) && r.ID[:len(prefix)] == prefix {
			if found != "" {
				return "", usagef("%q matches more than one run; give more of its id", prefix)
			}
			found = r.ID
		}
	}
	if found == "" {
		return "", usagef("no recent run starts with %q", prefix)
	}
	return found, nil
}
