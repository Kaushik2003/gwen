package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// serve starts h on a fresh Unix socket and returns a client for it.
func serve(t *testing.T, h http.Handler) *client.Client {
	t.Helper()
	path := testutil.SocketPath(t)
	ln, err := net.Listen("unix", path)
	require.NoError(t, err)
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	return client.New(path)
}

type recorded struct {
	Method string
	Path   string
	Query  string
	Body   string
}

func recorder(t *testing.T, status int, response string) (*client.Client, *recorded) {
	t.Helper()
	var mu sync.Mutex
	got := &recorded{}
	c := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		*got = recorded{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery, Body: string(body)}
		mu.Unlock()
		if len(body) > 0 {
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, response)
	}))
	return c, got
}

func TestEveryMethodMatchesTheContract(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	id := "01926d2e-7a4b-7c3d-8e9f-0a1b2c3d4e5f"
	tests := []struct {
		name   string
		status int
		call   func(c *client.Client) error
		want   recorded
	}{
		{"Health", 200, func(c *client.Client) error { _, err := c.Health(ctx); return err },
			recorded{"GET", "/v1/health", "", ""}},
		{"Status", 200, func(c *client.Client) error { _, err := c.Status(ctx); return err },
			recorded{"GET", "/v1/status", "", ""}},
		{"ClockIn", 200, func(c *client.Client) error {
			_, err := c.ClockIn(ctx, wire.ClockInRequest{ProjectID: &id})
			return err
		}, recorded{"POST", "/v1/day/clock-in", "", `{"project_id":"` + id + `","task_id":null}`}},
		{"ClockOut", 200, func(c *client.Client) error { _, err := c.ClockOut(ctx); return err },
			recorded{"POST", "/v1/day/clock-out", "", `{}`}},
		{"BreakStart", 200, func(c *client.Client) error { _, err := c.BreakStart(ctx); return err },
			recorded{"POST", "/v1/break/start", "", `{}`}},
		{"BreakEnd", 200, func(c *client.Client) error { _, err := c.BreakEnd(ctx); return err },
			recorded{"POST", "/v1/break/end", "", `{}`}},
		{"Switch", 200, func(c *client.Client) error {
			_, err := c.Switch(ctx, wire.SwitchRequest{})
			return err
		}, recorded{"POST", "/v1/session/switch", "", `{"project_id":null,"task_id":null}`}},
		{"Snooze", 200, func(c *client.Client) error { _, err := c.Snooze(ctx); return err },
			recorded{"POST", "/v1/nudge/snooze", "", `{}`}},
		{"ListDays", 200, func(c *client.Client) error { _, err := c.ListDays(ctx, "2026-09-09", "2026-09-15"); return err },
			recorded{"GET", "/v1/days", "from=2026-09-09&to=2026-09-15", ""}},
		{"GetDay", 200, func(c *client.Client) error { _, err := c.GetDay(ctx, "2026-09-15"); return err },
			recorded{"GET", "/v1/days/2026-09-15", "", ""}},
		{"PatchDay", 200, func(c *client.Client) error {
			_, err := c.PatchDay(ctx, "2026-09-15", wire.PatchDayRequest{Note: testutil.Ptr("sick"), Rev: testutil.Ptr(int64(2))})
			return err
		}, recorded{"PATCH", "/v1/days/2026-09-15", "", `{"note":"sick","rev":2}`}},
		{"CreateSegment", 201, func(c *client.Client) error {
			_, err := c.CreateSegment(ctx, wire.CreateSegmentRequest{Day: "2026-09-15", Kind: "work", StartedAt: 1, EndedAt: testutil.Ptr(int64(2))})
			return err
		}, recorded{"POST", "/v1/segments", "", `{"day":"2026-09-15","kind":"work","project_id":null,"task_id":null,"started_at":1,"ended_at":2}`}},
		{"PatchSegment", 200, func(c *client.Client) error {
			_, err := c.PatchSegment(ctx, id, wire.PatchSegmentRequest{ProjectID: wire.Null[string](), Rev: testutil.Ptr(int64(4))})
			return err
		}, recorded{"PATCH", "/v1/segments/" + id, "", `{"project_id":null,"rev":4}`}},
		{"SplitSegment", 200, func(c *client.Client) error {
			_, err := c.SplitSegment(ctx, id, wire.SplitSegmentRequest{At: 5})
			return err
		}, recorded{"POST", "/v1/segments/" + id + "/split", "", `{"at":5}`}},
		{"DeleteSegment", 204, func(c *client.Client) error { return c.DeleteSegment(ctx, id) },
			recorded{"DELETE", "/v1/segments/" + id, "", ""}},
		{"ListProjects default", 200, func(c *client.Client) error { _, err := c.ListProjects(ctx, ""); return err },
			recorded{"GET", "/v1/projects", "", ""}},
		{"ListProjects all", 200, func(c *client.Client) error { _, err := c.ListProjects(ctx, wire.ArchivedAll); return err },
			recorded{"GET", "/v1/projects", "archived=all", ""}},
		{"CreateProject", 201, func(c *client.Client) error {
			_, err := c.CreateProject(ctx, wire.CreateProjectRequest{Name: "Internship"})
			return err
		}, recorded{"POST", "/v1/projects", "", `{"name":"Internship"}`}},
		{"GetProject", 200, func(c *client.Client) error { _, err := c.GetProject(ctx, id); return err },
			recorded{"GET", "/v1/projects/" + id, "", ""}},
		{"PatchProject", 200, func(c *client.Client) error {
			_, err := c.PatchProject(ctx, id, wire.PatchProjectRequest{Archived: testutil.Ptr(true)})
			return err
		}, recorded{"PATCH", "/v1/projects/" + id, "", `{"archived":true}`}},
		{"DeleteProject", 204, func(c *client.Client) error { return c.DeleteProject(ctx, id) },
			recorded{"DELETE", "/v1/projects/" + id, "", ""}},
		{"ListTasks", 200, func(c *client.Client) error {
			_, err := c.ListTasks(ctx, wire.TaskQuery{ProjectID: id, Status: "all", DueBefore: "2026-10-01"})
			return err
		}, recorded{"GET", "/v1/tasks", "due_before=2026-10-01&project_id=" + id + "&status=all", ""}},
		{"CreateTask", 201, func(c *client.Client) error {
			_, err := c.CreateTask(ctx, wire.CreateTaskRequest{Title: "Read"})
			return err
		}, recorded{"POST", "/v1/tasks", "", `{"project_id":null,"title":"Read","due_day":null,"estimate_minutes":null}`}},
		{"GetTask", 200, func(c *client.Client) error { _, err := c.GetTask(ctx, id); return err },
			recorded{"GET", "/v1/tasks/" + id, "", ""}},
		{"PatchTask", 200, func(c *client.Client) error {
			_, err := c.PatchTask(ctx, id, wire.PatchTaskRequest{DueDay: wire.Null[string](), Title: testutil.Ptr("Write")})
			return err
		}, recorded{"PATCH", "/v1/tasks/" + id, "", `{"title":"Write","due_day":null}`}},
		{"CompleteTask", 200, func(c *client.Client) error {
			_, err := c.CompleteTask(ctx, id, wire.CompleteTaskRequest{})
			return err
		}, recorded{"POST", "/v1/tasks/" + id + "/complete", "", `{}`}},
		{"ReopenTask", 200, func(c *client.Client) error { _, err := c.ReopenTask(ctx, id); return err },
			recorded{"POST", "/v1/tasks/" + id + "/reopen", "", `{}`}},
		{"DeleteTask", 204, func(c *client.Client) error { return c.DeleteTask(ctx, id) },
			recorded{"DELETE", "/v1/tasks/" + id, "", ""}},
		{"StatsSummary", 200, func(c *client.Client) error { _, err := c.StatsSummary(ctx, "2026-08-17", "2026-09-15"); return err },
			recorded{"GET", "/v1/stats/summary", "from=2026-08-17&to=2026-09-15", ""}},
		{"StatsHeatmap", 200, func(c *client.Client) error { _, err := c.StatsHeatmap(ctx, 2026); return err },
			recorded{"GET", "/v1/stats/heatmap", "year=2026", ""}},
		{"GetConfig", 200, func(c *client.Client) error { _, err := c.GetConfig(ctx); return err },
			recorded{"GET", "/v1/config", "", ""}},
		{"PatchConfig", 200, func(c *client.Client) error {
			_, err := c.PatchConfig(ctx, wire.ConfigPatch{"tracking": {"soft_idle": "10s"}})
			return err
		}, recorded{"PATCH", "/v1/config", "", `{"tracking":{"soft_idle":"10s"}}`}},
		{"NotifyTest", 200, func(c *client.Client) error { _, err := c.NotifyTest(ctx); return err },
			recorded{"POST", "/v1/notify/test", "", `{}`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := "{}"
			if tc.status == http.StatusNoContent {
				body = ""
			}
			c, got := recorder(t, tc.status, body)
			require.NoError(t, tc.call(c))
			require.Equal(t, tc.want.Method, got.Method)
			require.Equal(t, tc.want.Path, got.Path)
			require.Equal(t, tc.want.Query, got.Query)
			if tc.want.Body == "" {
				require.Empty(t, got.Body)
			} else {
				require.JSONEq(t, tc.want.Body, got.Body)
			}
		})
	}
}

