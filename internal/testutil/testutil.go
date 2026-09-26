// Package testutil holds fixtures shared by tests in several packages. It is
// imported only from _test.go files.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/store"
)

// Day0 is the reference date of the golden scenarios in docs/11-testing.md.
const Day0 = "2026-09-15"

// T0 is the reference instant: Day0 at 09:00 UTC.
var T0 = time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)

// At returns the instant at hms on Day0 in UTC. hms is "HH:MM", "HH:MM:SS", or
// "HH:MM:SS.mmm", the notation of the golden scenarios.
func At(hms string) time.Time { return AtIn(time.UTC, Day0, hms) }

// AtOn returns the instant at hms on day in UTC.
func AtOn(day, hms string) time.Time { return AtIn(time.UTC, day, hms) }

// AtIn returns the instant at which the wall clock in loc reads hms on day. A
// malformed literal is a bug in the test and panics.
func AtIn(loc *time.Location, day, hms string) time.Time {
	for _, layout := range []string{"15:04", "15:04:05", "15:04:05.000"} {
		if t, err := time.ParseInLocation(time.DateOnly+" "+layout, day+" "+hms, loc); err == nil {
			return t
		}
	}
	panic("testutil: malformed day or time " + day + " " + hms)
}

// NewDB returns a migrated store in a fresh temporary directory, closed when the
// test ends. Its clock is a fake reading T0.
func NewDB(t testing.TB) *store.DB {
	t.Helper()
	return NewDBWithClock(t, clock.NewFake(T0))
}

// NewDBWithClock is NewDB with the given clock.
func NewDBWithClock(t testing.TB, clk clock.Clock) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), t.TempDir(), clk)
	if err != nil {
		t.Fatalf("testutil: open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// SocketPath returns a path for a Unix socket in a fresh directory removed when
// the test ends. It is short: a socket path may not exceed 108 bytes, which a
// path under t.TempDir() can.
func SocketPath(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gwen")
	if err != nil {
		t.Fatalf("testutil: socket directory: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "gwend.sock")
}

// Ptr returns a pointer to a copy of v, for optional fields in literals.
func Ptr[T any](v T) *T { return &v }
