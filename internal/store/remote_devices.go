package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// RemoteDevice is a paired phone (00011_remote.sql). The token its cookie
// carries is never stored, only its SHA-256.
type RemoteDevice struct {
	ID         string
	Name       string
	CreatedAt  time.Time
	LastSeenAt *time.Time
}

// RemoteDeviceRepo keeps the paired phones. Revoking deletes the row.
type RemoteDeviceRepo interface {
	// Create pairs a phone whose cookie token hashes to tokenHash.
	Create(ctx context.Context, name, tokenHash string) (RemoteDevice, error)
	List(ctx context.Context) ([]RemoteDevice, error)
	// ByTokenHash returns the phone holding that token, or ErrNotFound.
	ByTokenHash(ctx context.Context, tokenHash string) (RemoteDevice, error)
	// Seen records that the phone called now.
	Seen(ctx context.Context, id string, at time.Time) error
	// Delete revokes a phone, or returns ErrNotFound.
	Delete(ctx context.Context, id string) error
}

type remoteDeviceRepo struct{ db *DB }

// NewRemoteDeviceRepo returns the SQLite RemoteDeviceRepo.
func NewRemoteDeviceRepo(db *DB) RemoteDeviceRepo { return remoteDeviceRepo{db} }

// MaxDeviceName is the longest name of a paired phone, in characters.
const MaxDeviceName = 60

const remoteDeviceCols = `id, name, created_at, last_seen_at`

func scanRemoteDevice(s scanner) (RemoteDevice, error) {
	var (
		d       RemoteDevice
		created int64
		seen    sql.NullInt64
	)
	if err := s.Scan(&d.ID, &d.Name, &created, &seen); err != nil {
		return RemoteDevice{}, err
	}
	d.CreatedAt, d.LastSeenAt = timeOf(created), timePtr(seen)
	return d, nil
}

func (r remoteDeviceRepo) Create(ctx context.Context, name, tokenHash string) (RemoteDevice, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Phone"
	}
	if runes := []rune(name); len(runes) > MaxDeviceName {
		name = string(runes[:MaxDeviceName])
	}
	d := RemoteDevice{ID: model.NewID(), Name: name, CreatedAt: r.db.Now()}
	const qCreateRemoteDevice = `INSERT INTO remote_devices (id, name, token_hash, created_at) VALUES (?, ?, ?, ?)`
	if _, err := r.db.sql.ExecContext(ctx, qCreateRemoteDevice, d.ID, d.Name, tokenHash, d.CreatedAt.UnixMilli()); err != nil {
		return RemoteDevice{}, fmt.Errorf("pair device: %w", classify(err))
	}
	return d, nil
}

func (r remoteDeviceRepo) List(ctx context.Context) ([]RemoteDevice, error) {
	const qListRemoteDevices = `SELECT ` + remoteDeviceCols + ` FROM remote_devices WHERE revoked_at IS NULL ORDER BY created_at`
	rows, err := r.db.sql.QueryContext(ctx, qListRemoteDevices)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	defer rows.Close()
	out := []RemoteDevice{}
	for rows.Next() {
		d, err := scanRemoteDevice(rows)
		if err != nil {
			return nil, fmt.Errorf("list devices: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r remoteDeviceRepo) ByTokenHash(ctx context.Context, tokenHash string) (RemoteDevice, error) {
	const qRemoteDeviceByToken = `SELECT ` + remoteDeviceCols + ` FROM remote_devices WHERE token_hash = ? AND revoked_at IS NULL`
	d, err := scanRemoteDevice(r.db.sql.QueryRowContext(ctx, qRemoteDeviceByToken, tokenHash))
	if err != nil {
		return RemoteDevice{}, fmt.Errorf("device by token: %w", notFound(err, "paired device", ""))
	}
	return d, nil
}

func (r remoteDeviceRepo) Seen(ctx context.Context, id string, at time.Time) error {
	const qRemoteDeviceSeen = `UPDATE remote_devices SET last_seen_at = ? WHERE id = ?`
	if _, err := r.db.sql.ExecContext(ctx, qRemoteDeviceSeen, at.UnixMilli(), id); err != nil {
		return fmt.Errorf("device seen: %w", err)
	}
	return nil
}

func (r remoteDeviceRepo) Delete(ctx context.Context, id string) error {
	const qDeleteRemoteDevice = `DELETE FROM remote_devices WHERE id = ?`
	res, err := r.db.sql.ExecContext(ctx, qDeleteRemoteDevice, id)
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Fail(ErrNotFound, "no paired device %s", id)
	}
	return nil
}
