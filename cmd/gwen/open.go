package main

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

// `gwen open` shows the Gwen window (plan.md 3.15) on a screen, a project, a
// session, or a terminal, starting it when it is closed. The launcher entry,
// the tray, and the panel widget all run it.
func (c *cli) openCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [target]",
		Short: "Open the Gwen window on today, inbox, gwen, plan, board, goals, review, history, memory, settings, or project:<id>",
		Args: func(_ *cobra.Command, a []string) error {
			if len(a) > 1 {
				return usagef("expected at most one target, got %d", len(a))
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, a []string) error {
			args := []string{}
			if len(a) == 1 && a[0] != "" {
				args = append(args, "--open", a[0])
			}
			bin, err := exec.LookPath("gwen-ui")
			if err != nil {
				home, _ := os.UserHomeDir()
				bin = filepath.Join(home, ".local", "bin", "gwen-ui")
			}
			// gwen-ui keeps one window: a second start hands it the target and exits.
			p := exec.Command(bin, args...)
			if err := p.Start(); err != nil {
				return err
			}
			return p.Process.Release()
		},
	}
}