func TestResponseDecoding(t *testing.T) {
	t.Parallel()
	c, _ := recorder(t, 200, `{"ok": true, "version": "1.0.0", "schema_version": 1, "pid": 4242, "future_field": 1}`)
	h, err := c.Health(context.Background())
	require.NoError(t, err)
	require.Equal(t, wire.Health{OK: true, Version: "1.0.0", SchemaVersion: 1, PID: 4242}, *h)
}

func TestErrorEnvelopeBecomesAPIError(t *testing.T) {
	t.Parallel()
	c, _ := recorder(t, 422, `{"error": {"code": "invalid_state", "message": "cannot start a break while off", "details": {"state": "off"}}}`)
	_, err := c.BreakStart(context.Background())
	var ae *client.APIError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, 422, ae.Status)
	require.Equal(t, wire.CodeInvalidState, ae.Code)
	require.Equal(t, "cannot start a break while off", ae.Message)
	require.Equal(t, "cannot start a break while off", err.Error())
	require.Equal(t, map[string]any{"state": "off"}, ae.Details)
	require.True(t, client.IsCode(err, wire.CodeInvalidState))
	require.False(t, client.IsCode(err, wire.CodeConflict))
}

func TestErrorWithoutEnvelope(t *testing.T) {
	t.Parallel()
	c, _ := recorder(t, 502, `<html>bad gateway</html>`)
	_, err := c.Status(context.Background())
	var ae *client.APIError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, wire.CodeInternal, ae.Code)
	require.Equal(t, 502, ae.Status)
	require.NotNil(t, ae.Details)
}

