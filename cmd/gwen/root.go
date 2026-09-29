package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/timeengine"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// cli is the CLI's dependencies; main wires the real ones, tests fakes.
type cli struct {
	newAPI func(socket string) client.API
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	clk    clock.Clock
	loc    *time.Location
	// open shows a URL in the default browser.
	open func(url string) error

	json   bool
	socket string
	api    client.API
	cfg    *config.Config
}

func newRootCmd(c *cli) *cobra.Command {
	root := &cobra.Command{
		Use:           "gwen",
		Short:         "Track your working day and plan your study goals",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if c.socket == "" {
				s, err := config.DefaultSocketPath()
				if err != nil {
					return err
				}
				c.socket = s
			}
			c.api = c.newAPI(c.socket)
			return nil
		},
	}
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &usageError{err.Error()} })
	root.PersistentFlags().BoolVar(&c.json, "json", false, "print the raw wire JSON instead of human output")
	root.PersistentFlags().StringVar(&c.socket, "socket", "", "daemon socket (default $XDG_RUNTIME_DIR/gwen/gwend.sock)")
	root.AddCommand(
		c.statusCmd(), c.healthCmd(),
		c.inCmd(), c.outCmd(), c.breakCmd(), c.backCmd(), c.switchCmd(), c.snoozeCmd(),
		c.todayCmd(), c.logCmd(), c.dayCmd(), c.segCmd(),
		c.projectCmd(), c.taskCmd(),
		c.goalCmd(), c.commitCmd(), c.planCmd(), c.briefCmd(),
		c.calCmd(),
		c.statsCmd(), c.configCmd(), c.notifyCmd(), c.watchCmd(),
		newSetupCmd(),
	)
	return root
}

// args validates the positional argument count as a usage error.
func args(n int) cobra.PositionalArgs {
	return func(_ *cobra.Command, a []string) error {
		if len(a) != n {
			return usagef("expected %d argument(s), got %d", n, len(a))
		}
		return nil
	}
}

func maxArgs(n int) cobra.PositionalArgs {
	return func(_ *cobra.Command, a []string) error {
		if len(a) > n {
			return usagef("expected at most %d argument(s), got %d", n, len(a))
		}
		return nil
	}
}

// emit prints v as wire JSON with --json, or runs the human renderer.
func (c *cli) emit(v any, human func(w io.Writer) error) error {
	if c.json {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(c.stdout, "%s\n", b)
		return err
	}
	return human(c.stdout)
}

// config is the daemon's configuration, fetched once per command.
func (c *cli) config(ctx context.Context) (config.Config, error) {
	if c.cfg != nil {
		return *c.cfg, nil
	}
	w, err := c.api.GetConfig(ctx)
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.FromWire(*w)
	if err != nil {
		return config.Config{}, err
	}
	c.cfg = &cfg
	return cfg, nil
}

// today is the day now belongs to under tracking.day_rollover.
func (c *cli) today(ctx context.Context) (string, error) {
	cfg, err := c.config(ctx)
	if err != nil {
		return "", err
	}
	return timeengine.ConfigFrom(cfg, c.loc).DayOf(c.clk.Now()), nil
}

// day validates a YYYY-MM-DD argument, or returns today for "".
func (c *cli) day(ctx context.Context, s string) (string, error) {
	if s == "" {
		return c.today(ctx)
	}
	t, err := time.Parse(model.DayLayout, s)
	if err != nil || t.Format(model.DayLayout) != s {
		return "", usagef("%q is not a date like 2026-09-15", s)
	}
	return s, nil
}

// addDays offsets a YYYY-MM-DD date by n calendar days.
func addDays(day string, n int) string {
	t, _ := time.Parse(model.DayLayout, day)
	return time.Date(t.Year(), t.Month(), t.Day()+n, 0, 0, 0, 0, time.UTC).Format(model.DayLayout)
}

