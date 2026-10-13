package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/model"
)

type memoryRepo struct{ db *DB }

// NewMemoryRepo returns the SQLite MemoryRepo.
func NewMemoryRepo(db *DB) MemoryRepo { return memoryRepo{db} }

// sqlLimit is the LIMIT of "at most n" for a listing, where n <= 0 is no
// limit.
func sqlLimit(n int) int {
	if n <= 0 {
		return -1
	}
	return n
}

// hashHex is the hex SHA-256 of s, as memory.Hash computes it.
func hashHex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// queryList runs a query and scans each of its rows.
func queryList[T any](ctx context.Context, q Querier, scan func(scanner) (T, error), query string,
	args ...any) ([]T, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// stringArgs makes query arguments of ss.
func stringArgs(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// checkpoint copies the write-ahead log into the database, so that pages
// holding erased words are not left in it, when no reader is in the way. A
// failure is only logged: what was asked is done.
func checkpoint(ctx context.Context, db *DB, what string) {
	if _, err := db.sql.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		slog.Warn(what+", but the write-ahead log was not checkpointed", "err", err)
	}
}

// optimizeIndex merges a full-text index's segments, the only way it drops
// the words of rows that were erased.
func optimizeIndex(ctx context.Context, q Querier, index string) error {
	_, err := q.ExecContext(ctx, `INSERT INTO `+index+` (`+index+`) VALUES ('optimize')`)
	return err
}

// Episodes.

const episodeCols = `e.id, e.kind, e.channel, e.speaker, e.title, e.body, e.source, e.at, e.sensitivity,
	e.content_hash, e.created_at, e.updated_at, e.deleted_at, e.device_id, e.rev`

func scanEpisode(s scanner, extra ...any) (model.Episode, error) {
	var (
		e   model.Episode
		at  int64
		env envelopeScan
	)
	dest := append([]any{&e.ID, &e.Kind, &e.Channel, &e.Speaker, &e.Title, &e.Body, &e.Source, &at, &e.Sensitivity,
		&e.ContentHash}, env.dest()...)
	if err := s.Scan(append(dest, extra...)...); err != nil {
		return model.Episode{}, err
	}
	e.At, e.Envelope = timeOf(at), env.envelope()
	return e, nil
}

func validSensitivity(s string) bool {
	switch s {
	case model.SensitivityPublic, model.SensitivityPersonal, model.SensitivitySensitive:
		return true
	}
	return false
}

// insertEpisode stores e as AddEpisode describes and returns it as stored.
func insertEpisode(ctx context.Context, q Querier, db *DB, e model.Episode) (model.Episode, error) {
	switch {
	case e.Kind == "":
		return model.Episode{}, FailField(ErrInvalid, "kind", "an episode needs a kind")
	case e.Speaker != model.SpeakerUser && e.Speaker != model.SpeakerGwen && e.Speaker != model.SpeakerExternal:
		return model.Episode{}, FailField(ErrInvalid, "speaker",
			"an episode is said by the user, Gwen, or someone else, not %q", e.Speaker)
	case !validSensitivity(e.Sensitivity):
		return model.Episode{}, FailField(ErrInvalid, "sensitivity",
			"sensitivity is public, personal, or sensitive, not %q", e.Sensitivity)
	case e.At.IsZero():
		return model.Episode{}, FailField(ErrInvalid, "at", "an episode needs the time it was said")
	}
	if e.ID == "" {
		e.ID = model.NewID()
	}
	now := db.Now()
	e.At, e.ContentHash = timeOf(e.At.UnixMilli()), hashHex(e.Body)
	e.Envelope = model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: db.DeviceID(), Rev: 1}
	const qInsertEpisode = `INSERT INTO episodes (id, kind, channel, speaker, title, body, source, at, sensitivity,
		content_hash, ` + envelopeCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)`
	if _, err := q.ExecContext(ctx, qInsertEpisode, e.ID, e.Kind, e.Channel, e.Speaker, e.Title, e.Body, e.Source,
		e.At.UnixMilli(), e.Sensitivity, e.ContentHash, now.UnixMilli(), now.UnixMilli(), e.DeviceID, e.Rev); err != nil {
		return model.Episode{}, fmt.Errorf("add episode: %w", classify(err))
	}
	return e, nil
}

func getEpisode(ctx context.Context, q Querier, id string) (model.Episode, error) {
	const qGetEpisode = `SELECT ` + episodeCols + ` FROM episodes e WHERE e.id = ? AND e.deleted_at IS NULL`
	e, err := scanEpisode(q.QueryRowContext(ctx, qGetEpisode, id))
	if err != nil {
		return model.Episode{}, fmt.Errorf("get episode: %w", notFound(err, "episode", id))
	}
	return e, nil
}

// eraseEpisode makes a live episode a tombstone whose words are erased. Its
// content_hash stays. Erasing a gone episode does nothing.
func eraseEpisode(ctx context.Context, q Querier, db *DB, id string) error {
	now := db.Now().UnixMilli()
	const qEraseEpisode = `UPDATE episodes SET title = '', body = '', source = '', deleted_at = ?, updated_at = ?,
		device_id = ?, rev = rev + 1 WHERE id = ? AND deleted_at IS NULL`
	_, err := q.ExecContext(ctx, qEraseEpisode, now, now, db.DeviceID(), id)
	return err
}

func (r memoryRepo) AddEpisode(ctx context.Context, e model.Episode) (model.Episode, error) {
	return insertEpisode(ctx, r.db.sql, r.db, e)
}

func (r memoryRepo) Episode(ctx context.Context, id string) (model.Episode, error) {
	return getEpisode(ctx, r.db.sql, id)
}

func (r memoryRepo) EpisodesAfter(ctx context.Context, rowid int64, limit int) ([]EpisodeRow, error) {
	const qEpisodesAfter = `SELECT ` + episodeCols + `, e.rowid FROM episodes e
		WHERE e.rowid > ? AND e.deleted_at IS NULL ORDER BY e.rowid LIMIT ?`
	out, err := queryList(ctx, r.db.sql, func(s scanner) (EpisodeRow, error) {
		var row EpisodeRow
		var err error
		row.Episode, err = scanEpisode(s, &row.Rowid)
		return row, err
	}, qEpisodesAfter, rowid, sqlLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("episodes after %d: %w", rowid, err)
	}
	return out, nil
}

func (r memoryRepo) SearchEpisodes(ctx context.Context, text string, limit int) ([]EpisodeHit, error) {
	match := searchQuery(text)
	if match == "" || limit <= 0 {
		return []EpisodeHit{}, nil
	}
	const qSearchEpisodes = `SELECT ` + episodeCols + `, f.rank FROM episodes_fts f JOIN episodes e ON e.rowid = f.rowid
		WHERE episodes_fts MATCH ? AND e.deleted_at IS NULL ORDER BY f.rank, e.at DESC LIMIT ?`
	out, err := queryList(ctx, r.db.sql, func(s scanner) (EpisodeHit, error) {
		var h EpisodeHit
		var err error
		h.Episode, err = scanEpisode(s, &h.Rank)
		return h, err
	}, qSearchEpisodes, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search episodes: %w", err)
	}
	return out, nil
}

func (r memoryRepo) EpisodesBetween(ctx context.Context, from, to time.Time, limit int) ([]model.Episode, error) {
	const qEpisodesBetween = `SELECT ` + episodeCols + ` FROM episodes e
		WHERE e.deleted_at IS NULL AND e.at >= ? AND e.at < ? ORDER BY e.at DESC, e.rowid DESC LIMIT ?`
	out, err := queryList(ctx, r.db.sql, func(s scanner) (model.Episode, error) { return scanEpisode(s) },
		qEpisodesBetween, from.UnixMilli(), to.UnixMilli(), sqlLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("episodes between: %w", err)
	}
	return out, nil
}

func (r memoryRepo) EraseEpisode(ctx context.Context, id string, ledger LedgerItem) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		e, err := getEpisode(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := eraseEpisode(ctx, tx, r.db, id); err != nil {
			return err
		}
		if err := optimizeIndex(ctx, tx, "episodes_fts"); err != nil {
			return err
		}
		if ledger.Data == nil {
			ledger.Data = map[string]any{"action": "episode.erase", "episode": id, "hash": e.ContentHash}
		}
		return appendLedger(ctx, tx, r.db, ledger)
	})
	if err != nil {
		return fmt.Errorf("erase episode %s: %w", id, err)
	}
	checkpoint(ctx, r.db, "episode erased")
	return nil
}

// Chat messages, as the memory reads them.

func (r memoryRepo) MessagesAfter(ctx context.Context, rowid int64, limit int) ([]MessageRow, error) {
	const qMessagesAfter = `SELECT ` + chatMessageCols + `, m.rowid, t.title FROM chat_messages m
		JOIN chat_threads t ON t.id = m.thread_id
		WHERE m.rowid > ? AND m.deleted_at IS NULL AND t.deleted_at IS NULL ORDER BY m.rowid LIMIT ?`
	out, err := queryList(ctx, r.db.sql, func(s scanner) (MessageRow, error) {
		var row MessageRow
		var err error
		row.ChatMessage, err = scanChatMessage(s, &row.Rowid, &row.ThreadTitle)
		return row, err
	}, qMessagesAfter, rowid, sqlLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("messages after %d: %w", rowid, err)
	}
	return out, nil
}

func (r memoryRepo) MessagesBefore(ctx context.Context, threadID string, at time.Time,
	n int) ([]model.ChatMessage, error) {
	if n <= 0 {
		return []model.ChatMessage{}, nil
	}
	const qMessagesBefore = `SELECT ` + chatMessageCols + ` FROM chat_messages m
		WHERE m.thread_id = ? AND m.deleted_at IS NULL AND m.at < ? ORDER BY m.at DESC, m.rowid DESC LIMIT ?`
	out, err := queryList(ctx, r.db.sql, func(s scanner) (model.ChatMessage, error) { return scanChatMessage(s) },
		qMessagesBefore, threadID, at.UnixMilli(), n)
	if err != nil {
		return nil, fmt.Errorf("messages before: %w", err)
	}
	slices.Reverse(out)
	return out, nil
}

func scanChatHit(s scanner) (model.ChatHit, error) {
	var h model.ChatHit
	var err error
	h.ChatMessage, err = scanChatMessage(s, &h.ThreadTitle)
	return h, err
}

func (r memoryRepo) Message(ctx context.Context, id string) (model.ChatHit, error) {
	const qMessage = `SELECT ` + chatMessageCols + `, t.title FROM chat_messages m
		JOIN chat_threads t ON t.id = m.thread_id
		WHERE m.id = ? AND m.deleted_at IS NULL AND t.deleted_at IS NULL`
	h, err := scanChatHit(r.db.sql.QueryRowContext(ctx, qMessage, id))
	if err != nil {
		return model.ChatHit{}, fmt.Errorf("get message: %w", notFound(err, "message", id))
	}
	return h, nil
}

func (r memoryRepo) MessagesBetween(ctx context.Context, from, to time.Time, limit int) ([]model.ChatHit, error) {
	const qMessagesBetween = `SELECT ` + chatMessageCols + `, t.title FROM chat_messages m
		JOIN chat_threads t ON t.id = m.thread_id
		WHERE m.deleted_at IS NULL AND t.deleted_at IS NULL AND m.at >= ? AND m.at < ?
		ORDER BY m.at DESC, m.rowid DESC LIMIT ?`
	out, err := queryList(ctx, r.db.sql, scanChatHit, qMessagesBetween, from.UnixMilli(), to.UnixMilli(),
		sqlLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("messages between: %w", err)
	}
	return out, nil
}

// Claims.

const claimCols = `c.id, c.key, c.subject_id, c.value, c.text, c.cardinality, c.status, c.trust, c.sensitivity,
	c.confidence, c.valid_from, c.valid_to, c.occurred_at, c.mentioned_at, c.recorded_at, c.superseded_at,
	c.superseded_by, c.suppressed, c.source_kind, c.source_id, c.span, c.source_device_id, c.channel, c.extractor,
	c.content_hash, c.created_at, c.updated_at, c.deleted_at, c.device_id, c.rev`

func scanClaim(s scanner, extra ...any) (model.Claim, error) {
	var (
		c                                          model.Claim
		subject, supersededBy                      sql.NullString
		value                                      string
		validFrom, validTo, occurred, supersededAt sql.NullInt64
		mentioned, recorded                        int64
		env                                        envelopeScan
	)
	dest := append([]any{&c.ID, &c.Key, &subject, &value, &c.Text, &c.Cardinality, &c.Status, &c.Trust,
		&c.Sensitivity, &c.Confidence, &validFrom, &validTo, &occurred, &mentioned, &recorded, &supersededAt,
		&supersededBy, &c.Suppressed, &c.SourceKind, &c.SourceID, &c.Span, &c.SourceDeviceID, &c.Channel,
		&c.Extractor, &c.ContentHash}, env.dest()...)
	if err := s.Scan(append(dest, extra...)...); err != nil {
		return model.Claim{}, err
	}
	c.SubjectID, c.SupersededBy, c.Value = stringPtr(subject), stringPtr(supersededBy), []byte(value)
	c.ValidFrom, c.ValidTo, c.OccurredAt = timePtr(validFrom), timePtr(validTo), timePtr(occurred)
	c.MentionedAt, c.RecordedAt, c.SupersededAt = timeOf(mentioned), timeOf(recorded), timePtr(supersededAt)
	c.Envelope = env.envelope()
	return c, nil
}

func scanClaimRow(s scanner) (model.Claim, error) { return scanClaim(s) }

// likePrefix is the LIKE pattern, escaped with \, of the strings that start
// with prefix.
func likePrefix(prefix string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(prefix) + "%"
}

func (r memoryRepo) Claim(ctx context.Context, id string) (model.Claim, error) {
	const qClaim = `SELECT ` + claimCols + ` FROM claims c WHERE c.id = ? AND c.deleted_at IS NULL`
	c, err := scanClaim(r.db.sql.QueryRowContext(ctx, qClaim, id))
	if err != nil {
		return model.Claim{}, fmt.Errorf("get claim: %w", notFound(err, "claim", id))
	}
	return c, nil
}

func (r memoryRepo) Claims(ctx context.Context, f ClaimFilter) ([]model.Claim, error) {
	q := `SELECT ` + claimCols + ` FROM claims c WHERE c.deleted_at IS NULL`
	var args []any
	if len(f.Statuses) > 0 {
		q += ` AND c.status IN (` + placeholders(len(f.Statuses)) + `)`
		args = append(args, stringArgs(f.Statuses)...)
	}
	if f.KeyPrefix != "" {
		q += ` AND c.key LIKE ? ESCAPE '\'`
		args = append(args, likePrefix(f.KeyPrefix))
	}
	if f.SubjectSet {
		q += ` AND c.subject_id IS ?`
		args = append(args, nullString(f.Subject))
	}
	if !f.IncludeSuppressed {
		q += ` AND c.suppressed = 0`
	}
	if f.ExcludeSensitive {
		q += ` AND c.sensitivity <> 'sensitive'`
	}
	if f.ValidAt != nil {
		q += ` AND (c.valid_from IS NULL OR c.valid_from <= ?) AND (c.valid_to IS NULL OR c.valid_to > ?)`
		args = append(args, f.ValidAt.UnixMilli(), f.ValidAt.UnixMilli())
	}
	q += ` ORDER BY c.recorded_at DESC, c.id DESC LIMIT ? OFFSET ?`
	out, err := queryList(ctx, r.db.sql, scanClaimRow, q, append(args, sqlLimit(f.Limit), max(f.Offset, 0))...)
	if err != nil {
		return nil, fmt.Errorf("list claims: %w", err)
	}
	return out, nil
}

