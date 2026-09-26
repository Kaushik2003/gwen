// Package client is the Go client for the daemon's socket API
// (docs/04-api-contract.md), shared by the CLI, the tray, and the GUI host.
// Clients never open the database; everything goes through API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"syscall"

	"github.com/kzark/gwen/internal/wire"
)

// API has one method per endpoint. Methods return *APIError for an error
// response and ErrDaemonNotRunning when nothing is listening on the socket.
type API interface {
	Health(ctx context.Context) (*wire.Health, error)
	Status(ctx context.Context) (*wire.Status, error)

	ClockIn(ctx context.Context, req wire.ClockInRequest) (*wire.Status, error)
	ClockOut(ctx context.Context) (*wire.Status, error)
	BreakStart(ctx context.Context) (*wire.Status, error)
	BreakEnd(ctx context.Context) (*wire.Status, error)
	Switch(ctx context.Context, req wire.SwitchRequest) (*wire.Status, error)
	Snooze(ctx context.Context) (*wire.Status, error)

	ListDays(ctx context.Context, from, to string) (*wire.DayList, error)
	GetDay(ctx context.Context, day string) (*wire.DayDetail, error)
	PatchDay(ctx context.Context, day string, req wire.PatchDayRequest) (*wire.WorkDay, error)
	CreateSegment(ctx context.Context, req wire.CreateSegmentRequest) (*wire.Segment, error)
	PatchSegment(ctx context.Context, id string, req wire.PatchSegmentRequest) (*wire.Segment, error)
	SplitSegment(ctx context.Context, id string, req wire.SplitSegmentRequest) (*wire.SegmentList, error)
	DeleteSegment(ctx context.Context, id string) error

	// ListProjects takes wire.ArchivedFalse, ArchivedTrue, or ArchivedAll; ""
	// means the server default, false.
	ListProjects(ctx context.Context, archived string) (*wire.ProjectList, error)
	CreateProject(ctx context.Context, req wire.CreateProjectRequest) (*wire.Project, error)
	GetProject(ctx context.Context, id string) (*wire.Project, error)
	PatchProject(ctx context.Context, id string, req wire.PatchProjectRequest) (*wire.Project, error)
	DeleteProject(ctx context.Context, id string) error

	ListTasks(ctx context.Context, q wire.TaskQuery) (*wire.TaskList, error)
	CreateTask(ctx context.Context, req wire.CreateTaskRequest) (*wire.Task, error)
	GetTask(ctx context.Context, id string) (*wire.Task, error)
	PatchTask(ctx context.Context, id string, req wire.PatchTaskRequest) (*wire.Task, error)
	CompleteTask(ctx context.Context, id string, req wire.CompleteTaskRequest) (*wire.Task, error)
	ReopenTask(ctx context.Context, id string) (*wire.Task, error)
	DeleteTask(ctx context.Context, id string) error

	StatsSummary(ctx context.Context, from, to string) (*wire.StatsSummary, error)
	StatsHeatmap(ctx context.Context, year int) (*wire.Heatmap, error)

	GetConfig(ctx context.Context) (*wire.Config, error)
	PatchConfig(ctx context.Context, patch wire.ConfigPatch) (*wire.Config, error)

	NotifyTest(ctx context.Context) (*wire.NotifyTestResult, error)

	// Events opens GET /v1/events. The stream does not reconnect; see Follower.
	Events(ctx context.Context) (EventStream, error)
}

// ErrDaemonNotRunning means the socket is missing or refuses connections.
var ErrDaemonNotRunning = errors.New("gwen isn't running")

// APIError is an error response from the daemon.
type APIError struct {
	Status  int            // HTTP status
	Code    string         // wire.Code*
	Message string         // lowercase sentence, safe to show
	Details map[string]any // never nil
}

func (e *APIError) Error() string { return e.Message }

// IsCode reports whether err is an *APIError with the given code.
func IsCode(err error, code string) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Code == code
}

// Client implements API over HTTP on the daemon's Unix socket.
type Client struct {
	socket string
	http   *http.Client
}

var _ API = (*Client)(nil)

// New returns a client for the daemon listening on socketPath. It does not
// connect until the first request.
func New(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
		DisableCompression: true,
		MaxIdleConns:       2,
	}
	return &Client{socket: socketPath, http: &http.Client{Transport: transport}}
}

