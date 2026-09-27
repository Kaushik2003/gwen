package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// Instants are Unix milliseconds in the database and UTC time.Time in Go.

func timeOf(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

func nullMillis(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

func timePtr(n sql.NullInt64) *time.Time {
	if !n.Valid {
		return nil
	}
	t := timeOf(n.Int64)
	return &t
}

func nullString(s *string) sql.NullString {
	if s == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *s, Valid: true}
}

func stringPtr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	s := n.String
	return &s
}

// scanner is *sql.Row or *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

// envelopeCols ends the column list of every synced table.
const envelopeCols = `created_at, updated_at, deleted_at, device_id, rev`

type envelopeScan struct {
	created, updated int64
	deleted          sql.NullInt64
	device           string
	rev              int64
}

func (e *envelopeScan) dest() []any {
	return []any{&e.created, &e.updated, &e.deleted, &e.device, &e.rev}
}

func (e *envelopeScan) envelope() model.Envelope {
	return model.Envelope{
		CreatedAt: timeOf(e.created),
		UpdatedAt: timeOf(e.updated),
		DeletedAt: timePtr(e.deleted),
		DeviceID:  e.device,
		Rev:       e.rev,
	}
}

// notFound maps sql.ErrNoRows to a user-facing ErrNotFound.
func notFound(err error, what, id string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return Fail(ErrNotFound, "no %s %s", what, id)
	}
	return err
}

// checkRev enforces optimistic concurrency on PATCH: a given rev must match.
func checkRev(given *int64, stored int64) error {
	if given != nil && *given != stored {
		return FailField(ErrConflict, "rev", "changed elsewhere")
	}
	return nil
}

// validDay reports whether s is a real calendar date in YYYY-MM-DD form.
func validDay(s string) bool {
	t, err := time.Parse(model.DayLayout, s)
	return err == nil && t.Format(model.DayLayout) == s
}

// validColor normalizes a "#rrggbb" colour to lower case.
func validColor(s string) (string, bool) {
	if len(s) != 7 || s[0] != '#' {
		return "", false
	}
	s = strings.ToLower(s)
	for _, r := range s[1:] {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return "", false
		}
	}
	return s, true
}

// placeholders returns "?, ?, ?" for n parameters.
func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("?, ", n-1) + "?"
}