func (r memoryRepo) ClaimsByKey(ctx context.Context, key string, subjectID *string,
	statuses ...string) ([]model.Claim, error) {
	q := `SELECT ` + claimCols + ` FROM claims c WHERE c.deleted_at IS NULL AND c.key = ? AND c.subject_id IS ?`
	args := []any{key, nullString(subjectID)}
	if len(statuses) > 0 {
		q += ` AND c.status IN (` + placeholders(len(statuses)) + `)`
		args = append(args, stringArgs(statuses)...)
	}
	q += ` ORDER BY COALESCE(c.valid_from, c.mentioned_at) DESC, c.recorded_at DESC, c.id DESC`
	out, err := queryList(ctx, r.db.sql, scanClaimRow, q, args...)
	if err != nil {
		return nil, fmt.Errorf("claims of %s: %w", key, err)
	}
	return out, nil
}

func (r memoryRepo) ClaimsByHash(ctx context.Context, hash string) ([]model.Claim, error) {
	const qClaimsByHash = `SELECT ` + claimCols + ` FROM claims c WHERE c.content_hash = ?
		ORDER BY c.recorded_at DESC, c.id DESC`
	out, err := queryList(ctx, r.db.sql, scanClaimRow, qClaimsByHash, hash)
	if err != nil {
		return nil, fmt.Errorf("claims by hash: %w", err)
	}
	return out, nil
}

