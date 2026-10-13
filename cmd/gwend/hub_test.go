package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

const hubToken = "hub-secret-token"

// startHub runs gwend --hub in-process on a free port and returns its URL
// and data directory.
func startHub(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	require.NoError(t, config.WriteCredential(config.CredentialsDir(dataDir), config.CredSyncToken, []byte(hubToken+"\n")))
	ready := make(chan string, 1)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runHub(ctx, hubOptions{dataDir: dataDir, configPath: filepath.Join(dir, "config.toml"),
			listen: "127.0.0.1:0", version: "1.0.0-test", clk: clock.NewFake(t0), loc: time.UTC,
			level: new(slog.LevelVar), ready: ready})
	}()
	addr := <-ready
	require.NotEmpty(t, addr)
	t.Cleanup(func() {
		cancel()
		require.NoError(t, <-done)
	})
	return "http://" + addr, dataDir
}

func hubGet(t *testing.T, u, bearer string, cookie *http.Cookie) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	require.NoError(t, err)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestHubServesTheReadOnlySubset(t *testing.T) {
	t.Parallel()
	base, _ := startHub(t)

	code, body := hubGet(t, base+"/sync/v1/health", hubToken, nil)
	require.Equal(t, http.StatusOK, code)
	require.JSONEq(t, `{"ok": true, "schema_version": 10}`, body)
	code, _ = hubGet(t, base+"/sync/v1/health", "wrong", nil)
	require.Equal(t, http.StatusUnauthorized, code)
	code, _ = hubGet(t, base+"/v1/health", "", nil)
	require.Equal(t, http.StatusUnauthorized, code)

	resp, err := http.PostForm(base+"/login", url.Values{"token": {hubToken}})
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	cookie := resp.Cookies()[0]
	for _, path := range []string{"/v1/health", "/v1/days?from=2026-09-01&to=2026-09-30", "/v1/projects", "/v1/tasks",
		"/v1/stats/summary?from=2026-09-01&to=2026-09-30", "/v1/stats/heatmap", "/v1/goals", "/v1/commitments",
		"/v1/plan?day=2026-09-15"} {
		code, body := hubGet(t, base+path, "", cookie)
		require.Equal(t, http.StatusOK, code, "%s: %s", path, body)
	}
	for _, path := range []string{"/v1/status", "/v1/events", "/v1/config", "/v1/briefing"} {
		code, _ := hubGet(t, base+path, "", cookie)
		require.Equal(t, http.StatusNotFound, code, path)
	}
	req, _ := http.NewRequest(http.MethodPost, base+"/v1/projects", strings.NewReader(`{"name": "x"}`))
	req.AddCookie(cookie)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode, "no writes on the hub")

	code, body = hubGet(t, base+"/", "", nil)
	require.Equal(t, http.StatusOK, code)
	require.Contains(t, body, "<title>Gwen</title>", "the dashboard needs no sign-in to load")
	code, _ = hubGet(t, base+"/history", "", nil)
	require.Equal(t, http.StatusOK, code, "unknown paths serve the page")
}

func TestLaptopSyncsToTheHub(t *testing.T) {
	t.Parallel()
	base, hubData := startHub(t)
	d := startDaemonWith(t, setup{}, func(o *options) { o.sync = true })

	_, err := d.c.SyncStatus(d.ctx)
	require.Contains(t, apiErr(t, err, wire.CodeUnavailable).Message, "gwen setup sync")

	p := d.project("Internship")
	tk, err := d.c.CreateTask(d.ctx, wire.CreateTaskRequest{Title: "Report", ProjectID: &p.ID,
		DueDay: testutil.Ptr(testutil.Day0), EstimateMinutes: testutil.Ptr(60)})
	require.NoError(t, err)
	_, err = d.c.GetPlan(d.ctx, "")
	require.NoError(t, err)
	_, err = d.c.PatchConfig(d.ctx, wire.ConfigPatch{"sync": {"hub_url": base}})
	require.NoError(t, err)
	require.NoError(t, config.WriteCredential(config.CredentialsDir(d.dataDir), config.CredSyncToken, []byte(hubToken+"\n")))

	next := stream(t, d)
	st, err := d.c.SyncNow(d.ctx)
	require.NoError(t, err)
	require.True(t, st.Configured)
	require.Nil(t, st.LastError)
	require.NotNil(t, st.LastPushAt)
	var ev wire.IntegrationChanged
	require.NoError(t, json.Unmarshal(next(wire.EventIntegrationChanged).Data, &ev))
	require.Equal(t, *st, ev.Sync)

	code, body := hubGet(t, base+"/v1/tasks/"+tk.ID, hubToken, nil)
	require.Equal(t, http.StatusOK, code, body)
	var got wire.Task
	require.NoError(t, json.Unmarshal([]byte(body), &got))
	require.Equal(t, "Report", got.Title)
	code, body = hubGet(t, base+"/v1/plan?day="+testutil.Day0, hubToken, nil)
	require.Equal(t, http.StatusOK, code, body)
	var plan wire.Plan
	require.NoError(t, json.Unmarshal([]byte(body), &plan))
	require.Len(t, plan.Items, 1, "the laptop's plan, as stored")

	// The hub never generates: a day with no stored plan stays empty.
	code, body = hubGet(t, base+"/v1/plan?day=2026-09-16", hubToken, nil)
	require.Equal(t, http.StatusOK, code, body)
	require.NoError(t, json.Unmarshal([]byte(body), &plan))
	require.Empty(t, plan.Items)
	_ = hubData
}

