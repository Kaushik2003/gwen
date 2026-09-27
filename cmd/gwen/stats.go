package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

func (c *cli) statsCmd() *cobra.Command {
	var from, to string
	stats := &cobra.Command{
		Use:   "stats",
		Short: "Totals and streaks (default: the last 30 days)",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			f, t, err := c.rangeFlags(ctx, from, to, 30)
			if err != nil {
				return err
			}
			s, err := c.api.StatsSummary(ctx, f, t)
			if err != nil {
				return err
			}
			return c.emit(s, func(w io.Writer) error {
				fmt.Fprintf(w, "%s to %s\n", s.From, s.To)
				fmt.Fprintf(w, "Worked %s over %d days (average %s) · %s on breaks\n", client.FormatMillis(s.WorkedMs),
					s.DaysTracked, client.FormatMillis(s.AvgWorkedMs), client.FormatMillis(s.BreakMs))
				fmt.Fprintf(w, "Target met on %d days · streak %d (longest %d)\n", s.DaysTargetMet, s.CurrentStreak, s.LongestStreak)
				tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
				for _, p := range s.ByProject {
					fmt.Fprintf(tw, "  %s\t%s\n", p.Name, client.FormatMillis(p.WorkedMs))
				}
				return tw.Flush()
			})
		},
	}
	stats.Flags().StringVar(&from, "from", "", "first day, YYYY-MM-DD")
	stats.Flags().StringVar(&to, "to", "", "last day, YYYY-MM-DD (default today)")

	var year int
	heatmap := &cobra.Command{
		Use:   "heatmap",
		Short: "A year of worked time, one character per day",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if !cmd.Flags().Changed("year") {
				year = c.clk.Now().In(c.loc).Year()
			}
			h, err := c.api.StatsHeatmap(ctx, year)
			if err != nil {
				return err
			}
			if c.json {
				return c.emit(h, nil)
			}
			cfg, err := c.config(ctx)
			if err != nil {
				return err
			}
			_, err = io.WriteString(c.stdout, renderHeatmap(*h, cfg.Tracking.DailyTarget))
			return err
		},
	}
	heatmap.Flags().IntVar(&year, "year", 0, "the year (default this year)")
	stats.AddCommand(heatmap)
	return stats
}

// renderHeatmap prints one row per weekday with one character per week: ·
// for nothing, then ░ ▒ ▓ █ for up to 25 %, 50 %, 75 %, and above 75 % of the
// daily target.
func renderHeatmap(h wire.Heatmap, target time.Duration) string {
	worked := map[string]int64{}
	for _, d := range h.Days {
		worked[d.Day] = d.WorkedMs
	}
	jan1 := time.Date(h.Year, time.January, 1, 0, 0, 0, 0, time.UTC)
	start := jan1.AddDate(0, 0, -((int(jan1.Weekday()) + 6) % 7)) // the Monday on or before
	end := time.Date(h.Year, time.December, 31, 0, 0, 0, 0, time.UTC)
	weeks := int(end.Sub(start).Hours()/24)/7 + 1
	var b strings.Builder
	fmt.Fprintf(&b, "%d\n", h.Year)
	for row, label := range []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"} {
		b.WriteString(label + " ")
		for week := range weeks {
			d := start.AddDate(0, 0, week*7+row)
			if d.Year() != h.Year {
				b.WriteString(" ")
				continue
			}
			b.WriteString(heatCell(worked[d.Format(model.DayLayout)], target))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func heatCell(ms int64, target time.Duration) string {
	if ms <= 0 {
		return "·"
	}
	if target <= 0 {
		return "█"
	}
	switch r := float64(ms) / float64(target.Milliseconds()); {
	case r <= 0.25:
		return "░"
	case r <= 0.5:
		return "▒"
	case r <= 0.75:
		return "▓"
	default:
		return "█"
	}
}

func (c *cli) configCmd() *cobra.Command {
	cfgCmd := group("config", "Show or change the configuration")
	get := &cobra.Command{
		Use:   "get [KEY]",
		Short: "Show every key, or one key's value",
		Args:  maxArgs(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			w, err := c.api.GetConfig(cmd.Context())
			if err != nil {
				return err
			}
			flat, err := flatten(w)
			if err != nil {
				return err
			}
			if len(a) == 1 {
				v, ok := flat[a[0]]
				if !ok {
					return usagef("unknown key %q", a[0])
				}
				if c.json {
					return c.emit(v, nil)
				}
				_, err := fmt.Fprintln(c.stdout, render(v))
				return err
			}
			return c.emit(w, func(out io.Writer) error {
				keys := make([]string, 0, len(flat))
				for k := range flat {
					keys = append(keys, k)
				}
				slices.Sort(keys)
				for _, k := range keys {
					fmt.Fprintf(out, "%s = %s\n", k, render(flat[k]))
				}
				return nil
			})
		},
	}
	set := &cobra.Command{
		Use:   "set KEY VALUE",
		Short: "Change one key",
		Args:  args(2),
		RunE: func(cmd *cobra.Command, a []string) error {
			ctx := cmd.Context()
			w, err := c.api.GetConfig(ctx)
			if err != nil {
				return err
			}
			flat, err := flatten(w)
			if err != nil {
				return err
			}
			key, raw := a[0], a[1]
			current, ok := flat[key]
			section, name, dotted := strings.Cut(key, ".")
			if !ok || !dotted {
				return usagef("unknown key %q; see gwen config get", key)
			}
			var value any = raw
			switch current.(type) {
			case bool:
				b, err := strconv.ParseBool(raw)
				if err != nil {
					return usagef("%s takes true or false", key)
				}
				value = b
			case []any:
				list := []any{}
				for part := range strings.SplitSeq(raw, ",") {
					if part = strings.TrimSpace(part); part != "" {
						list = append(list, part)
					}
				}
				value = list
			}
			got, err := c.api.PatchConfig(ctx, wire.ConfigPatch{section: {name: value}})
			if err != nil {
				return err
			}
			newFlat, err := flatten(got)
			if err != nil {
				return err
			}
			return c.emit(got, func(out io.Writer) error {
				_, err := fmt.Fprintf(out, "%s = %s\n", key, render(newFlat[key]))
				return err
			})
		},
	}
	cfgCmd.AddCommand(get, set)
	return cfgCmd
}

// flatten turns the config into dotted keys and their JSON values.
func flatten(w *wire.Config) (map[string]any, error) {
	b, err := json.Marshal(w)
	if err != nil {
		return nil, err
	}
	var sections map[string]map[string]any
	if err := json.Unmarshal(b, &sections); err != nil {
		return nil, err
	}
	flat := map[string]any{}
	for s, keys := range sections {
		for k, v := range keys {
			flat[s+"."+k] = v
		}
	}
	return flat, nil
}

func render(v any) string {
	switch v := v.(type) {
	case []any:
		parts := make([]string, len(v))
		for i, p := range v {
			parts[i] = fmt.Sprint(p)
		}
		return strings.Join(parts, ",")
	case string:
		if v == "" {
			return `""`
		}
		return v
	}
	return fmt.Sprint(v)
}

func (c *cli) notifyCmd() *cobra.Command {
	notify := group("notify", "Notifications")
	test := &cobra.Command{
		Use:   "test",
		Short: "Send a test notification to the desktop and the phone",
		Args:  args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := c.api.NotifyTest(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(res, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Desktop: %s\nPhone:   %s\n", res.Desktop, res.Phone)
				return err
			})
		},
	}
	notify.AddCommand(test)
	return notify
}