func (r memoryRepo) ClaimsFrom(ctx context.Context, sourceKind, sourceID string) ([]model.Claim, error) {
	const qClaimsFrom = `SELECT ` + claimCols + ` FROM claims c
		WHERE c.deleted_at IS NULL AND c.source_kind = ? AND c.source_id = ? ORDER BY c.recorded_at, c.id`
	out, err := queryList(ctx, r.db.sql, scanClaimRow, qClaimsFrom, sourceKind, sourceID)
	if err != nil {
		return nil, fmt.Errorf("claims from %s:%s: %w", sourceKind, sourceID, err)
	}
	return out, nil
}

func (r memoryRepo) SearchClaims(ctx context.Context, text string, limit int) ([]ClaimHit, error) {
	match := searchQuery(text)
	if match == "" || limit <= 0 {
		return []ClaimHit{}, nil
	}
	const qSearchClaims = `SELECT ` + claimCols + `, f.rank FROM claims_fts f JOIN claims c ON c.rowid = f.rowid
		WHERE claims_fts MATCH ? AND c.deleted_at IS NULL ORDER BY f.rank, c.recorded_at DESC LIMIT ?`
	out, err := queryList(ctx, r.db.sql, func(s scanner) (ClaimHit, error) {
		var h ClaimHit
		var err error
		h.Claim, err = scanClaim(s, &h.Rank)
		return h, err
	}, qSearchClaims, match, limit)
	if err != nil {
		return nil, fmt.Errorf("search claims: %w", err)
	}
	return out, nil
}

