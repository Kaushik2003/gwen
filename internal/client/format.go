package client

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Formatting shared by every client (docs/08-clients.md#shared-behaviour). The
// GUI mirrors these in TypeScript.

// FormatDuration renders d as "7h 32m", "8h", "45m", or "30s" below one
// minute. Seconds are dropped from a minute up, and negative durations render
// as "0s".
func FormatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	h, m := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh %dm", h, m)
	}
}

// FormatMillis is FormatDuration for a wire *_ms value.
func FormatMillis(ms int64) string { return FormatDuration(time.Duration(ms) * time.Millisecond) }

// FormatTime renders t as 24-hour "HH:MM" in t's location.
func FormatTime(t time.Time) string { return t.Format("15:04") }

// FormatDay renders t's date as "YYYY-MM-DD" in t's location.
func FormatDay(t time.Time) string { return t.Format(time.DateOnly) }

// ShortIDLen is the length of a short id and the shortest suffix ResolveID accepts.
const ShortIDLen = 8

// ShortID returns the last 8 characters of an id. UUIDv7 prefixes are
// timestamps and collide; suffixes do not.
func ShortID(id string) string {
	if len(id) <= ShortIDLen {
		return id
	}
	return id[len(id)-ShortIDLen:]
}

// Errors from ResolveID.
var (
	ErrAmbiguousID = errors.New("ambiguous id")
	ErrUnknownID   = errors.New("unknown id")
)

// ResolveID resolves a full id, or a suffix of at least 8 characters, against
// the ids of the relevant list. Matching ignores case.
func ResolveID(input string, ids []string) (string, error) {
	in := strings.ToLower(strings.TrimSpace(input))
	for _, id := range ids {
		if strings.ToLower(id) == in {
			return id, nil
		}
	}
	if len(in) < ShortIDLen {
		return "", fmt.Errorf("%w %q: give the full id or at least %d characters", ErrUnknownID, input, ShortIDLen)
	}
	var match string
	for _, id := range ids {
		if strings.HasSuffix(strings.ToLower(id), in) {
			if match != "" {
				return "", fmt.Errorf("%w %q", ErrAmbiguousID, input)
			}
			match = id
		}
	}
	if match == "" {
		return "", fmt.Errorf("%w %q", ErrUnknownID, input)
	}
	return match, nil
}