// instant turns "HH:MM" on a work day into an instant in the local zone. A
// time before the day rollover belongs to the next calendar date, as it does
// for the work day itself.
func (c *cli) instant(ctx context.Context, day, hhmm string) (time.Time, error) {
	tod, err := config.ParseTimeOfDay(hhmm)
	if err != nil {
		return time.Time{}, usagef("%q is not a time like 09:30", hhmm)
	}
	cfg, err := c.config(ctx)
	if err != nil {
		return time.Time{}, err
	}
	d, _ := time.Parse(model.DayLayout, day)
	date := d.Day()
	if tod < cfg.Tracking.DayRollover {
		date++
	}
	return tod.On(d.Year(), d.Month(), date, c.loc), nil
}

// duration parses a Go duration flag such as 7h30m.
func duration(flag, s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, usagef("--%s %q is not a duration like 7h30m", flag, s)
	}
	return d, nil
}

// project resolves an id, a unique suffix of at least 8 characters, or the
// exact name of a live, non-archived project, ignoring case.
func (c *cli) project(ctx context.Context, s string) (wire.Project, error) {
	all, err := c.api.ListProjects(ctx, wire.ArchivedAll)
	if err != nil {
		return wire.Project{}, err
	}
	ids := make([]string, len(all.Projects))
	for i, p := range all.Projects {
		ids[i] = p.ID
	}
	id, err := client.ResolveID(s, ids)
	if err == nil {
		for _, p := range all.Projects {
			if p.ID == id {
				return p, nil
			}
		}
	}
	if errors.Is(err, client.ErrAmbiguousID) {
		return wire.Project{}, err
	}
	for _, p := range all.Projects {
		if p.ArchivedAt == nil && strings.EqualFold(p.Name, s) {
			return p, nil
		}
	}
	return wire.Project{}, fmt.Errorf("no project matches %q", s)
}

// projectFlag resolves --project: "none" is unassigned.
func (c *cli) projectFlag(ctx context.Context, s string) (*string, error) {
	if s == "" || strings.EqualFold(s, "none") {
		return nil, nil
	}
	p, err := c.project(ctx, s)
	if err != nil {
		return nil, err
	}
	return &p.ID, nil
}

// task resolves a task id or a unique suffix of at least 8 characters.
func (c *cli) task(ctx context.Context, s string) (wire.Task, error) {
	all, err := c.api.ListTasks(ctx, wire.TaskQuery{Status: "all"})
	if err != nil {
		return wire.Task{}, err
	}
	ids := make([]string, len(all.Tasks))
	for i, t := range all.Tasks {
		ids[i] = t.ID
	}
	id, err := client.ResolveID(s, ids)
	if err != nil {
		if errors.Is(err, client.ErrUnknownID) {
			return wire.Task{}, fmt.Errorf("no task matches %q", s)
		}
		return wire.Task{}, err
	}
	for _, t := range all.Tasks {
		if t.ID == id {
			return t, nil
		}
	}
	return wire.Task{}, fmt.Errorf("no task matches %q", s)
}

func (c *cli) taskFlag(ctx context.Context, s string) (*string, error) {
	if s == "" || strings.EqualFold(s, "none") {
		return nil, nil
	}
	t, err := c.task(ctx, s)
	if err != nil {
		return nil, err
	}
	return &t.ID, nil
}

// segment resolves a segment id or suffix among the segments of day.
func (c *cli) segment(ctx context.Context, day, s string) (wire.Segment, error) {
	d, err := c.api.GetDay(ctx, day)
	if err != nil {
		return wire.Segment{}, err
	}
	ids := make([]string, len(d.Segments))
	for i, g := range d.Segments {
		ids[i] = g.ID
	}
	id, err := client.ResolveID(s, ids)
	if err != nil {
		if errors.Is(err, client.ErrUnknownID) {
			return wire.Segment{}, fmt.Errorf("no segment on %s matches %q", day, s)
		}
		return wire.Segment{}, err
	}
	for _, g := range d.Segments {
		if g.ID == id {
			return g, nil
		}
	}
	return wire.Segment{}, fmt.Errorf("no segment on %s matches %q", day, s)
}

// projectNames maps project ids to names for rendering, deleted ones aside.
func (c *cli) projectNames(ctx context.Context) (map[string]string, error) {
	all, err := c.api.ListProjects(ctx, wire.ArchivedAll)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, p := range all.Projects {
		names[p.ID] = p.Name
	}
	return names, nil
}
