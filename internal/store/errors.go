package store

import (
	"errors"
	"fmt"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Sentinel errors callers branch on. The API maps them to wire codes
// (docs/04-api-contract.md#errors); wrap them with context using %w.
var (
	// ErrNotFound: the row does not exist or is deleted.
	ErrNotFound = errors.New("not found")
	// ErrConflict: a rev mismatch, a uniqueness violation, or an invariant of
	// docs/03-data-model.md#invariants that is not rule 7.
	ErrConflict = errors.New("conflict")
	// ErrInvalid: a value the schema or invariant 7 rejects.
	ErrInvalid = errors.New("invalid")
)

// Error is a store failure with a message fit to show the user; the API uses
// Message as the error message and Field as details.field
// (docs/04-api-contract.md#errors). errors.Is matches its sentinel, and
// errors.As still finds it after further wrapping.
type Error struct {
	Err     error  // ErrNotFound, ErrConflict, or ErrInvalid
	Message string // a lowercase sentence, e.g. "segments may not overlap"
	Field   string // the offending request field, or ""
}

func (e *Error) Error() string { return e.Message }

func (e *Error) Unwrap() error { return e.Err }

// Fail returns an *Error for a sentinel with a formatted message.
func Fail(sentinel error, format string, args ...any) error {
	return &Error{Err: sentinel, Message: fmt.Sprintf(format, args...)}
}

// FailField is Fail naming the offending request field.
func FailField(sentinel error, field, format string, args ...any) error {
	return &Error{Err: sentinel, Message: fmt.Sprintf(format, args...), Field: field}
}

// classify maps a SQLite constraint violation to a sentinel, keeping the
// driver's message: UNIQUE and PRIMARY KEY become ErrConflict, CHECK and NOT
// NULL become ErrInvalid, and FOREIGN KEY becomes ErrNotFound. Any other error
// is returned unchanged.
func classify(err error) error {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return err
	}
	switch se.Code() {
	case sqlite3.SQLITE_CONSTRAINT_UNIQUE, sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY:
		return fmt.Errorf("%w: %v", ErrConflict, err)
	case sqlite3.SQLITE_CONSTRAINT_CHECK, sqlite3.SQLITE_CONSTRAINT_NOTNULL:
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	case sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY:
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	return err
}
