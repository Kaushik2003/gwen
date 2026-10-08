package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/client/clienttest"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestCalendarGolden(t *testing.T) {
	t.Parallel()
	connected := &wire.CalendarStatus{Enabled: true, Connected: true, CalendarID: testutil.Ptr("cal-1"),
		LastSyncAt: testutil.Ptr(wire.Millis(testutil.At("10:20")))}
	failed := &wire.CalendarStatus{Enabled: true, Connected: true, LastError: testutil.Ptr("push events: quota")}
	tests := []struct {
		name, method string
		args         []string
		result       any
		out          string
	}{
		{"status", "CalendarStatus", []string{"cal", "status"}, connected,
			"Calendar: connected\nGwen calendar: cal-1\nLast sync: 2026-09-15 10:20 am\n"},
		{"status off", "CalendarStatus", []string{"cal", "status"}, &wire.CalendarStatus{},
			"Calendar: turned off\nLast sync: never\n"},
		{"sync with an error", "CalendarSync", []string{"cal", "sync"}, failed,
			"Calendar: connected\nLast sync: never\nLast error: push events: quota\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := fake().Returns(tc.method, tc.result, nil)
			out, errOut, code := runCLI(t, f, tc.args...)
			require.Equal(t, exitOK, code, errOut)
			require.Equal(t, tc.out, out)
		})
	}
}

func TestCalendarConnectOpensTheBrowser(t *testing.T) {
	t.Parallel()
	f := fake().Returns("CalendarAuthStart", &wire.CalendarAuth{AuthURL: "https://accounts.google.com/o/oauth2/auth?x=1"}, nil)
	var out, errb bytes.Buffer
	var opened []string
	c := &cli{newAPI: func(string) client.API { return f }, stdin: strings.NewReader(""), stdout: &out, stderr: &errb,
		clk: clock.NewFake(now), loc: time.UTC, open: func(u string) error {
			opened = append(opened, u)
			return errors.New("no browser")
		}}
	require.Equal(t, exitOK, c.execute(context.Background(), []string{"cal", "connect"}))
	require.Equal(t, []string{"https://accounts.google.com/o/oauth2/auth?x=1"}, opened)
	require.Contains(t, out.String(), "  https://accounts.google.com/o/oauth2/auth?x=1\n")
	require.Contains(t, errb.String(), "could not open a browser")

	f = clienttest.New().Returns("CalendarAuthStart", nil, &client.APIError{Code: wire.CodeUnavailable,
		Message: "no Google OAuth client; run gwen setup calendar --client-file PATH"})
	_, errOut, code := runCLI(t, f, "cal", "connect")
	require.Equal(t, exitAPI, code)
	require.Contains(t, errOut, "gwen setup calendar")
}

func TestSetupCalendar(t *testing.T) {
	t.Parallel()
	f := clienttest.New().Returns("PatchConfig", &wire.Config{}, nil)
	r := setupTestEnv(t, f, "")
	require.NoError(t, setupExec(t, r, "calendar"))
	require.Contains(t, r.out.String(), "Desktop app")
	require.Empty(t, f.CallsTo("PatchConfig"), "without a file it only explains")

	bad := filepath.Join(t.TempDir(), "web.json")
	require.NoError(t, os.WriteFile(bad, []byte(`{"web": {"client_id": "x"}}`), 0o600))
	require.Error(t, setupExec(t, r, "calendar", "--client-file", bad))

	good := filepath.Join(t.TempDir(), "client.json")
	body := `{"installed": {"client_id": "id.apps.googleusercontent.com", "client_secret": "s"}}`
	require.NoError(t, os.WriteFile(good, []byte(body), 0o600))
	require.NoError(t, setupExec(t, r, "calendar", "--client-file", good))
	got, err := config.ReadCredential(r.env.credDir, config.CredGoogleClient)
	require.NoError(t, err)
	require.Equal(t, body, string(got))
	info, err := os.Stat(filepath.Join(r.env.credDir, config.CredGoogleClient))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	require.Equal(t, []any{wire.ConfigPatch{"calendar": {"enabled": true}}}, f.CallsTo("PatchConfig")[0].Args)
	require.Contains(t, r.out.String(), "gwen cal connect")
}
