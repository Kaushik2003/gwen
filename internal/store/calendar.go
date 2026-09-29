package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// GcalLink ties a plan item to its Google Calendar event.
type GcalLink struct {
	PlanItemID string
	EventID    string
	ETag       string
	SyncedAt   time.Time
}

// BusyInterval is time another calendar is busy.
type BusyInterval struct {
	CalendarID string
	Start, End time.Time
}

// CalendarRepo keeps the Google Calendar caches, gcal_links and
// calendar_busy, and the plan-item reads and writes a sync needs
// (docs/07-integrations.md#google-calendar).
type CalendarRepo interface {
	Links(ctx context.Context) ([]GcalLink, error)
	PutLink(ctx context.Context, l GcalLink) error
	DeleteLink(ctx context.Context, planItemID string) error
	// ForgetCalendar clears every link, for a calendar created afresh.
	ForgetCalendar(ctx context.Context) error
	// Item returns a live plan item.
	Item(ctx context.Context, id string) (model.PlanItem, error)
	// Desired returns the live items on days from..to with status planned or
	// done, a start_at, and a live task: the events the calendar should hold.
	Desired(ctx context.Context, from, to string) ([]PlanEntry, error)
	// MoveItem sets an item's start, length, and day from its event, and pins it.
	MoveItem(ctx context.Context, id string, start time.Time, planned time.Duration, day string) (model.PlanItem, error)
	// SkipItem sets a planned item skipped; it reports whether it changed.
	SkipItem(ctx context.Context, id string) (model.PlanItem, bool, error)
	// ReplaceBusy replaces every busy interval overlapping [from, to).
	ReplaceBusy(ctx context.Context, from, to time.Time, busy []BusyInterval) error
}

type calendarRepo struct{ db *DB }

// NewCalendarRepo returns the SQLite CalendarRepo.
func NewCalendarRepo(db *DB) CalendarRepo { return calendarRepo{db} }

func (r calendarRepo) Links(ctx context.Context) ([]GcalLink, error) {
	const qLinks = `SELECT plan_item_id, event_id, etag, synced_at FROM gcal_links ORDER BY plan_item_id`
	rows, err := r.db.sql.QueryContext(ctx, qLinks)
	if err != nil {
		return nil, fmt.Errorf("calendar links: %w", err)
	}
	defer rows.Close()
	out := []GcalLink{}
	for rows.Next() {
		var l GcalLink
		var synced int64
		if err := rows.Scan(&l.PlanItemID, &l.EventID, &l.ETag, &synced); err != nil {
			return nil, fmt.Errorf("calendar links: %w", err)
		}
		l.SyncedAt = timeOf(synced)
		out = append(out, l)
	}
	return out, rows.Err()
}

func (r calendarRepo) PutLink(ctx context.Context, l GcalLink) error {
	const qPutLink = `INSERT INTO gcal_links (plan_item_id, event_id, etag, synced_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (plan_item_id) DO UPDATE SET event_id = excluded.event_id, etag = excluded.etag,
		synced_at = excluded.synced_at`
	if _, err := r.db.sql.ExecContext(ctx, qPutLink, l.PlanItemID, l.EventID, l.ETag, l.SyncedAt.UnixMilli()); err != nil {
		return fmt.Errorf("put calendar link %s: %w", l.PlanItemID, classify(err))
	}
	return nil
}

func (r calendarRepo) DeleteLink(ctx context.Context, planItemID string) error {
	const qDeleteLink = `DELETE FROM gcal_links WHERE plan_item_id = ?`
	if _, err := r.db.sql.ExecContext(ctx, qDeleteLink, planItemID); err != nil {
		return fmt.Errorf("delete calendar link %s: %w", planItemID, err)
	}
	return nil
}

func (r calendarRepo) ForgetCalendar(ctx context.Context) error {
	const qClearLinks = `DELETE FROM gcal_links`
	if _, err := r.db.sql.ExecContext(ctx, qClearLinks); err != nil {
		return fmt.Errorf("clear calendar links: %w", err)
	}
	return nil
}

func getPlanItem(ctx context.Context, q Querier, id string) (model.PlanItem, error) {
	const qGetPlanItem = `SELECT ` + planItemCols + ` FROM plan_items WHERE id = ? AND deleted_at IS NULL`
	it, err := scanPlanItem(q.QueryRowContext(ctx, qGetPlanItem, id))
	if err != nil {
		return model.PlanItem{}, fmt.Errorf("get plan item: %w", notFound(err, "plan item", id))
	}
	return it, nil
}

