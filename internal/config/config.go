// Package config loads, validates, and saves config.toml. The table of keys,
// types, and defaults is owned by docs/03-data-model.md#configuration; this
// package is its only implementation.
//
// Config holds parsed values. Its string form, used for the file and the API, is
// wire.Config.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/kzark/gwen/internal/wire"
)

// LLM providers (llm.provider).
const (
	ProviderNone             = "none"
	ProviderAnthropic        = "anthropic"
	ProviderOpenAICompatible = "openai_compatible"
)

// Config is the parsed configuration.
type Config struct {
	Tracking Tracking
	Nudge    Nudge
	Ntfy     Ntfy
	Planner  Planner
	Calendar Calendar
	LLM      LLM
	Sync     Sync
	Log      Log
}

// Tracking is the [tracking] section.
type Tracking struct {
	DailyTarget time.Duration
	SoftIdle    time.Duration
	HardIdle    time.Duration
	DayRollover TimeOfDay
}

// Nudge is the [nudge] section.
type Nudge struct {
	Desktop       bool
	Phone         bool
	BreakReminder time.Duration
	Repeat        time.Duration
	Snooze        time.Duration
}

// Ntfy is the [ntfy] section. Topic is sensitive and never logged.
type Ntfy struct {
	Server         string
	FallbackServer string
	Topic          string
}

// Planner is the [planner] section.
type Planner struct {
	DayStart TimeOfDay
	DayEnd   TimeOfDay
	Buffer   time.Duration
}

// Calendar is the [calendar] section.
type Calendar struct {
	Enabled       bool
	Name          string
	BusyCalendars []string
}

// LLM is the [llm] section.
type LLM struct {
	Provider string
	Model    string
	Endpoint string
	Timeout  time.Duration
}

// Sync is the [sync] section.
type Sync struct {
	HubURL   string
	Interval time.Duration
}

// Log is the [log] section.
type Log struct {
	Level slog.Level
}

// KeyError is a configuration error attributable to one key.
type KeyError struct {
	Key string // dotted, e.g. "tracking.soft_idle"
	Msg string
}

func (e *KeyError) Error() string { return e.Key + ": " + e.Msg }

func keyErr(key, format string, args ...any) error {
	return &KeyError{Key: key, Msg: fmt.Sprintf(format, args...)}
}

// Defaults returns the default configuration.
func Defaults() Config {
	return Config{
		Tracking: Tracking{
			DailyTarget: 8 * time.Hour,
			SoftIdle:    3 * time.Minute,
			HardIdle:    10 * time.Minute,
			DayRollover: 4 * 60,
		},
		Nudge: Nudge{
			Desktop:       true,
			Phone:         false,
			BreakReminder: 15 * time.Minute,
			Repeat:        10 * time.Minute,
			Snooze:        10 * time.Minute,
		},
		Ntfy:     Ntfy{Server: "https://ntfy.sh"},
		Planner:  Planner{DayStart: 9 * 60, DayEnd: 23 * 60, Buffer: 30 * time.Minute},
		Calendar: Calendar{Enabled: false, Name: "Gwen", BusyCalendars: []string{"primary"}},
		LLM:      LLM{Provider: ProviderNone, Model: "claude-sonnet-5", Timeout: 30 * time.Second},
		Sync:     Sync{Interval: 5 * time.Minute},
		Log:      Log{Level: slog.LevelInfo},
	}
}

// Load reads the file at path, overlaying it key by key on the defaults. A
// missing file yields the defaults, which are then written to path with mode
// 0600. Unknown keys and invalid values are a *KeyError.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		c := Defaults()
		if err := Save(path, c); err != nil {
			return Config{}, err
		}
		return c, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	return parseTOML(data)
}

// parseTOML overlays the file on the defaults with the same key and type rules
// as Patch, so a file and a PATCH /v1/config body are checked identically.
func parseTOML(data []byte) (Config, error) {
	var raw map[string]any
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	sections := Defaults().Wire()
	var known map[string]any
	if err := roundTrip(sections, &known); err != nil {
		return Config{}, err
	}
	p := wire.ConfigPatch{}
	for name, v := range raw {
		table, ok := v.(map[string]any)
		if !ok {
			if _, isSection := known[name]; isSection {
				return Config{}, keyErr(name, "must be a [%s] table", name)
			}
			return Config{}, keyErr(name, "unknown key")
		}
		p[name] = table
	}
	return Patch(Defaults(), p)
}

