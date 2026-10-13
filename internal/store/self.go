package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kzark/gwen/internal/model"
)

// The longest attitude and note a self version holds (00010_memory.sql).
const (
	maxSelfAttitude = 40
	maxSelfNote     = 600
)

type selfRepo struct{ db *DB }

// NewSelfRepo returns the SQLite SelfRepo.
func NewSelfRepo(db *DB) SelfRepo { return selfRepo{db} }

const selfCols = `id, attitude, note, since, reason, thread_id, ` + envelopeCols

func scanSelf(s scanner) (model.SelfVersion, error) {
	var (
		v      model.SelfVersion
		since  sql.NullInt64
		thread sql.NullString
		env    envelopeScan
	)
	dest := append([]any{&v.ID, &v.Attitude, &v.Note, &since, &v.Reason, &thread}, env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.SelfVersion{}, err
	}
	v.Since, v.ThreadID, v.Envelope = timePtr(since), stringPtr(thread), env.envelope()
	return v, nil
}

func (r selfRepo) Current(ctx context.Context) (model.SelfVersion, error) {
	const qCurrentSelf = `SELECT ` + selfCols + ` FROM self_versions WHERE deleted_at IS NULL
		ORDER BY created_at DESC, rowid DESC LIMIT 1`
	v, err := scanSelf(r.db.sql.QueryRowContext(ctx, qCurrentSelf))
	if errors.Is(err, sql.ErrNoRows) {
		return model.SelfVersion{}, Fail(ErrNotFound, "the assistant has not chosen how to treat you yet")
	}
	if err != nil {
		return model.SelfVersion{}, fmt.Errorf("current self: %w", err)
	}
	return v, nil
}

func (r selfRepo) Add(ctx context.Context, v model.SelfVersion) (model.SelfVersion, error) {
	return insertSelf(ctx, r.db.sql, r.db, v)
}

func (r selfRepo) History(ctx context.Context, limit int) ([]model.SelfVersion, error) {
	const qSelfHistory = `SELECT ` + selfCols + ` FROM self_versions WHERE deleted_at IS NULL
		ORDER BY created_at DESC, rowid DESC LIMIT ?`
	out, err := queryList(ctx, r.db.sql, scanSelf, qSelfHistory, sqlLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("self history: %w", err)
	}
	return out, nil
}

// insertSelf appends a self version with the envelope from the store and
// returns it as stored.
func insertSelf(ctx context.Context, q Querier, db *DB, v model.SelfVersion) (model.SelfVersion, error) {
	switch v.Reason {
	case model.SelfChat, model.SelfReset, model.SelfImport:
	default:
		return model.SelfVersion{}, FailField(ErrInvalid, "reason",
			"a self version comes from a chat, a reset, or an import, not %q", v.Reason)
	}
	if v.ID == "" {
		v.ID = model.NewID()
	}
	if v.Since != nil {
		since := timeOf(v.Since.UnixMilli())
		v.Since = &since
	}
	now := db.Now()
	v.Envelope = model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: db.DeviceID(), Rev: 1}
	const qAddSelf = `INSERT INTO self_versions (id, attitude, note, since, reason, thread_id, ` + envelopeCols + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`
	if _, err := q.ExecContext(ctx, qAddSelf, v.ID, v.Attitude, v.Note, nullMillis(v.Since), v.Reason,
		nullString(v.ThreadID), now.UnixMilli(), now.UnixMilli(), v.DeviceID, v.Rev); err != nil {
		return model.SelfVersion{}, fmt.Errorf("add self version: %w", classify(err))
	}
	return v, nil
}

// importSelf moves how the assistant chose to treat the user from
// local_state, where databases before 00010_memory.sql kept it as the JSON
// of wire.AssistantSelf, to self_versions, once. A value that no longer reads
// is dropped, as the API dropped it.
func importSelf(ctx context.Context, db *DB) error {
	_, err := GetLocal(ctx, db.sql, KeySelfImported)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNotFound) {
		return err
	}
	err = db.InTx(ctx, func(tx *sql.Tx) error {
		v, err := GetLocal(ctx, tx, KeyAssistantSelf)
		switch {
		case err == nil:
			var old struct {
				Attitude string `json:"attitude"`
				Note     string `json:"note"`
				Since    *int64 `json:"since"`
			}
			if err := json.Unmarshal([]byte(v), &old); err != nil {
				slog.Warn("assistant self unreadable, not imported", "err", err)
				break
			}
			if old.Attitude == "" && old.Note == "" {
				break
			}
			self := model.SelfVersion{Attitude: cutRunes(old.Attitude, maxSelfAttitude),
				Note: cutRunes(old.Note, maxSelfNote), Reason: model.SelfImport}
			if old.Since != nil {
				since := timeOf(*old.Since)
				self.Since = &since
			}
			if _, err := insertSelf(ctx, tx, db, self); err != nil {
				return err
			}
		case !errors.Is(err, ErrNotFound):
			return err
		}
		if err := DeleteLocal(ctx, tx, KeyAssistantSelf); err != nil {
			return err
		}
		return SetLocalTime(ctx, tx, KeySelfImported, db.Now(), db.Now())
	})
	if err != nil {
		return fmt.Errorf("import assistant self: %w", err)
	}
	return nil
}