func (r memoryRepo) CountClaims(ctx context.Context) (map[string]int, error) {
	const qCountClaims = `SELECT status, count(*) FROM claims WHERE deleted_at IS NULL GROUP BY status`
	return countBy(ctx, r.db.sql, "count claims", qCountClaims)
}

// countBy runs a query of (name, count) rows into a map.
func countBy(ctx context.Context, q Querier, what, query string) (map[string]int, error) {
	type pair struct {
		k string
		n int
	}
	rows, err := queryList(ctx, q, func(s scanner) (pair, error) {
		var p pair
		return p, s.Scan(&p.k, &p.n)
	}, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	out := make(map[string]int, len(rows))
	for _, p := range rows {
		out[p.k] = p.n
	}
	return out, nil
}

// insertClaim stores a new claim with the envelope from the store.
func insertClaim(ctx context.Context, q Querier, db *DB, c model.Claim) error {
	if !json.Valid(c.Value) {
		return FailField(ErrInvalid, "value", "a claim's value must be JSON")
	}
	now := db.Now()
	if c.ID == "" {
		c.ID = model.NewID()
	}
	if c.RecordedAt.IsZero() {
		c.RecordedAt = now
	}
	if c.MentionedAt.IsZero() {
		c.MentionedAt = c.RecordedAt
	}
	if c.SourceDeviceID == "" {
		c.SourceDeviceID = db.DeviceID()
	}
	if c.SupersededBy != nil && c.SupersededAt == nil {
		c.SupersededAt = &now
	}
	const qInsertClaim = `INSERT INTO claims (id, key, subject_id, value, text, cardinality, status, trust,
		sensitivity, confidence, valid_from, valid_to, occurred_at, mentioned_at, recorded_at, superseded_at,
		superseded_by, suppressed, source_kind, source_id, span, source_device_id, channel, extractor, content_hash,
		` + envelopeCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?,
		?, ?, NULL, ?, 1)`
	_, err := q.ExecContext(ctx, qInsertClaim, c.ID, c.Key, nullString(c.SubjectID), string(c.Value), c.Text,
		c.Cardinality, c.Status, c.Trust, c.Sensitivity, c.Confidence, nullMillis(c.ValidFrom), nullMillis(c.ValidTo),
		nullMillis(c.OccurredAt), c.MentionedAt.UnixMilli(), c.RecordedAt.UnixMilli(), nullMillis(c.SupersededAt),
		nullString(c.SupersededBy), boolInt(c.Suppressed), c.SourceKind, c.SourceID, c.Span, c.SourceDeviceID,
		c.Channel, c.Extractor, c.ContentHash, now.UnixMilli(), now.UnixMilli(), db.DeviceID())
	if err != nil {
		return fmt.Errorf("add claim: %w", classify(err))
	}
	return nil
}

// Why a claim was forgotten, in the ledger rows of the conflicts it closes.
const (
	reasonClaimForgotten = "claim forgotten"
	reasonChatDeleted    = "chat deleted"
)

// updateClaim applies u to a live claim, as ClaimUpdate describes. A claim
// it forgets first has its words taken out of everything derived from it
// (forgetWords), whose ledger rows give reason.
func updateClaim(ctx context.Context, tx *sql.Tx, db *DB, u ClaimUpdate, reason string) error {
	var (
		status, text, value, span string
		deleted                   sql.NullInt64
	)
	const qClaimStatus = `SELECT status, deleted_at, text, value, span FROM claims WHERE id = ?`
	err := tx.QueryRowContext(ctx, qClaimStatus, u.ID).Scan(&status, &deleted, &text, &value, &span)
	if errors.Is(err, sql.ErrNoRows) || err == nil && deleted.Valid {
		return Fail(ErrNotFound, "no claim %s", u.ID)
	}
	if err != nil {
		return err
	}
	if u.WasStatus != "" && u.WasStatus != status {
		return Fail(ErrConflict, "the claim changed while this was decided")
	}
	now := db.Now().UnixMilli()
	var (
		sets []string
		args []any
	)
	set := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	switch {
	case u.SupersededBy != nil:
		set("status", model.ClaimSuperseded)
		set("superseded_by", *u.SupersededBy)
		set("superseded_at", now)
	case u.Status != nil:
		set("status", *u.Status)
		if *u.Status != model.ClaimSuperseded {
			// A claim no longer superseded is superseded by nothing.
			set("superseded_by", nil)
			set("superseded_at", nil)
		}
	}
	if u.ValidTo.Set {
		set("valid_to", nullMillis(u.ValidTo.Value))
	}
	if u.Suppressed != nil {
		set("suppressed", boolInt(*u.Suppressed))
	}
	if u.Confidence != nil {
		set("confidence", *u.Confidence)
	}
	if u.Trust != nil {
		set("trust", *u.Trust)
	}
	if u.Forget {
		if err := forgetWords(ctx, tx, db, u.ID, text, value, span, reason); err != nil {
			return fmt.Errorf("forget claim %s: %w", u.ID, err)
		}
		set("deleted_at", now)
		set("text", "")
		set("value", "null")
		set("span", "")
	}
	if len(sets) == 0 {
		return nil
	}
	set("updated_at", now)
	set("device_id", db.DeviceID())
	q := `UPDATE claims SET ` + strings.Join(sets, ", ") + `, rev = rev + 1 WHERE id = ?`
	if _, err := tx.ExecContext(ctx, q, append(args, u.ID)...); err != nil {
		return fmt.Errorf("update claim %s: %w", u.ID, classify(err))
	}
	return nil
}

// Shortest words of a claim that an observation line is dropped for holding:
// its value when that is a JSON string, its span, and its text.
const (
	minForgetValue = 4
	minForgetSpan  = 8
	minForgetText  = 12
)

// forgetWords takes a forgotten claim's words, text, value, and span, out
// of what was made from it, in tx:
//   - each memory note citing it keeps the ref and loses the words: its line
//     becomes "[m3] (forgotten)";
//   - each profile card citing it is deleted, to be compiled again;
//   - each conflict it is in loses its reason, and an open one is dismissed
//     with a ledger row giving reason;
//   - each observation log loses the lines holding its value, span, or text,
//     ignoring case.
func forgetWords(ctx context.Context, tx *sql.Tx, db *DB, id, text, value, span, reason string) error {
	ref := "claim:" + id
	if err := forgetInNotes(ctx, tx, ref); err != nil {
		return err
	}
	const qDropCards = `DELETE FROM profile_cards
		WHERE EXISTS (SELECT 1 FROM json_each(profile_cards.refs) WHERE value = ?)`
	if _, err := tx.ExecContext(ctx, qDropCards, ref); err != nil {
		return err
	}
	if err := forgetInConflicts(ctx, tx, db, id, reason); err != nil {
		return err
	}
	var needles []string
	var s string
	if json.Unmarshal([]byte(value), &s) == nil && utf8.RuneCountInString(strings.TrimSpace(s)) >= minForgetValue {
		needles = append(needles, strings.TrimSpace(s))
	}
	if utf8.RuneCountInString(strings.TrimSpace(span)) >= minForgetSpan {
		needles = append(needles, strings.TrimSpace(span))
	}
	if utf8.RuneCountInString(strings.TrimSpace(text)) >= minForgetText {
		needles = append(needles, strings.TrimSpace(text))
	}
	return forgetInObservations(ctx, tx, db, needles)
}

// forgetInNotes rewrites the lines of the notes citing ref.
func forgetInNotes(ctx context.Context, tx *sql.Tx, ref string) error {
	type cited struct{ msg, note, ref string }
	const qCitingNotes = `SELECT n.message_id, n.note, j.key FROM chat_notes n, json_each(n.refs) j
		WHERE j.value = ? ORDER BY n.message_id`
	rows, err := queryList(ctx, tx, func(s scanner) (cited, error) {
		var c cited
		return c, s.Scan(&c.msg, &c.note, &c.ref)
	}, qCitingNotes, ref)
	if err != nil {
		return err
	}
	notes, order := map[string]string{}, []string{}
	for _, c := range rows {
		note, seen := notes[c.msg]
		if !seen {
			note = c.note
			order = append(order, c.msg)
		}
		prefix := "[" + c.ref + "]"
		lines := strings.Split(note, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, prefix) {
				lines[i] = prefix + " (forgotten)"
			}
		}
		notes[c.msg] = strings.Join(lines, "\n")
	}
	const qRewriteNote = `UPDATE chat_notes SET note = ? WHERE message_id = ?`
	for _, msg := range order {
		if _, err := tx.ExecContext(ctx, qRewriteNote, notes[msg], msg); err != nil {
			return err
		}
	}
	return nil
}

// forgetInConflicts erases the reasons of the conflicts a claim is in, and
// dismisses the open ones.
func forgetInConflicts(ctx context.Context, tx *sql.Tx, db *DB, id, reason string) error {
	open, err := openConflictsOf(ctx, tx, id)
	if err != nil {
		return err
	}
	now, device := db.Now().UnixMilli(), db.DeviceID()
	for _, c := range open {
		const qDismiss = `UPDATE conflicts SET status = 'dismissed', resolution = NULL, resolved_at = NULL, reason = '',
			updated_at = ?, device_id = ?, rev = rev + 1 WHERE id = ?`
		if _, err := tx.ExecContext(ctx, qDismiss, now, device, c.ID); err != nil {
			return err
		}
		if err := appendLedger(ctx, tx, db, LedgerItem{Data: map[string]any{"action": "conflict.dismiss",
			"conflict": c.ID, "reason": reason}}); err != nil {
			return err
		}
	}
	const qEraseReasons = `UPDATE conflicts SET reason = '', updated_at = ?, device_id = ?, rev = rev + 1
		WHERE (claim_a = ? OR claim_b = ?) AND reason <> ''`
	_, err = tx.ExecContext(ctx, qEraseReasons, now, device, id, id)
	return err
}

// forgetInObservations drops the lines of every observation log that hold
// one of needles, ignoring case.
func forgetInObservations(ctx context.Context, tx *sql.Tx, db *DB, needles []string) error {
	if len(needles) == 0 {
		return nil
	}
	for i, n := range needles {
		needles[i] = strings.ToLower(n)
	}
	type observation struct{ thread, log string }
	logs, err := queryList(ctx, tx, func(s scanner) (observation, error) {
		var o observation
		return o, s.Scan(&o.thread, &o.log)
	}, `SELECT thread_id, log FROM chat_observations ORDER BY thread_id`)
	if err != nil {
		return err
	}
	for _, o := range logs {
		lines := strings.Split(o.log, "\n")
		kept := lines[:0:0]
		for _, line := range lines {
			lower := strings.ToLower(line)
			if !slices.ContainsFunc(needles, func(n string) bool { return strings.Contains(lower, n) }) {
				kept = append(kept, line)
			}
		}
		if len(kept) == len(lines) {
			continue
		}
		const qRewriteLog = `UPDATE chat_observations SET log = ?, updated_at = ? WHERE thread_id = ?`
		if _, err := tx.ExecContext(ctx, qRewriteLog, strings.Join(kept, "\n"), db.Now().UnixMilli(),
			o.thread); err != nil {
			return err
		}
	}
	return nil
}

// Entities.

const entityCols = `id, kind, name, aliases, ` + envelopeCols

func scanEntity(s scanner) (model.Entity, error) {
	var (
		e       model.Entity
		aliases string
		env     envelopeScan
	)
	if err := s.Scan(append([]any{&e.ID, &e.Kind, &e.Name, &aliases}, env.dest()...)...); err != nil {
		return model.Entity{}, err
	}
	if err := json.Unmarshal([]byte(aliases), &e.Aliases); err != nil {
		return model.Entity{}, fmt.Errorf("aliases of entity %s: %w", e.ID, err)
	}
	e.Envelope = env.envelope()
	return e, nil
}

func (r memoryRepo) Entity(ctx context.Context, id string) (model.Entity, error) {
	const qEntity = `SELECT ` + entityCols + ` FROM entities WHERE id = ? AND deleted_at IS NULL`
	e, err := scanEntity(r.db.sql.QueryRowContext(ctx, qEntity, id))
	if err != nil {
		return model.Entity{}, fmt.Errorf("get entity: %w", notFound(err, "entity", id))
	}
	return e, nil
}

func (r memoryRepo) Entities(ctx context.Context, kind string) ([]model.Entity, error) {
	const qEntities = `SELECT ` + entityCols + ` FROM entities WHERE deleted_at IS NULL AND (? = '' OR kind = ?)
		ORDER BY name COLLATE NOCASE, id`
	out, err := queryList(ctx, r.db.sql, scanEntity, qEntities, kind, kind)
	if err != nil {
		return nil, fmt.Errorf("list entities: %w", err)
	}
	return out, nil
}

func (r memoryRepo) FindEntities(ctx context.Context, name string) ([]model.Entity, error) {
	if strings.TrimSpace(name) == "" {
		return []model.Entity{}, nil
	}
	const qFindEntities = `SELECT ` + entityCols + ` FROM entities WHERE deleted_at IS NULL
		AND (lower(name) = lower(?) OR EXISTS (SELECT 1 FROM json_each(aliases) WHERE lower(value) = lower(?)))
		ORDER BY name COLLATE NOCASE, id`
	out, err := queryList(ctx, r.db.sql, scanEntity, qFindEntities, name, name)
	if err != nil {
		return nil, fmt.Errorf("find entities: %w", err)
	}
	return out, nil
}

// upsertEntity inserts an entity or, when its id exists, renames it and
// gives it the aliases.
func upsertEntity(ctx context.Context, q Querier, db *DB, e model.Entity) error {
	if e.ID == "" {
		e.ID = model.NewID()
	}
	aliases := e.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	b, err := json.Marshal(aliases)
	if err != nil {
		return err
	}
	now := db.Now().UnixMilli()
	const qUpsertEntity = `INSERT INTO entities (id, kind, name, aliases, ` + envelopeCols + `)
		VALUES (?, ?, ?, ?, ?, ?, NULL, ?, 1)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, aliases = excluded.aliases,
			updated_at = excluded.updated_at, device_id = excluded.device_id, rev = entities.rev + 1`
	if _, err := q.ExecContext(ctx, qUpsertEntity, e.ID, e.Kind, e.Name, string(b), now, now,
		db.DeviceID()); err != nil {
		return fmt.Errorf("add entity: %w", classify(err))
	}
	return nil
}

// Conflicts.

const conflictCols = `id, claim_a, claim_b, reason, sensitivity, status, resolution, resolved_at, ` + envelopeCols

func scanConflict(s scanner) (model.Conflict, error) {
	var (
		c          model.Conflict
		resolution sql.NullString
		resolved   sql.NullInt64
		env        envelopeScan
	)
	if err := s.Scan(append([]any{&c.ID, &c.ClaimA, &c.ClaimB, &c.Reason, &c.Sensitivity, &c.Status, &resolution,
		&resolved}, env.dest()...)...); err != nil {
		return model.Conflict{}, err
	}
	c.Resolution, c.ResolvedAt, c.Envelope = stringPtr(resolution), timePtr(resolved), env.envelope()
	return c, nil
}

func (r memoryRepo) Conflict(ctx context.Context, id string) (model.Conflict, error) {
	const qConflict = `SELECT ` + conflictCols + ` FROM conflicts WHERE id = ? AND deleted_at IS NULL`
	c, err := scanConflict(r.db.sql.QueryRowContext(ctx, qConflict, id))
	if err != nil {
		return model.Conflict{}, fmt.Errorf("get conflict: %w", notFound(err, "conflict", id))
	}
	return c, nil
}

func (r memoryRepo) Conflicts(ctx context.Context, status string) ([]model.Conflict, error) {
	const qConflicts = `SELECT ` + conflictCols + ` FROM conflicts WHERE deleted_at IS NULL AND (? = '' OR status = ?)
		ORDER BY created_at DESC, id DESC`
	out, err := queryList(ctx, r.db.sql, scanConflict, qConflicts, status, status)
	if err != nil {
		return nil, fmt.Errorf("list conflicts: %w", err)
	}
	return out, nil
}

func (r memoryRepo) OpenConflictsOf(ctx context.Context, claimID string) ([]model.Conflict, error) {
	out, err := openConflictsOf(ctx, r.db.sql, claimID)
	if err != nil {
		return nil, fmt.Errorf("conflicts of claim %s: %w", claimID, err)
	}
	return out, nil
}

func openConflictsOf(ctx context.Context, q Querier, claimID string) ([]model.Conflict, error) {
	const qOpenConflictsOf = `SELECT ` + conflictCols + ` FROM conflicts
		WHERE deleted_at IS NULL AND status = 'open' AND (claim_a = ? OR claim_b = ?) ORDER BY created_at, id`
	return queryList(ctx, q, scanConflict, qOpenConflictsOf, claimID, claimID)
}

func insertConflict(ctx context.Context, q Querier, db *DB, c model.Conflict) error {
	now := db.Now()
	if c.ID == "" {
		c.ID = model.NewID()
	}
	if c.Status == "" {
		c.Status = model.ConflictOpen
	}
	if c.Status == model.ConflictResolved && c.ResolvedAt == nil {
		c.ResolvedAt = &now
	}
	const qInsertConflict = `INSERT INTO conflicts (id, claim_a, claim_b, reason, sensitivity, status, resolution,
		resolved_at, ` + envelopeCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, 1)`
	if _, err := q.ExecContext(ctx, qInsertConflict, c.ID, c.ClaimA, c.ClaimB, c.Reason, c.Sensitivity, c.Status,
		nullString(c.Resolution), nullMillis(c.ResolvedAt), now.UnixMilli(), now.UnixMilli(),
		db.DeviceID()); err != nil {
		return fmt.Errorf("add conflict: %w", classify(err))
	}
	return nil
}

// updateConflict resolves or dismisses a live conflict; resolved_at is set
// when it is resolved.
func updateConflict(ctx context.Context, q Querier, db *DB, u ConflictUpdate) error {
	now := db.Now().UnixMilli()
	var resolvedAt sql.NullInt64
	if u.Status == model.ConflictResolved {
		resolvedAt = sql.NullInt64{Int64: now, Valid: true}
	}
	const qUpdateConflict = `UPDATE conflicts SET status = ?, resolution = ?, resolved_at = ?, updated_at = ?,
		device_id = ?, rev = rev + 1 WHERE id = ? AND deleted_at IS NULL`
	res, err := q.ExecContext(ctx, qUpdateConflict, u.Status, nullString(u.Resolution), resolvedAt, now,
		db.DeviceID(), u.ID)
	if err != nil {
		return fmt.Errorf("update conflict %s: %w", u.ID, classify(err))
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return Fail(ErrNotFound, "no conflict %s", u.ID)
	}
	return nil
}

// Links.

const linkCols = `id, from_kind, from_id, to_kind, to_id, relation, ` + envelopeCols

func scanLink(s scanner) (model.MemoryLink, error) {
	var (
		l   model.MemoryLink
		env envelopeScan
	)
	if err := s.Scan(append([]any{&l.ID, &l.FromKind, &l.FromID, &l.ToKind, &l.ToID, &l.Relation},
		env.dest()...)...); err != nil {
		return model.MemoryLink{}, err
	}
	l.Envelope = env.envelope()
	return l, nil
}

func (r memoryRepo) LinksFrom(ctx context.Context, fromID string) ([]model.MemoryLink, error) {
	const qLinksFrom = `SELECT ` + linkCols + ` FROM memory_links WHERE deleted_at IS NULL AND from_id = ?
		ORDER BY created_at, id`
	out, err := queryList(ctx, r.db.sql, scanLink, qLinksFrom, fromID)
	if err != nil {
		return nil, fmt.Errorf("links from %s: %w", fromID, err)
	}
	return out, nil
}

func (r memoryRepo) LinksTo(ctx context.Context, toKind, toID string) ([]model.MemoryLink, error) {
	const qLinksTo = `SELECT ` + linkCols + ` FROM memory_links WHERE deleted_at IS NULL AND to_kind = ? AND to_id = ?
		ORDER BY created_at, id`
	out, err := queryList(ctx, r.db.sql, scanLink, qLinksTo, toKind, toID)
	if err != nil {
		return nil, fmt.Errorf("links to %s:%s: %w", toKind, toID, err)
	}
	return out, nil
}

// insertLink stores a link unless the same one is live. Unlike INSERT OR
// IGNORE, ON CONFLICT DO NOTHING skips only that, not a row the schema
// rejects.
func insertLink(ctx context.Context, q Querier, db *DB, l model.MemoryLink) error {
	if l.ID == "" {
		l.ID = model.NewID()
	}
	now := db.Now().UnixMilli()
	const qInsertLink = `INSERT INTO memory_links (id, from_kind, from_id, to_kind, to_id, relation, ` + envelopeCols + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, ?, 1) ON CONFLICT DO NOTHING`
	if _, err := q.ExecContext(ctx, qInsertLink, l.ID, l.FromKind, l.FromID, l.ToKind, l.ToID, l.Relation, now, now,
		db.DeviceID()); err != nil {
		return fmt.Errorf("add link: %w", classify(err))
	}
	return nil
}

// Jobs.

// jsonObject marshals v, which must make a JSON object; nil is {}.
func jsonObject(v any) (string, error) {
	if v == nil {
		return "{}", nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	s := string(b)
	if s == "null" {
		return "{}", nil
	}
	if !strings.HasPrefix(s, "{") {
		return "", fmt.Errorf("%s is not a JSON object", s)
	}
	return s, nil
}

// insertJob enqueues a pending job, unless one of the same kind and subject
// is pending or running.
func insertJob(ctx context.Context, q Querier, now time.Time, j NewJob) error {
	payload, err := jsonObject(j.Payload)
	if err != nil {
		return FailField(ErrInvalid, "payload", "a job's payload must be a JSON object: %v", err)
	}
	runAfter := j.RunAfter
	if runAfter.IsZero() {
		runAfter = now
	}
	const qInsertJob = `INSERT INTO memory_jobs (kind, subject, payload, state, attempts, run_after, last_error,
		created_at, updated_at) VALUES (?, ?, ?, 'pending', 0, ?, '', ?, ?) ON CONFLICT DO NOTHING`
	if _, err := q.ExecContext(ctx, qInsertJob, j.Kind, j.Subject, payload, runAfter.UnixMilli(), now.UnixMilli(),
		now.UnixMilli()); err != nil {
		return fmt.Errorf("enqueue %s job: %w", j.Kind, classify(err))
	}
	return nil
}

// Apply.

func (r memoryRepo) Apply(ctx context.Context, ch MemoryChange) error {
	forgot := slices.ContainsFunc(ch.Updates, func(u ClaimUpdate) bool { return u.Forget })
	err := r.db.InTx(ctx, func(tx *sql.Tx) error { return applyChange(ctx, tx, r.db, ch, forgot) })
	if err != nil {
		return fmt.Errorf("change memory: %w", err)
	}
	if forgot {
		checkpoint(ctx, r.db, "claims forgotten")
	}
	return nil
}

// applyChange writes ch in tx in the order MemoryChange gives. When it
// forgets claims, forgot merges the search index so their words leave it.
func applyChange(ctx context.Context, tx *sql.Tx, db *DB, ch MemoryChange, forgot bool) error {
	for _, e := range ch.Entities {
		if err := upsertEntity(ctx, tx, db, e); err != nil {
			return err
		}
	}
	for _, c := range ch.Claims {
		if err := insertClaim(ctx, tx, db, c); err != nil {
			return err
		}
	}
	for _, u := range ch.Updates {
		if err := updateClaim(ctx, tx, db, u, reasonClaimForgotten); err != nil {
			return err
		}
	}
	for _, c := range ch.Conflicts {
		if err := insertConflict(ctx, tx, db, c); err != nil {
			return err
		}
	}
	for _, u := range ch.ConflictUpdates {
		if err := updateConflict(ctx, tx, db, u); err != nil {
			return err
		}
	}
	for _, l := range ch.Links {
		if err := insertLink(ctx, tx, db, l); err != nil {
			return err
		}
	}
	for _, item := range ch.Ledger {
		if err := appendLedger(ctx, tx, db, item); err != nil {
			return err
		}
	}
	for _, j := range ch.Jobs {
		if err := insertJob(ctx, tx, db.Now(), j); err != nil {
			return err
		}
	}
	if forgot {
		return optimizeIndex(ctx, tx, "claims_fts")
	}
	return nil
}

// The ledger.

// zeroHash is the prev_hash of the first ledger row.
var zeroHash = strings.Repeat("0", 64)

// ledgerHash is a ledger row's hash, which covers the row before it.
func ledgerHash(prev, kind, runID string, at int64, data string) string {
	return hashHex(prev + "\n" + kind + "\n" + runID + "\n" + strconv.FormatInt(at, 10) + "\n" + data)
}

// appendLedger appends a row to the ledger, chained to the newest. It must
// run in a transaction, which holds the write lock, so no other row slips in
// between.
func appendLedger(ctx context.Context, q Querier, db *DB, item LedgerItem) error {
	kind := item.Kind
	if kind == "" {
		kind = model.LedgerMemory
	}
	data, err := jsonObject(item.Data)
	if err != nil {
		return FailField(ErrInvalid, "data", "a ledger row's data must be a JSON object: %v", err)
	}
	prev := zeroHash
	const qLedgerHead = `SELECT hash FROM ledger ORDER BY seq DESC LIMIT 1`
	if err := q.QueryRowContext(ctx, qLedgerHead).Scan(&prev); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read the ledger: %w", err)
	}
	runID := ""
	if item.RunID != nil {
		runID = *item.RunID
	}
	at := db.Now().UnixMilli()
	const qAppendLedger = `INSERT INTO ledger (run_id, kind, data, at, prev_hash, hash) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := q.ExecContext(ctx, qAppendLedger, nullString(item.RunID), kind, data, at, prev,
		ledgerHash(prev, kind, runID, at, data)); err != nil {
		return fmt.Errorf("append to the ledger: %w", classify(err))
	}
	return nil
}