func TestDaemonNotRunning(t *testing.T) {
	t.Parallel()
	t.Run("missing socket", func(t *testing.T) {
		t.Parallel()
		c := client.New(testutil.SocketPath(t))
		_, err := c.Status(context.Background())
		require.ErrorIs(t, err, client.ErrDaemonNotRunning)
		_, err = c.Events(context.Background())
		require.ErrorIs(t, err, client.ErrDaemonNotRunning)
	})
	t.Run("stale socket", func(t *testing.T) {
		t.Parallel()
		path := testutil.SocketPath(t)
		ln, err := net.Listen("unix", path)
		require.NoError(t, err)
		ln.(*net.UnixListener).SetUnlinkOnClose(false)
		require.NoError(t, ln.Close())
		_, err = os.Stat(path)
		require.NoError(t, err, "socket file should remain")
		_, err = client.New(path).Status(context.Background())
		require.ErrorIs(t, err, client.ErrDaemonNotRunning)
	})
}

func TestContextCancelled(t *testing.T) {
	t.Parallel()
	c, _ := recorder(t, 200, `{}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.Status(ctx)
	require.ErrorIs(t, err, context.Canceled)
}

func TestEventsOverSocket(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	c := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/events", r.URL.Path)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "id: 1\nevent: state_changed\ndata: {\"state\":\"off\"}\n\n: ping\n\n")
		w.(http.Flusher).Flush()
		<-release
		io.WriteString(w, "id: 2\nevent: projects_changed\ndata: {}\n\n")
	}))
	stream, err := c.Events(context.Background())
	require.NoError(t, err)
	defer stream.Close()

	ev, err := stream.Next()
	require.NoError(t, err)
	require.Equal(t, int64(1), ev.ID)
	require.Equal(t, wire.EventStateChanged, ev.Name)
	var st wire.Status
	require.NoError(t, json.Unmarshal(ev.Data, &st))
	require.Equal(t, wire.StateOff, st.State)

	close(release)
	ev, err = stream.Next()
	require.NoError(t, err)
	require.Equal(t, wire.Event{ID: 2, Name: wire.EventProjectsChanged, Data: json.RawMessage(`{}`)}, ev)

	_, err = stream.Next()
	require.ErrorIs(t, err, io.EOF)
}

func TestEventsErrorResponse(t *testing.T) {
	t.Parallel()
	c, _ := recorder(t, 503, `{"error": {"code": "unavailable", "message": "no", "details": {}}}`)
	_, err := c.Events(context.Background())
	require.True(t, client.IsCode(err, wire.CodeUnavailable))
}

func TestEventsCancel(t *testing.T) {
	t.Parallel()
	c := serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := c.Events(ctx)
	require.NoError(t, err)
	errc := make(chan error, 1)
	go func() {
		_, err := stream.Next()
		errc <- err
	}()
	cancel()
	err = <-errc
	require.Error(t, err)
	require.True(t, errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "canceled"), err)
}
