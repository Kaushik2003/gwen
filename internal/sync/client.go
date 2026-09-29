package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	gosync "sync"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/store"
)

// Client timing (docs/07-integrations.md#client).
const (
	PushBatch = 500
	Overlap   = 5 * time.Minute
)

// ErrUnavailable is matched by the error of every call made while no hub is
// configured; its message names the setup step.
var ErrUnavailable = errors.New("sync unavailable")

type unavailable struct{ msg string }

func (e *unavailable) Error() string        { return e.msg }
func (e *unavailable) Is(target error) bool { return target == ErrUnavailable }

// Status is what GET /v1/sync/status reports.
type Status struct {
	Configured bool
	LastPushAt *time.Time
	LastPullAt *time.Time
	LastError  string
}

// ClientOptions configure a Client.
type ClientOptions struct {
	DB      *store.DB
	Repo    store.SyncRepo
	Clock   clock.Clock
	CredDir string
	// Config is the configuration in effect; it is read at every use.
	Config     func() config.Config
	HTTPClient *http.Client // nil means http.DefaultClient
	// OnApplied sees every pulled page applied, for SSE invalidations.
	OnApplied func(store.Applied)
	// OnAttempt runs after every sync attempt.
	OnAttempt func()
}

// Client pushes this device's changes to the hub and pulls everyone else's.
// Syncs never overlap.
type Client struct {
	o      ClientOptions
	syncMu gosync.Mutex

	mu       gosync.Mutex
	lastPush *time.Time
	lastPull *time.Time
	lastErr  string
}

// NewClient returns a Client; call Run to sync in the background.
func NewClient(o ClientOptions) *Client {
	if o.HTTPClient == nil {
		o.HTTPClient = http.DefaultClient
	}
	if o.OnApplied == nil {
		o.OnApplied = func(store.Applied) {}
	}
	if o.OnAttempt == nil {
		o.OnAttempt = func() {}
	}
	return &Client{o: o}
}

// hub returns the hub URL and token, or an unavailable error.
func (c *Client) hub() (string, string, error) {
	u := strings.TrimRight(c.o.Config().Sync.HubURL, "/")
	if u == "" {
		return "", "", &unavailable{"no sync hub is configured; run gwen setup sync"}
	}
	tok, err := config.ReadCredentialLine(c.o.CredDir, config.CredSyncToken)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && tok == "") {
		return "", "", &unavailable{"no sync token; run gwen setup sync"}
	}
	return u, tok, err
}

// Available is nil when a hub and its token are configured.
func (c *Client) Available() error {
	_, _, err := c.hub()
	return err
}

// Status reports the client's state.
func (c *Client) Status(context.Context) Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Status{Configured: c.Available() == nil, LastPushAt: c.lastPush, LastPullAt: c.lastPull, LastError: c.lastErr}
}

// Run syncs every sync.interval until ctx ends, while a hub is configured.
func (c *Client) Run(ctx context.Context) {
	t := c.o.Clock.NewTimer(c.o.Config().Sync.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C():
		}
		if c.Available() == nil {
			c.Sync(ctx)
		}
		t.Reset(c.o.Config().Sync.Interval)
	}
}

// Sync pushes and then pulls, and returns the resulting status. A failure is
// recorded in the status; only a missing configuration is an error.
func (c *Client) Sync(ctx context.Context) (Status, error) {
	base, token, err := c.hub()
	if err != nil {
		return Status{}, err
	}
	c.syncMu.Lock()
	err = c.push(ctx, base, token)
	if err == nil {
		err = c.pull(ctx, base, token)
	}
	c.syncMu.Unlock()
	c.mu.Lock()
	c.lastErr = ""
	if err != nil {
		c.lastErr = err.Error()
		slog.Warn("sync failed", "err", err)
	}
	c.mu.Unlock()
	c.o.OnAttempt()
	return c.Status(ctx), nil
}

// push sends every row changed since the watermark, less the overlap, in
// batches, and advances the watermark once all have gone.
func (c *Client) push(ctx context.Context, base, token string) error {
	db := c.o.DB.SQL()
	since := time.UnixMilli(0)
	wm, err := store.GetLocalTime(ctx, db, store.KeySyncPushWatermark)
	switch {
	case err == nil:
		since = wm.Add(-Overlap)
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	changed, err := c.o.Repo.Changed(ctx, since)
	if err != nil {
		return err
	}
	var newest int64
	for start := 0; start < len(changed); start += PushBatch {
		batch := store.Rows{}
		for _, tr := range changed[start:min(start+PushBatch, len(changed))] {
			batch[tr.Table] = append(batch[tr.Table], tr.Row)
			newest = max(newest, tr.Row["updated_at"].(int64))
		}
		rows, err := json.Marshal(batch)
		if err != nil {
			return err
		}
		var res PushResponse
		body := PushRequest{DeviceID: c.o.DB.DeviceID(), Rows: rows}
		if err := c.call(ctx, http.MethodPost, base+"/sync/v1/push", token, body, &res); err != nil {
			return fmt.Errorf("push: %w", err)
		}
		if res.Ignored > 0 {
			slog.Warn("the hub ignored pushed rows", "ignored", res.Ignored)
		}
	}
	now := c.o.Clock.Now()
	if newest > 0 {
		if err := store.SetLocalTime(ctx, db, store.KeySyncPushWatermark, time.UnixMilli(newest), now); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.lastPush = &now
	c.mu.Unlock()
	return nil
}

// pull applies pages from the hub until none remain, storing the cursor
// after each.
func (c *Client) pull(ctx context.Context, base, token string) error {
	db := c.o.DB.SQL()
	cursor, err := store.GetLocal(ctx, db, store.KeySyncPullCursor)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	for {
		q := url.Values{"device_id": {c.o.DB.DeviceID()}, "cursor": {cursor}, "limit": {strconv.Itoa(PullLimit)}}
		var raw struct {
			Rows   json.RawMessage `json:"rows"`
			Cursor string          `json:"cursor"`
			More   bool            `json:"more"`
		}
		if err := c.call(ctx, http.MethodGet, base+"/sync/v1/pull?"+q.Encode(), token, nil, &raw); err != nil {
			return fmt.Errorf("pull: %w", err)
		}
		rows, err := store.DecodeRows(raw.Rows)
		if err != nil {
			return fmt.Errorf("pull: %w", err)
		}
		if rows.Count() > 0 {
			res, err := c.o.Repo.Apply(ctx, rows, false)
			if err != nil {
				return fmt.Errorf("apply pulled rows: %w", err)
			}
			if res.Ignored > 0 {
				slog.Warn("pulled rows ignored", "ignored", res.Ignored)
			}
			c.o.OnApplied(res)
		}
		cursor = raw.Cursor
		if err := store.SetLocal(ctx, db, store.KeySyncPullCursor, cursor, c.o.Clock.Now()); err != nil {
			return err
		}
		if !raw.More {
			break
		}
	}
	now := c.o.Clock.Now()
	c.mu.Lock()
	c.lastPull = &now
	c.mu.Unlock()
	return nil
}

// call sends one request to the hub and decodes a 2xx reply into out.
func (c *Client) call(ctx context.Context, method, u, token string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.o.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		if json.Unmarshal(b, &e) == nil && e.Error.Message != "" {
			return fmt.Errorf("the hub answered %d: %s", resp.StatusCode, e.Error.Message)
		}
		return fmt.Errorf("the hub answered %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