// SocketPath returns the socket the client talks to.
func (c *Client) SocketPath() string { return c.socket }

// empty is the body of commands that take no parameters.
var empty = struct{}{}

func (c *Client) Health(ctx context.Context) (*wire.Health, error) {
	return call[wire.Health](ctx, c, http.MethodGet, "/v1/health", nil, nil)
}

func (c *Client) Status(ctx context.Context) (*wire.Status, error) {
	return call[wire.Status](ctx, c, http.MethodGet, "/v1/status", nil, nil)
}

func (c *Client) ClockIn(ctx context.Context, req wire.ClockInRequest) (*wire.Status, error) {
	return call[wire.Status](ctx, c, http.MethodPost, "/v1/day/clock-in", nil, req)
}

func (c *Client) ClockOut(ctx context.Context) (*wire.Status, error) {
	return call[wire.Status](ctx, c, http.MethodPost, "/v1/day/clock-out", nil, empty)
}

func (c *Client) BreakStart(ctx context.Context) (*wire.Status, error) {
	return call[wire.Status](ctx, c, http.MethodPost, "/v1/break/start", nil, empty)
}

func (c *Client) BreakEnd(ctx context.Context) (*wire.Status, error) {
	return call[wire.Status](ctx, c, http.MethodPost, "/v1/break/end", nil, empty)
}

func (c *Client) Switch(ctx context.Context, req wire.SwitchRequest) (*wire.Status, error) {
	return call[wire.Status](ctx, c, http.MethodPost, "/v1/session/switch", nil, req)
}

func (c *Client) Snooze(ctx context.Context) (*wire.Status, error) {
	return call[wire.Status](ctx, c, http.MethodPost, "/v1/nudge/snooze", nil, empty)
}

func (c *Client) ListDays(ctx context.Context, from, to string) (*wire.DayList, error) {
	return call[wire.DayList](ctx, c, http.MethodGet, "/v1/days", query("from", from, "to", to), nil)
}

func (c *Client) GetDay(ctx context.Context, day string) (*wire.DayDetail, error) {
	return call[wire.DayDetail](ctx, c, http.MethodGet, "/v1/days/"+url.PathEscape(day), nil, nil)
}

func (c *Client) PatchDay(ctx context.Context, day string, req wire.PatchDayRequest) (*wire.WorkDay, error) {
	return call[wire.WorkDay](ctx, c, http.MethodPatch, "/v1/days/"+url.PathEscape(day), nil, req)
}

func (c *Client) CreateSegment(ctx context.Context, req wire.CreateSegmentRequest) (*wire.Segment, error) {
	return call[wire.Segment](ctx, c, http.MethodPost, "/v1/segments", nil, req)
}

func (c *Client) PatchSegment(ctx context.Context, id string, req wire.PatchSegmentRequest) (*wire.Segment, error) {
	return call[wire.Segment](ctx, c, http.MethodPatch, "/v1/segments/"+url.PathEscape(id), nil, req)
}

func (c *Client) SplitSegment(ctx context.Context, id string, req wire.SplitSegmentRequest) (*wire.SegmentList, error) {
	return call[wire.SegmentList](ctx, c, http.MethodPost, "/v1/segments/"+url.PathEscape(id)+"/split", nil, req)
}

func (c *Client) DeleteSegment(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/segments/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) ListProjects(ctx context.Context, archived string) (*wire.ProjectList, error) {
	return call[wire.ProjectList](ctx, c, http.MethodGet, "/v1/projects", query("archived", archived), nil)
}

func (c *Client) CreateProject(ctx context.Context, req wire.CreateProjectRequest) (*wire.Project, error) {
	return call[wire.Project](ctx, c, http.MethodPost, "/v1/projects", nil, req)
}

func (c *Client) GetProject(ctx context.Context, id string) (*wire.Project, error) {
	return call[wire.Project](ctx, c, http.MethodGet, "/v1/projects/"+url.PathEscape(id), nil, nil)
}

func (c *Client) PatchProject(ctx context.Context, id string, req wire.PatchProjectRequest) (*wire.Project, error) {
	return call[wire.Project](ctx, c, http.MethodPatch, "/v1/projects/"+url.PathEscape(id), nil, req)
}

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/projects/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) ListTasks(ctx context.Context, q wire.TaskQuery) (*wire.TaskList, error) {
	params := query("project_id", q.ProjectID, "status", q.Status, "due_before", q.DueBefore)
	return call[wire.TaskList](ctx, c, http.MethodGet, "/v1/tasks", params, nil)
}

