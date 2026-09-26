// Package store is the SQLite database: the connection, migrations, local state,
// and the repositories. The daemon is its only user; no client opens the file.
// The schema and its rules are owned by docs/03-data-model.md.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/model"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

//go:embed migrations/*.sql
var embedded embed.FS

// DatabaseFile is the database's file name inside the data directory.
const DatabaseFile = "gwen.db"

// DB is the process's database handle. It is safe for concurrent use.
type DB struct {
	sql           *sql.DB
	clock         clock.Clock
	dataDir       string
	deviceID      string
	schemaVersion int64
}

// Querier is satisfied by *sql.DB and *sql.Tx, so that a query helper can run
// inside or outside a transaction.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Open opens or creates <dataDir>/gwen.db, applies pending migrations after
// backing the database up, and generates the device_id on first start. clk is
// the source of every envelope timestamp the store writes.
func Open(ctx context.Context, dataDir string, clk clock.Clock) (*DB, error) {
	migrations, err := fs.Sub(embedded, "migrations")
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	return open(ctx, dataDir, clk, migrations)
}

func open(ctx context.Context, dataDir string, clk clock.Clock, migrations fs.FS) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(dataDir, DatabaseFile)
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)&_txlock=immediate"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	sqlDB.SetMaxOpenConns(4)

	db := &DB{sql: sqlDB, clock: clk, dataDir: dataDir}
	if err := db.init(ctx, migrations); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	return db, nil
}

func (db *DB) init(ctx context.Context, migrations fs.FS) error {
	var mode string
	if err := db.sql.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&mode); err != nil {
		return fmt.Errorf("read journal mode: %w", err)
	}
	if mode != "wal" {
		return fmt.Errorf("journal mode is %q, want wal", mode)
	}
	version, err := migrate(ctx, db.sql, db.dataDir, migrations)
	if err != nil {
		return err
	}
	db.schemaVersion = version
	id, err := bootstrapDeviceID(ctx, db.sql, db.clock.Now())
	if err != nil {
		return err
	}
	db.deviceID = id
	return nil
}

// SQL returns the underlying pool for repositories and read queries.
func (db *DB) SQL() *sql.DB { return db.sql }

// Close closes the pool.
func (db *DB) Close() error { return db.sql.Close() }

// DeviceID is this device's id, written into device_id on every synced write.
func (db *DB) DeviceID() string { return db.deviceID }

// Now is the store's clock reading, used for updated_at and similar stamps.
// Instants are truncated to the millisecond, as they are stored.
func (db *DB) Now() time.Time { return db.clock.Now().Truncate(time.Millisecond) }

// SchemaVersion is the migration version the database is at.
func (db *DB) SchemaVersion() int64 { return db.schemaVersion }

// InTx runs fn in one transaction, which takes the write lock at BEGIN. It
// commits when fn returns nil and rolls back otherwise, including when fn
// panics, in which case the panic continues after the rollback.
func (db *DB) InTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := db.sql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err != nil {
				err = errors.Join(err, fmt.Errorf("roll back: %w", rbErr))
			}
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

func bootstrapDeviceID(ctx context.Context, db *sql.DB, now time.Time) (string, error) {
	const qInsertDeviceID = `INSERT INTO local_state (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO NOTHING`
	if _, err := db.ExecContext(ctx, qInsertDeviceID, KeyDeviceID, model.NewID(), now.UnixMilli()); err != nil {
		return "", fmt.Errorf("bootstrap device id: %w", err)
	}
	id, err := GetLocal(ctx, db, KeyDeviceID)
	if err != nil {
		return "", fmt.Errorf("bootstrap device id: %w", err)
	}
	return id, nil
}
