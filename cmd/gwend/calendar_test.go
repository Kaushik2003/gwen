package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

func TestCalendarEndpoints(t *testing.T) {
	t.Parallel()
	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error": {"code": 500, "message": "down"}}`, http.StatusInternalServerError)
	}))
	t.Cleanup(google.Close)
	d := startDaemonWith(t, setup{calendar: true}, func(o *options) { o.calendarEndpoint = google.URL + "/" })

	_, err := d.c.CalendarStatus(d.ctx)
	require.Contains(t, apiErr(t, err, wire.CodeUnavailable).Message, "gwen setup calendar")
	_, err = d.c.CalendarAuthStart(d.ctx)
	apiErr(t, err, wire.CodeUnavailable)
	_, err = d.c.CalendarSync(d.ctx)
	apiErr(t, err, wire.CodeUnavailable)

	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"calendar": {"enabled": true}})
	require.NoError(t, err)
	credDir := config.CredentialsDir(d.dataDir)
	client, _ := json.Marshal(map[string]any{"installed": map[string]any{"client_id": "id", "client_secret": "s",
		"auth_uri": "https://accounts.google.com/o/oauth2/auth", "token_uri": google.URL + "/token", "redirect_uris": []string{"http://localhost"}}})
	require.NoError(t, config.WriteCredential(credDir, config.CredGoogleClient, client))

	st, err := d.c.CalendarStatus(d.ctx)
	require.NoError(t, err)
	require.Equal(t, wire.CalendarStatus{Enabled: true}, *st)
	auth, err := d.c.CalendarAuthStart(d.ctx)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(auth.AuthURL, "https://accounts.google.com/o/oauth2/auth?"), auth.AuthURL)
	_, err = d.c.CalendarSync(d.ctx)
	require.Contains(t, apiErr(t, err, wire.CodeUnavailable).Message, "gwen cal connect")

	// Connected, a failing sync reports its error and integration_changed.
	next := stream(t, d)
	tok, _ := json.Marshal(map[string]any{"access_token": "at", "token_type": "Bearer", "expiry": time.Now().Add(time.Hour)})
	require.NoError(t, config.WriteCredential(credDir, config.CredGoogleToken, tok))
	st, err = d.c.CalendarSync(d.ctx)
	require.NoError(t, err)
	require.True(t, st.Connected)
	require.NotNil(t, st.LastError)
	var ev wire.IntegrationChanged
	require.NoError(t, json.Unmarshal(next(wire.EventIntegrationChanged).Data, &ev))
	require.Equal(t, *st, ev.Calendar)
	require.False(t, ev.Sync.Configured)
}

func TestCalendarUnavailableWithoutTheService(t *testing.T) {
	t.Parallel()
	d := startDaemon(t, setup{})
	_, err := d.c.CalendarStatus(d.ctx)
	apiErr(t, err, wire.CodeUnavailable)
}
