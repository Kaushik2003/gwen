package main

import (
	"fmt"
	"io"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

func (c *cli) calCmd() *cobra.Command {
	cal := group("cal", "Sync the plan with Google Calendar")
	status := &cobra.Command{
		Use:   "status",
		Short: "Show the calendar connection",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.CalendarStatus(cmd.Context())
			if err != nil {
				return err
			}
			return c.printCalendar(st)
		},
	}
	connect := &cobra.Command{
		Use:   "connect",
		Short: "Sign in to Google in the browser to connect the calendar",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			auth, err := c.api.CalendarAuthStart(cmd.Context())
			if err != nil {
				return err
			}
			if c.json {
				return c.emit(auth, nil)
			}
			fmt.Fprintln(c.stdout, "Sign in to Google in your browser. If it does not open, visit:")
			fmt.Fprintln(c.stdout, "  "+auth.AuthURL)
			if err := c.open(auth.AuthURL); err != nil {
				fmt.Fprintf(c.stderr, "gwen: could not open a browser: %v\n", err)
			}
			fmt.Fprintln(c.stdout, "The link works for 5 minutes. Check with gwen cal status afterwards.")
			return nil
		},
	}
	sync := &cobra.Command{
		Use:   "sync",
		Short: "Sync with Google Calendar now",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.CalendarSync(cmd.Context())
			if err != nil {
				return err
			}
			return c.printCalendar(st)
		},
	}
	cal.AddCommand(status, connect, sync)
	return cal
}

func (c *cli) printCalendar(st *wire.CalendarStatus) error {
	return c.emit(st, func(w io.Writer) error {
		state := "not connected; run gwen cal connect"
		if st.Connected {
			state = "connected"
		}
		if !st.Enabled {
			state = "turned off"
		}
		fmt.Fprintf(w, "Calendar: %s\n", state)
		if st.CalendarID != nil {
			fmt.Fprintf(w, "Gwen calendar: %s\n", *st.CalendarID)
		}
		last := "never"
		if st.LastSyncAt != nil {
			last = client.FormatDay(wire.Time(*st.LastSyncAt).In(c.loc)) + " " +
				client.FormatTime(wire.Time(*st.LastSyncAt).In(c.loc))
		}
		fmt.Fprintf(w, "Last sync: %s\n", last)
		if st.LastError != nil {
			fmt.Fprintf(w, "Last error: %s\n", *st.LastError)
		}
		return nil
	})
}