// Save writes c to path atomically: a temporary file in the same directory is
// written with mode 0600, synced, and renamed over path.
func Save(path string, c Config) error {
	var buf bytes.Buffer
	buf.WriteString("# Gwen configuration. Keys and defaults: docs/03-data-model.md#configuration\n\n")
	if err := toml.NewEncoder(&buf).Encode(c.Wire()); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := writeFileAtomic(path, buf.Bytes(), 0o700); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// Wire returns the string form of c.
func (c Config) Wire() wire.Config {
	return wire.Config{
		Tracking: wire.TrackingConfig{
			DailyTarget: FormatDuration(c.Tracking.DailyTarget),
			SoftIdle:    FormatDuration(c.Tracking.SoftIdle),
			HardIdle:    FormatDuration(c.Tracking.HardIdle),
			DayRollover: c.Tracking.DayRollover.String(),
		},
		Nudge: wire.NudgeConfig{
			Desktop:       c.Nudge.Desktop,
			Phone:         c.Nudge.Phone,
			BreakReminder: FormatDuration(c.Nudge.BreakReminder),
			Repeat:        FormatDuration(c.Nudge.Repeat),
			Snooze:        FormatDuration(c.Nudge.Snooze),
		},
		Ntfy: wire.NtfyConfig{
			Server:         c.Ntfy.Server,
			FallbackServer: c.Ntfy.FallbackServer,
			Topic:          c.Ntfy.Topic,
		},
		Planner: wire.PlannerConfig{
			DayStart: c.Planner.DayStart.String(),
			DayEnd:   c.Planner.DayEnd.String(),
			Buffer:   FormatDuration(c.Planner.Buffer),
		},
		Calendar: wire.CalendarConfig{
			Enabled:       c.Calendar.Enabled,
			Name:          c.Calendar.Name,
			BusyCalendars: append([]string{}, c.Calendar.BusyCalendars...),
		},
		LLM: wire.LLMConfig{
			Provider: c.LLM.Provider,
			Model:    c.LLM.Model,
			Endpoint: c.LLM.Endpoint,
			Timeout:  FormatDuration(c.LLM.Timeout),
		},
		Sync: wire.SyncConfig{
			HubURL:   c.Sync.HubURL,
			Interval: FormatDuration(c.Sync.Interval),
		},
		Log: wire.LogConfig{Level: levelName(c.Log.Level)},
	}
}

// FromWire parses and validates the string form.
func FromWire(w wire.Config) (Config, error) {
	var p parser
	c := Config{
		Tracking: Tracking{
			DailyTarget: p.duration("tracking.daily_target", w.Tracking.DailyTarget),
			SoftIdle:    p.duration("tracking.soft_idle", w.Tracking.SoftIdle),
			HardIdle:    p.duration("tracking.hard_idle", w.Tracking.HardIdle),
			DayRollover: p.timeOfDay("tracking.day_rollover", w.Tracking.DayRollover),
		},
		Nudge: Nudge{
			Desktop:       w.Nudge.Desktop,
			Phone:         w.Nudge.Phone,
			BreakReminder: p.duration("nudge.break_reminder", w.Nudge.BreakReminder),
			Repeat:        p.duration("nudge.repeat", w.Nudge.Repeat),
			Snooze:        p.duration("nudge.snooze", w.Nudge.Snooze),
		},
		Ntfy: Ntfy{
			Server:         p.url("ntfy.server", w.Ntfy.Server, false),
			FallbackServer: p.url("ntfy.fallback_server", w.Ntfy.FallbackServer, true),
			Topic:          w.Ntfy.Topic,
		},
		Planner: Planner{
			DayStart: p.timeOfDay("planner.day_start", w.Planner.DayStart),
			DayEnd:   p.timeOfDay("planner.day_end", w.Planner.DayEnd),
			Buffer:   p.duration("planner.buffer", w.Planner.Buffer),
		},
		Calendar: Calendar{
			Enabled:       w.Calendar.Enabled,
			Name:          w.Calendar.Name,
			BusyCalendars: append([]string{}, w.Calendar.BusyCalendars...),
		},
		LLM: LLM{
			Provider: p.enum("llm.provider", w.LLM.Provider, ProviderNone, ProviderAnthropic, ProviderOpenAICompatible),
			Model:    w.LLM.Model,
			Endpoint: p.url("llm.endpoint", w.LLM.Endpoint, true),
			Timeout:  p.duration("llm.timeout", w.LLM.Timeout),
		},
		Sync: Sync{
			HubURL:   p.url("sync.hub_url", w.Sync.HubURL, true),
			Interval: p.duration("sync.interval", w.Sync.Interval),
		},
		Log: Log{Level: p.level("log.level", w.Log.Level)},
	}
	if p.err != nil {
		return Config{}, p.err
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate checks the rules that span keys or apply to parsed values.
func (c Config) Validate() error {
	durations := []struct {
		key string
		d   time.Duration
	}{
		{"tracking.daily_target", c.Tracking.DailyTarget},
		{"tracking.soft_idle", c.Tracking.SoftIdle},
		{"tracking.hard_idle", c.Tracking.HardIdle},
		{"nudge.break_reminder", c.Nudge.BreakReminder},
		{"nudge.repeat", c.Nudge.Repeat},
		{"nudge.snooze", c.Nudge.Snooze},
		{"planner.buffer", c.Planner.Buffer},
		{"llm.timeout", c.LLM.Timeout},
		{"sync.interval", c.Sync.Interval},
	}
	for _, d := range durations {
		if d.d <= 0 {
			return keyErr(d.key, "must be greater than zero")
		}
	}
	if c.Tracking.SoftIdle >= c.Tracking.HardIdle {
		return keyErr("tracking.soft_idle", "must be less than tracking.hard_idle")
	}
	if c.Planner.DayStart >= c.Planner.DayEnd {
		return keyErr("planner.day_start", "must be earlier than planner.day_end")
	}
	return nil
}

// LogValue renders c for slog with the ntfy topic redacted.
func (c Config) LogValue() slog.Value {
	w := c.Wire()
	topic := ""
	if w.Ntfy.Topic != "" {
		topic = "[redacted]"
	}
	return slog.GroupValue(
		slog.Group("tracking",
			"daily_target", w.Tracking.DailyTarget, "soft_idle", w.Tracking.SoftIdle,
			"hard_idle", w.Tracking.HardIdle, "day_rollover", w.Tracking.DayRollover),
		slog.Group("nudge",
			"desktop", w.Nudge.Desktop, "phone", w.Nudge.Phone, "break_reminder", w.Nudge.BreakReminder,
			"repeat", w.Nudge.Repeat, "snooze", w.Nudge.Snooze),
		slog.Group("ntfy",
			"server", w.Ntfy.Server, "fallback_server", w.Ntfy.FallbackServer, "topic", topic),
		slog.Group("planner",
			"day_start", w.Planner.DayStart, "day_end", w.Planner.DayEnd, "buffer", w.Planner.Buffer),
		slog.Group("calendar",
			"enabled", w.Calendar.Enabled, "name", w.Calendar.Name, "busy_calendars", w.Calendar.BusyCalendars),
		slog.Group("llm",
			"provider", w.LLM.Provider, "model", w.LLM.Model, "endpoint", w.LLM.Endpoint, "timeout", w.LLM.Timeout),
		slog.Group("sync", "hub_url", w.Sync.HubURL, "interval", w.Sync.Interval),
		slog.Group("log", "level", w.Log.Level),
	)
}

// FormatDuration renders d in the shortest Go duration syntax that parses back
// to it: "8h", "3m", "1h30m", "45s".
func FormatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}

// parser accumulates the first error while parsing the string form.
type parser struct{ err error }

func (p *parser) fail(key, format string, args ...any) {
	if p.err == nil {
		p.err = keyErr(key, format, args...)
	}
}

func (p *parser) duration(key, s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		p.fail(key, "%q is not a duration such as \"10m\" or \"1h30m\"", s)
		return 0
	}
	return d
}

func (p *parser) timeOfDay(key, s string) TimeOfDay {
	t, err := ParseTimeOfDay(s)
	if err != nil {
		p.fail(key, "%v", err)
	}
	return t
}

func (p *parser) url(key, s string, emptyOK bool) string {
	if s == "" {
		if !emptyOK {
			p.fail(key, "must not be empty")
		}
		return s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		p.fail(key, "%q is not an http or https URL", s)
	}
	return s
}

func (p *parser) enum(key, s string, allowed ...string) string {
	for _, a := range allowed {
		if s == a {
			return s
		}
	}
	p.fail(key, "%q is not one of %s", s, strings.Join(allowed, ", "))
	return s
}

func (p *parser) level(key, s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	p.fail(key, "%q is not one of debug, info, warn, error", s)
	return slog.LevelInfo
}

func levelName(l slog.Level) string {
	switch {
	case l <= slog.LevelDebug:
		return "debug"
	case l <= slog.LevelInfo:
		return "info"
	case l <= slog.LevelWarn:
		return "warn"
	default:
		return "error"
	}
}

// writeFileAtomic writes data to path via a synced temporary file and a rename,
// creating the parent directory with dirMode if needed. The file has mode 0600.
func writeFileAtomic(path string, data []byte, dirMode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	if d, err := os.Open(dir); err == nil {
		// Persist the rename; not every filesystem supports syncing a directory.
		_ = d.Sync()
		d.Close()
	}
	return nil
}