func scanLedgerEntry(s scanner) (model.LedgerEntry, error) {
	var (
		e     model.LedgerEntry
		runID sql.NullString
		data  string
		at    int64
	)
	if err := s.Scan(&e.Seq, &runID, &e.Kind, &data, &at, &e.PrevHash, &e.Hash); err != nil {
		return model.LedgerEntry{}, err
	}
	e.RunID, e.Data, e.At = stringPtr(runID), []byte(data), timeOf(at)
	return e, nil
}

func (r memoryRepo) Ledger(ctx context.Context, after int64, limit int) ([]model.LedgerEntry, error) {
	const qLedger = `SELECT seq, run_id, kind, data, at, prev_hash, hash FROM ledger WHERE seq > ? ORDER BY seq LIMIT ?`
	out, err := queryList(ctx, r.db.sql, scanLedgerEntry, qLedger, after, sqlLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("read the ledger: %w", err)
	}
	return out, nil
}

func (r memoryRepo) VerifyLedger(ctx context.Context) (int64, error) {
	const qWholeLedger = `SELECT seq, run_id, kind, data, at, prev_hash, hash FROM ledger ORDER BY seq`
	rows, err := r.db.sql.QueryContext(ctx, qWholeLedger)
	if err != nil {
		return 0, fmt.Errorf("verify the ledger: %w", err)
	}
	defer rows.Close()
	prev := zeroHash
	for rows.Next() {
		e, err := scanLedgerEntry(rows)
		if err != nil {
			return 0, fmt.Errorf("verify the ledger: %w", err)
		}
		runID := ""
		if e.RunID != nil {
			runID = *e.RunID
		}
		if e.PrevHash != prev || ledgerHash(e.PrevHash, e.Kind, runID, e.At.UnixMilli(), string(e.Data)) != e.Hash {
			return e.Seq, nil
		}
		prev = e.Hash
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("verify the ledger: %w", err)
	}
	return 0, nil
}