func (c *Client) CreateTask(ctx context.Context, req wire.CreateTaskRequest) (*wire.Task, error) {
	return call[wire.Task](ctx, c, http.MethodPost, "/v1/tasks", nil, req)
}

func (c *Client) GetTask(ctx context.Context, id string) (*wire.Task, error) {
	return call[wire.Task](ctx, c, http.MethodGet, "/v1/tasks/"+url.PathEscape(id), nil, nil)
}

func (c *Client) PatchTask(ctx context.Context, id string, req wire.PatchTaskRequest) (*wire.Task, error) {
	return call[wire.Task](ctx, c, http.MethodPatch, "/v1/tasks/"+url.PathEscape(id), nil, req)
}

func (c *Client) CompleteTask(ctx context.Context, id string, req wire.CompleteTaskRequest) (*wire.Task, error) {
	return call[wire.Task](ctx, c, http.MethodPost, "/v1/tasks/"+url.PathEscape(id)+"/complete", nil, req)
}

func (c *Client) ReopenTask(ctx context.Context, id string) (*wire.Task, error) {
	return call[wire.Task](ctx, c, http.MethodPost, "/v1/tasks/"+url.PathEscape(id)+"/reopen", nil, empty)
}

func (c *Client) DeleteTask(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v1/tasks/"+url.PathEscape(id), nil, nil, nil)
}

func (c *Client) StatsSummary(ctx context.Context, from, to string) (*wire.StatsSummary, error) {
	return call[wire.StatsSummary](ctx, c, http.MethodGet, "/v1/stats/summary", query("from", from, "to", to), nil)
}

func (c *Client) StatsHeatmap(ctx context.Context, year int) (*wire.Heatmap, error) {
	return call[wire.Heatmap](ctx, c, http.MethodGet, "/v1/stats/heatmap", query("year", strconv.Itoa(year)), nil)
}

func (c *Client) GetConfig(ctx context.Context) (*wire.Config, error) {
	return call[wire.Config](ctx, c, http.MethodGet, "/v1/config", nil, nil)
}

func (c *Client) PatchConfig(ctx context.Context, patch wire.ConfigPatch) (*wire.Config, error) {
	return call[wire.Config](ctx, c, http.MethodPatch, "/v1/config", nil, patch)
}

func (c *Client) NotifyTest(ctx context.Context) (*wire.NotifyTestResult, error) {
	return call[wire.NotifyTestResult](ctx, c, http.MethodPost, "/v1/notify/test", nil, empty)
}

func (c *Client) Events(ctx context.Context) (EventStream, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://gwend/v1/events", nil)
	if err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, decodeError(resp)
	}
	return newSSEReader(resp.Body), nil
}

// query builds query parameters from name/value pairs, skipping empty values.
func query(pairs ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			v.Set(pairs[i], pairs[i+1])
		}
	}
	return v
}

func call[T any](ctx context.Context, c *Client, method, path string, q url.Values, body any) (*T, error) {
	var out T
	if err := c.do(ctx, method, path, q, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// do sends one request. A nil body sends none; a nil out expects 204.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, body, out any) error {
	u := "http://gwend" + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s %s: encode request: %w", method, path, err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.send(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return decodeError(resp)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", method, path, err)
	}
	return nil
}

func (c *Client) send(req *http.Request) (*http.Response, error) {
	resp, err := c.http.Do(req)
	if err == nil {
		return resp, nil
	}
	if errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
		return nil, ErrDaemonNotRunning
	}
	if ctxErr := req.Context().Err(); ctxErr != nil {
		return nil, ctxErr
	}
	return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, err)
}

// decodeError turns a non-2xx response into an *APIError.
func decodeError(resp *http.Response) error {
	var body wire.ErrorResponse
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(raw, &body); err != nil || body.Error.Code == "" {
		return &APIError{
			Status:  resp.StatusCode,
			Code:    wire.CodeInternal,
			Message: fmt.Sprintf("unexpected response: %s", resp.Status),
			Details: map[string]any{},
		}
	}
	if body.Error.Details == nil {
		body.Error.Details = map[string]any{}
	}
	return &APIError{
		Status:  resp.StatusCode,
		Code:    body.Error.Code,
		Message: body.Error.Message,
		Details: body.Error.Details,
	}
}
