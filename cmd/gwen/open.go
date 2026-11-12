package main

import (
	"errors"
	"os/exec"

	"github.com/kzark/gwen/internal/client"
	"github.com/spf13/cobra"
)

// `gwen open` opens the Gwen window on one of Gwen's screens (plan.md 3.15).
// The launcher entry, the tray, and the panel widget all run it.
func (c *cli) openCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [screen]",
		Short: "Open the Gwen window on a screen: today, gwen, tasks, plan, review, connections, memory, settings",
		Args: func(_ *cobra.Command, a []string) error {
			if len(a) > 1 {
				return usagef("expected at most one screen, got %d", len(a))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, a []string) error {
			screen := ""
			if len(a) == 1 {
				screen = a[0]
			}
			err := c.api.OpenApp(cmd.Context(), screen)
			if !errors.Is(err, client.ErrDaemonNotRunning) {
				return err
			}
			// Without the daemon the window still opens; its Gwen tabs offer to start her.
			bin, lookErr := exec.LookPath("gwen-term")
			if lookErr != nil {
				return errors.New("the Gwen window isn't installed (run: make term), and gwend isn't running")
			}
			return exec.Command(bin).Start()
		},
	}
}