func (r calendarRepo) Item(ctx context.Context, id string) (model.PlanItem, error) {
	return getPlanItem(ctx, r.db.sql, id)
}

func (r calendarRepo) Desired(ctx context.Context, from, to string) ([]PlanEntry, error) {
	const qDesired = `SELECT ` + planItemCols + ` FROM plan_items WHERE deleted_at IS NULL AND day BETWEEN ? AND ?
		AND status IN ('planned', 'done') AND start_at IS NOT NULL ORDER BY day, position, id`
	items, err := queryPlanItems(ctx, r.db.sql, qDesired, from, to)
	if err != nil {
		return nil, fmt.Errorf("calendar events: %w", err)
	}
	return withTasks(ctx, r.db.sql, items)
}

func (r calendarRepo) MoveItem(ctx context.Context, id string, start time.Time, planned time.Duration, day string) (model.PlanItem, error) {
	var it model.PlanItem
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if it, err = getPlanItem(ctx, tx, id); err != nil {
			return err
		}
		start = start.Truncate(time.Millisecond)
		it.StartAt, it.Planned, it.Day, it.Pinned = &start, planned, day, true
		if err := writePlanItem(ctx, tx, r.db, &it); err != nil {
			return err
		}
		const qMoveDay = `UPDATE plan_items SET day = ? WHERE id = ?`
		_, err = tx.ExecContext(ctx, qMoveDay, day, id)
		return classify(err)
	})
	if err != nil {
		return model.PlanItem{}, fmt.Errorf("move plan item %s: %w", id, err)
	}
	return it, nil
}

func (r calendarRepo) SkipItem(ctx context.Context, id string) (model.PlanItem, bool, error) {
	var it model.PlanItem
	changed := false
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if it, err = getPlanItem(ctx, tx, id); err != nil {
			return err
		}
		if it.Status != model.PlanPlanned {
			return nil
		}
		it.Status, changed = model.PlanSkipped, true
		return writePlanItem(ctx, tx, r.db, &it)
	})
	if err != nil {
		return model.PlanItem{}, false, fmt.Errorf("skip plan item %s: %w", id, err)
	}
	return it, changed, nil
}

func (r calendarRepo) ReplaceBusy(ctx context.Context, from, to time.Time, busy []BusyInterval) error {
	now := r.db.Now().UnixMilli()
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		const qClearBusy = `DELETE FROM calendar_busy WHERE start_at < ? AND end_at > ?`
		if _, err := tx.ExecContext(ctx, qClearBusy, to.UnixMilli(), from.UnixMilli()); err != nil {
			return err
		}
		const qInsertBusy = `INSERT OR REPLACE INTO calendar_busy (calendar_id, event_id, start_at, end_at, fetched_at)
			VALUES (?, ?, ?, ?, ?)`
		for _, b := range busy {
			s, e := b.Start.UnixMilli(), b.End.UnixMilli()
			if e <= s {
				continue
			}
			if _, err := tx.ExecContext(ctx, qInsertBusy, b.CalendarID, fmt.Sprintf("%d-%d", s, e), s, e, now); err != nil {
				return classify(err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("replace busy times: %w", err)
	}
	return nil
}

// busyBetween is the busy time intersecting [from, to), for capacity.
func busyBetween(ctx context.Context, q Querier, from, to time.Time) ([]BusyInterval, error) {
	const qBusy = `SELECT calendar_id, start_at, end_at FROM calendar_busy WHERE start_at < ? AND end_at > ?
		ORDER BY start_at`
	rows, err := q.QueryContext(ctx, qBusy, to.UnixMilli(), from.UnixMilli())
	if err != nil {
		return nil, fmt.Errorf("busy times: %w", err)
	}
	defer rows.Close()
	var out []BusyInterval
	for rows.Next() {
		var b BusyInterval
		var s, e int64
		if err := rows.Scan(&b.CalendarID, &s, &e); err != nil {
			return nil, fmt.Errorf("busy times: %w", err)
		}
		b.Start, b.End = timeOf(s), timeOf(e)
		out = append(out, b)
	}
	return out, rows.Err()
}
