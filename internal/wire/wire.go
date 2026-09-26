// Package wire holds the JSON types of the local API in docs/04-api-contract.md,
// shared by internal/api and internal/client. Each object and request body is
// one struct named as in the contract.
//
// Field names and units are the column names and units of the data model:
// instants are int64 Unix milliseconds (*_at), dates are "YYYY-MM-DD", and
// durations computed at request time are milliseconds (*_ms). Nullable fields
// are pointers and are always sent, as null when unset. Responses never carry a
// nil slice: an empty list is sent as [].
package wire

import (
	"bytes"
	"encoding/json"
	"time"
)

// Millis converts an instant to its wire form.
func Millis(t time.Time) int64 { return t.UnixMilli() }

// Time converts a wire instant to a time.Time in the local zone.
func Time(ms int64) time.Time { return time.UnixMilli(ms) }

// MillisPtr converts a nullable instant to its wire form.
func MillisPtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	ms := t.UnixMilli()
	return &ms
}

// TimePtr converts a nullable wire instant to a *time.Time.
func TimePtr(ms *int64) *time.Time {
	if ms == nil {
		return nil
	}
	t := time.UnixMilli(*ms)
	return &t
}

// Optional is a PATCH field that distinguishes absent, null, and a value, for
// nullable columns: an absent field is left unchanged and null clears it
// (docs/04-api-contract.md#conventions). Tag fields `json:"name,omitzero"` so
// an unset Optional is not sent, and give them a ts_type tag so the GUI's
// generated TypeScript describes the JSON rather than this struct.
type Optional[T any] struct {
	Set   bool // the field was present
	Null  bool // present and null
	Value T    // the value when Set and not Null
}

// Some returns an Optional holding v.
func Some[T any](v T) Optional[T] { return Optional[T]{Set: true, Value: v} }

// Null returns an Optional that clears the field.
func Null[T any]() Optional[T] { return Optional[T]{Set: true, Null: true} }

// FromPtr returns Some(*p), or Null when p is nil.
func FromPtr[T any](p *T) Optional[T] {
	if p == nil {
		return Null[T]()
	}
	return Some(*p)
}

// IsZero reports whether the field is absent; encoding/json's omitzero uses it.
func (o Optional[T]) IsZero() bool { return !o.Set }

// Ptr returns the new value of a Set field: nil for null, else a pointer to a
// copy of Value.
func (o Optional[T]) Ptr() *T {
	if !o.Set || o.Null {
		return nil
	}
	v := o.Value
	return &v
}

// MarshalJSON encodes null or the value.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	if !o.Set || o.Null {
		return []byte("null"), nil
	}
	return json.Marshal(o.Value)
}

// UnmarshalJSON records that the field was present, and whether it was null.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	var zero T
	o.Set, o.Value = true, zero
	o.Null = bytes.Equal(bytes.TrimSpace(b), []byte("null"))
	if o.Null {
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}
