package store

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/kzark/gwen/internal/model"
)

// timelineVersion is the schema version that added waits_on, code
// sessions' task, and agent spans (00018_timeline.sql).
const timelineVersion = 18

// MaxWaits is how many tasks one task can wait on.
const MaxWaits = 20

func waitsJSON(ids []string) string {
	b, _ := json.Marshal(append([]string{}, ids...))
	return string(b)
}

// validWaits checks and normalizes what t waits on before a write: each id
// a live task other than t (ids of tasks that are gone are dropped), no
// repeats, and no loop back to t through what those tasks wait on in turn.
func validWaits(ctx context.Context, q Querier, t *model.Task) error {
	clean := make([]string, 0, len(t.WaitsOn))
	for _, id := range t.WaitsOn {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(clean, id) {
			continue
		}
		if id == t.ID {
			return FailField(ErrInvalid, "waits_on", "a task cannot wait on itself")
		}
		other, err := getTask(ctx, q, id)
		if err != nil {
			// Deleted since (or never synced here): it blocks nothing, so it
			// goes rather than refuse the whole write.
			continue
		}
		if other.RRule != nil {
			return FailField(ErrInvalid, "waits_on", "a task cannot wait on a repeating task; pick one of its days")
		}
		clean = append(clean, id)
	}
	if len(clean) > MaxWaits {
		return FailField(ErrInvalid, "waits_on", "a task can wait on at most %d tasks", MaxWaits)
	}
	t.WaitsOn = clean
	if t.ID == "" || len(clean) == 0 {
		return nil
	}
	// Walk everything the new list leads to; reaching t again is a loop.
	seen := map[string]bool{}
	next := slices.Clone(clean)
	for len(next) > 0 {
		id := next[len(next)-1]
		next = next[:len(next)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		var raw string
		const qWaits = `SELECT waits_on FROM tasks WHERE id = ? AND deleted_at IS NULL`
		if err := q.QueryRowContext(ctx, qWaits, id).Scan(&raw); err != nil {
			continue // gone: it leads nowhere
		}
		var ids []string
		_ = json.Unmarshal([]byte(raw), &ids)
		for _, w := range ids {
			if w == t.ID {
				other, _ := getTask(ctx, q, id)
				return FailField(ErrConflict, "waits_on", "that makes a loop: %q already waits on this task", other.Title)
			}
			next = append(next, w)
		}
	}
	return nil
}
