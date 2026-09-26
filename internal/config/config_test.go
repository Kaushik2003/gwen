package config_test

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gwen", "config.toml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func requireKeyError(t *testing.T, err error, key string) {
	t.Helper()
	var ke *config.KeyError
	require.ErrorAs(t, err, &ke)
	require.Equal(t, key, ke.Key)
	require.Contains(t, err.Error(), key)
}

func TestLoadMissingFileWritesDefaults(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "gwen", "config.toml")

	c, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, config.Defaults(), c)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), info.Mode().Perm())

	again, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, c, again)
}

func TestDefaultsMatchTheSpec(t *testing.T) {
	t.Parallel()
	w := config.Defaults().Wire()
	require.Equal(t, wire.Config{
		Tracking: wire.TrackingConfig{DailyTarget: "8h", SoftIdle: "3m", HardIdle: "10m", DayRollover: "04:00"},
		Nudge:    wire.NudgeConfig{Desktop: true, Phone: false, BreakReminder: "15m", Repeat: "10m", Snooze: "10m"},
		Ntfy:     wire.NtfyConfig{Server: "https://ntfy.sh", FallbackServer: "", Topic: ""},
		Planner:  wire.PlannerConfig{DayStart: "09:00", DayEnd: "23:00", Buffer: "30m"},
		Calendar: wire.CalendarConfig{Enabled: false, Name: "Gwen", BusyCalendars: []string{"primary"}},
		LLM:      wire.LLMConfig{Provider: "none", Model: "claude-sonnet-5", Endpoint: "", Timeout: "30s"},
		Sync:     wire.SyncConfig{HubURL: "", Interval: "5m"},
		Log:      wire.LogConfig{Level: "info"},
	}, w)
}

func TestLoadPartialFileOverlays(t *testing.T) {
	t.Parallel()
	path := writeFile(t, `
[tracking]
soft_idle = "10s"
hard_idle = "30s"

[calendar]
busy_calendars = ["primary", "work@example.com"]
`)
	c, err := config.Load(path)
	require.NoError(t, err)

	want := config.Defaults()
	want.Tracking.SoftIdle = 10 * time.Second
	want.Tracking.HardIdle = 30 * time.Second
	want.Calendar.BusyCalendars = []string{"primary", "work@example.com"}
	require.Equal(t, want, c)
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		key  string
	}{
		{name: "unknown key", body: "[tracking]\nsof_idle = \"3m\"\n", key: "tracking.sof_idle"},
		{name: "unknown section", body: "[trackng]\nsoft_idle = \"3m\"\n", key: "trackng"},
		{name: "soft not below hard", body: "[tracking]\nsoft_idle = \"10m\"\nhard_idle = \"10m\"\n", key: "tracking.soft_idle"},
		{name: "bad HH:MM", body: "[tracking]\nday_rollover = \"4:00\"\n", key: "tracking.day_rollover"},
		{name: "HH:MM out of range", body: "[planner]\nday_end = \"24:00\"\n", key: "planner.day_end"},
		{name: "day start not before end", body: "[planner]\nday_start = \"23:00\"\n", key: "planner.day_start"},
		{name: "zero duration", body: "[nudge]\nsnooze = \"0s\"\n", key: "nudge.snooze"},
		{name: "negative duration", body: "[planner]\nbuffer = \"-5m\"\n", key: "planner.buffer"},
		{name: "not a duration", body: "[sync]\ninterval = \"often\"\n", key: "sync.interval"},
		{name: "unknown enum", body: "[llm]\nprovider = \"skynet\"\n", key: "llm.provider"},
		{name: "unknown log level", body: "[log]\nlevel = \"loud\"\n", key: "log.level"},
		{name: "bad url", body: "[ntfy]\nfallback_server = \"ntfy.local\"\n", key: "ntfy.fallback_server"},
		{name: "empty required url", body: "[ntfy]\nserver = \"\"\n", key: "ntfy.server"},
		{name: "wrong toml type", body: "[nudge]\ndesktop = \"yes\"\n", key: "nudge.desktop"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.Load(writeFile(t, tc.body))
			requireKeyError(t, err, tc.key)
		})
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.toml")
	c := config.Defaults()
	c.Tracking.DailyTarget = 7*time.Hour + 30*time.Minute
	c.Tracking.DayRollover = 3*60 + 30
	c.Nudge.Phone = true
	c.Ntfy.Topic = config.NewTopic()
	c.Ntfy.FallbackServer = "http://192.168.1.20:8080"
	c.Calendar.BusyCalendars = []string{}
	c.LLM.Provider = config.ProviderOpenAICompatible
	c.LLM.Endpoint = "http://localhost:11434/v1"
	c.Log.Level = slog.LevelDebug

	require.NoError(t, config.Save(path, c))
	got, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, c, got)

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), info.Mode().Perm())
	_, err = os.Stat(path + ".tmp")
	require.True(t, errors.Is(err, fs.ErrNotExist), "temporary file left behind")
}

func TestLogValueRedactsTopic(t *testing.T) {
	t.Parallel()
	c := config.Defaults()
	c.Ntfy.Topic = config.NewTopic()

	for _, newHandler := range []func(*bytes.Buffer) slog.Handler{
		func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
		func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
	} {
		var buf bytes.Buffer
		slog.New(newHandler(&buf)).Info("config loaded", "config", c)
		out := buf.String()
		require.NotContains(t, out, c.Ntfy.Topic)
		require.Contains(t, out, "redacted")
		require.Contains(t, out, "soft_idle")
	}
}