// The hub takes a push holding a table it does not know, keeping the rest,
// and holds memory back from a client that does not say its schema: one from
// before memory, which could not take it.
func TestHubNegotiatesTables(t *testing.T) {
	t.Parallel()
	base, _ := startHub(t)
	const rows = `{
		"projects": [{"id": "00000000-0000-7000-8000-00000000000a", "name": "Study", "color": "#10b981",
			"archived_at": null, "created_at": 1, "updated_at": 1, "deleted_at": null, "device_id": "laptop", "rev": 1}],
		"entities": [{"id": "00000000-0000-7000-8000-0000000000e1", "kind": "pet", "name": "Miso", "aliases": "[]",
			"created_at": 1, "updated_at": 1, "deleted_at": null, "device_id": "laptop", "rev": 1}],
		"run_steps": [{"id": "from a newer gwend"}]}`
	require.JSONEq(t, `{"applied": 2, "ignored": 1}`, hubPush(t, base, "laptop", rows))

	tables := func(query string) []string {
		t.Helper()
		code, body := hubGet(t, base+"/sync/v1/pull?device_id=phone"+query, hubToken, nil)
		require.Equal(t, http.StatusOK, code, body)
		var page struct {
			Rows   map[string]json.RawMessage `json:"rows"`
			Cursor string                     `json:"cursor"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &page))
		require.Equal(t, "2", page.Cursor, query)
		var out []string
		for table := range page.Rows {
			out = append(out, table)
		}
		slices.Sort(out)
		return out
	}
	require.Equal(t, []string{"projects"}, tables(""))
	require.Equal(t, []string{"entities", "projects"}, tables("&schema_version=10"))
}

// hubPush pushes rows, as JSON, to the hub as device and returns the reply.
func hubPush(t *testing.T, base, device, rows string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, base+"/sync/v1/push",
		strings.NewReader(`{"device_id": "`+device+`", "rows": `+rows+`}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+hubToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, string(b))
	return string(b)
}

// Memory that another device learned is announced when it is pulled.
func TestPulledMemoryIsAnnounced(t *testing.T) {
	t.Parallel()
	base, _ := startHub(t)
	const claimID = "00000000-0000-7000-8000-0000000000d1"
	const rows = `{
		"claims": [{"id": "` + claimID + `", "key": "user.pet", "subject_id": null, "value": "\"a cat\"",
			"text": "They have a cat, Miso", "cardinality": "many", "status": "active", "trust": "owner_said",
			"sensitivity": "personal", "confidence": 0.75, "valid_from": null, "valid_to": null, "occurred_at": null,
			"mentioned_at": 1, "recorded_at": 1, "superseded_at": null, "superseded_by": null, "suppressed": 0,
			"source_kind": "msg", "source_id": "", "span": "", "source_device_id": "phone", "channel": "chat",
			"extractor": "", "content_hash": "h1", "created_at": 1, "updated_at": 1, "deleted_at": null,
			"device_id": "phone", "rev": 1}],
		"conflicts": [{"id": "00000000-0000-7000-8000-0000000000d3", "claim_a": "` + claimID + `",
			"claim_b": "00000000-0000-7000-8000-0000000000d2", "reason": "", "sensitivity": "personal",
			"status": "open", "resolution": null, "resolved_at": null, "created_at": 1, "updated_at": 1,
			"deleted_at": null, "device_id": "phone", "rev": 1}]}`
	require.JSONEq(t, `{"applied": 2, "ignored": 0}`, hubPush(t, base, "phone", rows))

	d := startDaemonWith(t, setup{}, func(o *options) { o.sync = true })
	_, err := d.c.PatchConfig(d.ctx, wire.ConfigPatch{"sync": {"hub_url": base}})
	require.NoError(t, err)
	cred := []byte(hubToken + "\n")
	require.NoError(t, config.WriteCredential(config.CredentialsDir(d.dataDir), config.CredSyncToken, cred))
	next := stream(t, d)
	st, err := d.c.SyncNow(d.ctx)
	require.NoError(t, err)
	require.Nil(t, st.LastError)
	var ev wire.MemoryChanged
	require.NoError(t, json.Unmarshal(next(wire.EventMemoryChanged).Data, &ev))
	require.Equal(t, wire.MemoryChanged{ClaimIDs: []string{claimID}, Conflicts: true}, ev)
}
