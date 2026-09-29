package main

import (
	"fmt"
	"io"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

func (c *cli) syncCmd() *cobra.Command {
	sync := group("sync", "Sync with the hub on your Raspberry Pi")
	status := &cobra.Command{
		Use:   "status",
		Short: "Show the sync hub connection",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.SyncStatus(cmd.Context())
			if err != nil {
				return err
			}
			return c.printSync(st)
		},
	}
	now := &cobra.Command{
		Use:   "now",
		Short: "Push and pull now",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.SyncNow(cmd.Context())
			if err != nil {
				return err
			}
			return c.printSync(st)
		},
	}
	sync.AddCommand(status, now)
	return sync
}

func (c *cli) printSync(st *wire.SyncStatus) error {
	return c.emit(st, func(w io.Writer) error {
		when := func(ms *int64) string {
			if ms == nil {
				return "never"
			}
			t := wire.Time(*ms).In(c.loc)
			return client.FormatDay(t) + " " + client.FormatTime(t)
		}
		state := "not configured; run gwen setup sync"
		if st.Configured {
			state = "configured"
		}
		fmt.Fprintf(w, "Sync: %s\nLast push: %s\nLast pull: %s\n", state, when(st.LastPushAt), when(st.LastPullAt))
		if st.LastError != nil {
			fmt.Fprintf(w, "Last error: %s\n", *st.LastError)
		}
		return nil
	})
}