func TestPatch(t *testing.T) {
	t.Parallel()
	base := config.Defaults()
	tests := []struct {
		name    string
		patch   wire.ConfigPatch
		check   func(t *testing.T, c config.Config)
		wantKey string
	}{
		{
			name:  "durations",
			patch: wire.ConfigPatch{"tracking": {"soft_idle": "10s", "hard_idle": "30s"}},
			check: func(t *testing.T, c config.Config) {
				require.Equal(t, 10*time.Second, c.Tracking.SoftIdle)
				require.Equal(t, 30*time.Second, c.Tracking.HardIdle)
			},
		},
		{
			name:  "bool and list",
			patch: wire.ConfigPatch{"nudge": {"phone": true}, "calendar": {"busy_calendars": []any{"a", "b"}}},
			check: func(t *testing.T, c config.Config) {
				require.True(t, c.Nudge.Phone)
				require.Equal(t, []string{"a", "b"}, c.Calendar.BusyCalendars)
				require.Equal(t, base.Tracking, c.Tracking)
			},
		},
		{name: "unknown section", patch: wire.ConfigPatch{"trackng": {"soft_idle": "1m"}}, wantKey: "trackng"},
		{name: "unknown key", patch: wire.ConfigPatch{"tracking": {"idle": "1m"}}, wantKey: "tracking.idle"},
		{name: "wrong type", patch: wire.ConfigPatch{"nudge": {"desktop": "yes"}}, wantKey: "nudge.desktop"},
		{name: "null", patch: wire.ConfigPatch{"ntfy": {"topic": nil}}, wantKey: "ntfy.topic"},
		{name: "list of numbers", patch: wire.ConfigPatch{"calendar": {"busy_calendars": []any{1.0}}}, wantKey: "calendar.busy_calendars"},
		{name: "invalid value", patch: wire.ConfigPatch{"tracking": {"day_rollover": "25:00"}}, wantKey: "tracking.day_rollover"},
		{name: "cross-key rule", patch: wire.ConfigPatch{"tracking": {"soft_idle": "20m"}}, wantKey: "tracking.soft_idle"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := config.Patch(base, tc.patch)
			if tc.wantKey != "" {
				requireKeyError(t, err, tc.wantKey)
				return
			}
			require.NoError(t, err)
			tc.check(t, got)
		})
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		d    time.Duration
		want string
	}{
		{8 * time.Hour, "8h"},
		{3 * time.Minute, "3m"},
		{90 * time.Minute, "1h30m"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{time.Hour + time.Second, "1h0m1s"},
		{1500 * time.Millisecond, "1.5s"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, config.FormatDuration(tc.d))
			back, err := time.ParseDuration(tc.want)
			require.NoError(t, err)
			require.Equal(t, tc.d, back)
		})
	}
}

func TestTimeOfDay(t *testing.T) {
	t.Parallel()
	tod, err := config.ParseTimeOfDay("04:30")
	require.NoError(t, err)
	require.Equal(t, 4, tod.Hour())
	require.Equal(t, 30, tod.Minute())
	require.Equal(t, "04:30", tod.String())

	ny, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	at := config.TimeOfDay(4*60).On(2026, time.October, 31+1, ny)
	require.Equal(t, time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), at.UTC(), "04:00 EST after the fall-back change")

	for _, bad := range []string{"", "4:30", "04:3", "0430", "24:00", "12:60", "ab:cd", "-1:00"} {
		_, err := config.ParseTimeOfDay(bad)
		require.Error(t, err, bad)
	}
}

func TestNewTopic(t *testing.T) {
	t.Parallel()
	a, b := config.NewTopic(), config.NewTopic()
	require.Regexp(t, regexp.MustCompile(`^gwen-[a-z2-7]{24}$`), a)
	require.NotEqual(t, a, b)
}

func TestCredentials(t *testing.T) {
	t.Parallel()
	dir := config.CredentialsDir(t.TempDir())

	_, err := config.ReadCredentialLine(dir, config.CredNtfyToken)
	require.ErrorIs(t, err, fs.ErrNotExist)

	require.NoError(t, config.WriteCredential(dir, config.CredNtfyToken, []byte("tk_secret\n")))
	got, err := config.ReadCredentialLine(dir, config.CredNtfyToken)
	require.NoError(t, err)
	require.Equal(t, "tk_secret", got)

	dirInfo, err := os.Stat(dir)
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o700), dirInfo.Mode().Perm())
	fileInfo, err := os.Stat(filepath.Join(dir, config.CredNtfyToken))
	require.NoError(t, err)
	require.Equal(t, fs.FileMode(0o600), fileInfo.Mode().Perm())

	require.Error(t, config.WriteCredential(dir, "../escape", []byte("x")))
}

func TestDefaultPaths(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "relative/ignored")
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")

	p, err := config.DefaultPaths()
	require.NoError(t, err)
	require.Equal(t, config.Paths{
		ConfigHome: "/home/u/.config",
		ConfigFile: "/home/u/.config/gwen/config.toml",
		DataDir:    "/home/u/.local/share/gwen",
		StateDir:   "/xdg/state/gwen",
		RuntimeDir: "/run/user/1000/gwen",
	}, p)
	require.Equal(t, "/home/u/.local/share/gwen/gwen.db", p.DatabaseFile())
	require.Equal(t, "/home/u/.local/share/gwen/credentials", p.CredentialsDir())
	require.Equal(t, "/xdg/state/gwen/gwend.log", p.LogFile())
	require.Equal(t, "/run/user/1000/gwen/gwend.sock", p.SocketPath())

	sock, err := config.DefaultSocketPath()
	require.NoError(t, err)
	require.Equal(t, p.SocketPath(), sock)
}