// Forgetting everything.

func (r memoryRepo) EraseAll(ctx context.Context, ledger LedgerItem) (Erased, error) {
	var erased Erased
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		now, device := r.db.Now().UnixMilli(), r.db.DeviceID()
		for _, step := range []struct {
			n     *int
			query string
		}{
			{&erased.Claims, `UPDATE claims SET text = '', value = 'null', span = '', deleted_at = ?, updated_at = ?,
				device_id = ?, rev = rev + 1 WHERE deleted_at IS NULL`},
			{&erased.Episodes, `UPDATE episodes SET title = '', body = '', source = '', deleted_at = ?, updated_at = ?,
				device_id = ?, rev = rev + 1 WHERE deleted_at IS NULL`},
			{&erased.Entities, `UPDATE entities SET name = '', aliases = '[]', deleted_at = ?, updated_at = ?,
				device_id = ?, rev = rev + 1 WHERE deleted_at IS NULL`},
			{&erased.Conflicts, `UPDATE conflicts SET reason = '', deleted_at = ?, updated_at = ?, device_id = ?,
				rev = rev + 1 WHERE deleted_at IS NULL`},
			{&erased.Links, `UPDATE memory_links SET deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1
				WHERE deleted_at IS NULL`},
		} {
			res, err := tx.ExecContext(ctx, step.query, now, now, device)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil {
				return err
			}
			*step.n = int(n)
		}
		const qLiveResources = `SELECT count(*) FROM resources WHERE deleted_at IS NULL`
		if err := tx.QueryRowContext(ctx, qLiveResources).Scan(&erased.Resources); err != nil {
			return err
		}
		for _, table := range []string{"chat_notes", "chat_observations", "profile_cards", "memory_jobs",
			"memory_backends", "memory_usage", "resources"} {
			if _, err := tx.ExecContext(ctx, `DELETE FROM `+table); err != nil {
				return err
			}
		}
		for _, index := range []string{"claims_fts", "episodes_fts"} {
			if err := optimizeIndex(ctx, tx, index); err != nil {
				return err
			}
		}
		data := map[string]any{}
		if ledger.Data != nil {
			s, err := jsonObject(ledger.Data)
			if err != nil {
				return FailField(ErrInvalid, "data", "a ledger row's data must be a JSON object: %v", err)
			}
			if err := json.Unmarshal([]byte(s), &data); err != nil {
				return err
			}
		}
		data["action"] = "memory.erase_all"
		data["claims"], data["episodes"], data["entities"] = erased.Claims, erased.Episodes, erased.Entities
		data["conflicts"], data["links"], data["resources"] = erased.Conflicts, erased.Links, erased.Resources
		ledger.Data = data
		return appendLedger(ctx, tx, r.db, ledger)
	})
	if err != nil {
		return Erased{}, fmt.Errorf("forget everything: %w", err)
	}
	checkpoint(ctx, r.db, "memory erased")
	return erased, nil
}
