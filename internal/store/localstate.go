package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Local state keys (docs/03-data-model.md#local-state-keys). The list is
// closed: SetLocal rejects any other key.
const (
	KeyDeviceID          = "device_id"
	KeyHeartbeatAt       = "heartbeat_at"
	KeyShutdownAt        = "shutdown_at"
	KeyGcalCalendarID    = "gcal_calendar_id"
	KeyGcalSyncToken     = "gcal_sync_token"
	KeySyncPushWatermark = "sync_push_watermark"
	KeySyncPullCursor    = "sync_pull_cursor"
)

func knownKey(key string) bool {
	switch key {
	case KeyDeviceID, KeyHeartbeatAt, KeyShutdownAt, KeyGcalCalendarID, KeyGcalSyncToken,
		KeySyncPushWatermark, KeySyncPullCursor:
		return true
	}
	return false
}

// GetLocal returns the value of a local_state key, or ErrNotFound.
func GetLocal(ctx context.Context, q Querier, key string) (string, error) {
	const qGetLocal = `SELECT value FROM local_state WHERE key = ?`
	var v string
	err := q.QueryRowContext(ctx, qGetLocal, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("local state %s: %w", key, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("local state %s: %w", key, err)
	}
	return v, nil
}

// SetLocal inserts or replaces a local_state key. device_id is written only by
// the store's own bootstrap and cannot be set.
func SetLocal(ctx context.Context, q Querier, key, value string, now time.Time) error {
	if !knownKey(key) || key == KeyDeviceID {
		return fmt.Errorf("set local state %s: %w", key, ErrInvalid)
	}
	const qSetLocal = `INSERT INTO local_state (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`
	if _, err := q.ExecContext(ctx, qSetLocal, key, value, now.UnixMilli()); err != nil {
		return fmt.Errorf("set local state %s: %w", key, classify(err))
	}
	return nil
}

// DeleteLocal removes a local_state key; removing an absent key is not an error.
func DeleteLocal(ctx context.Context, q Querier, key string) error {
	if key == KeyDeviceID {
		return fmt.Errorf("delete local state %s: %w", key, ErrInvalid)
	}
	const qDeleteLocal = `DELETE FROM local_state WHERE key = ?`
	if _, err := q.ExecContext(ctx, qDeleteLocal, key); err != nil {
		return fmt.Errorf("delete local state %s: %w", key, err)
	}
	return nil
}

// GetLocalTime reads a key holding Unix milliseconds, or ErrNotFound.
func GetLocalTime(ctx context.Context, q Querier, key string) (time.Time, error) {
	v, err := GetLocal(ctx, q, key)
	if err != nil {
		return time.Time{}, err
	}
	ms, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("local state %s: %q is not Unix milliseconds: %w", key, v, ErrInvalid)
	}
	return time.UnixMilli(ms), nil
}

// SetLocalTime stores t as Unix milliseconds under key.
func SetLocalTime(ctx context.Context, q Querier, key string, t, now time.Time) error {
	return SetLocal(ctx, q, key, strconv.FormatInt(t.UnixMilli(), 10), now)
}
