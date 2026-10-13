package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// maxJobError is the most characters of a job's last error that are kept.
const maxJobError = 1000

// keepProfiles is how many profile cards are kept, the newest.
const keepProfiles = 30

type memoryStateRepo struct{ db *DB }

// NewMemoryStateRepo returns the SQLite MemoryStateRepo.
func NewMemoryStateRepo(db *DB) MemoryStateRepo { return memoryStateRepo{db} }

// refsJSON is the JSON object of a refs map; nil is {}.
func refsJSON(refs map[string]string) (string, error) {
	if refs == nil {
		return "{}", nil
	}
	b, err := json.Marshal(refs)
	return string(b), err
}

func refsOf(s string) (map[string]string, error) {
	refs := map[string]string{}
	if err := json.Unmarshal([]byte(s), &refs); err != nil {
		return nil, fmt.Errorf("refs: %w", err)
	}
	return refs, nil
}

// cutRunes cuts s to at most n characters.
func cutRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// Observation logs.

func (r memoryStateRepo) Observation(ctx context.Context, threadID string) (model.ChatObservation, error) {
	const qObservation = `SELECT thread_id, through_id, through_at, log, reflections, updated_at
		FROM chat_observations WHERE thread_id = ?`
	var (
		o                model.ChatObservation
		through, updated int64
	)
	err := r.db.sql.QueryRowContext(ctx, qObservation, threadID).Scan(&o.ThreadID, &o.ThroughID, &through, &o.Log,
		&o.Reflections, &updated)
	if err != nil {
		return model.ChatObservation{}, fmt.Errorf("observation log: %w", notFound(err, "observation log of chat",
			threadID))
	}
	o.ThroughAt, o.UpdatedAt = timeOf(through), timeOf(updated)
	return o, nil
}

// SetObservation refuses a deleted thread, so a log folded while it was
// being deleted does not bring its words back.
func (r memoryStateRepo) SetObservation(ctx context.Context, o model.ChatObservation) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getChatThread(ctx, tx, o.ThreadID); err != nil {
			return err
		}
		const qSetObservation = `INSERT INTO chat_observations (thread_id, through_id, through_at, log, reflections,
			updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (thread_id) DO UPDATE SET through_id = excluded.through_id, through_at = excluded.through_at,
				log = excluded.log, reflections = excluded.reflections, updated_at = excluded.updated_at`
		_, err := tx.ExecContext(ctx, qSetObservation, o.ThreadID, o.ThroughID, o.ThroughAt.UnixMilli(), o.Log,
			o.Reflections, r.db.Now().UnixMilli())
		return classify(err)
	})
	if err != nil {
		return fmt.Errorf("set observation log: %w", err)
	}
	return nil
}

// Notes.

// AddNote refuses a deleted thread, as SetObservation does.
func (r memoryStateRepo) AddNote(ctx context.Context, n model.ChatNote) error {
	refs, err := refsJSON(n.Refs)
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}
	created := n.CreatedAt
	if created.IsZero() {
		created = r.db.Now()
	}
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getChatThread(ctx, tx, n.ThreadID); err != nil {
			return err
		}
		const qAddNote = `INSERT INTO chat_notes (message_id, thread_id, note, refs, created_at) VALUES (?, ?, ?, ?, ?)`
		_, err := tx.ExecContext(ctx, qAddNote, n.MessageID, n.ThreadID, n.Note, refs, created.UnixMilli())
		return classify(err)
	})
	if err != nil {
		return fmt.Errorf("add note: %w", err)
	}
	return nil
}

func (r memoryStateRepo) Notes(ctx context.Context, threadID string) (map[string]model.ChatNote, error) {
	const qNotes = `SELECT message_id, thread_id, note, refs, created_at FROM chat_notes WHERE thread_id = ?`
	notes, err := queryList(ctx, r.db.sql, func(s scanner) (model.ChatNote, error) {
		var (
			n       model.ChatNote
			refs    string
			created int64
		)
		if err := s.Scan(&n.MessageID, &n.ThreadID, &n.Note, &refs, &created); err != nil {
			return model.ChatNote{}, err
		}
		var err error
		n.Refs, err = refsOf(refs)
		n.CreatedAt = timeOf(created)
		return n, err
	}, qNotes, threadID)
	if err != nil {
		return nil, fmt.Errorf("notes of chat %s: %w", threadID, err)
	}
	out := make(map[string]model.ChatNote, len(notes))
	for _, n := range notes {
		out[n.MessageID] = n
	}
	return out, nil
}

// Profile cards.

func (r memoryStateRepo) LatestProfile(ctx context.Context) (model.ProfileCard, error) {
	const qLatestProfile = `SELECT id, day, text, refs, created_at FROM profile_cards
		ORDER BY created_at DESC, rowid DESC LIMIT 1`
	var (
		c       model.ProfileCard
		refs    string
		created int64
	)
	err := r.db.sql.QueryRowContext(ctx, qLatestProfile).Scan(&c.ID, &c.Day, &c.Text, &refs, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return model.ProfileCard{}, Fail(ErrNotFound, "no profile card yet")
	}
	if err != nil {
		return model.ProfileCard{}, fmt.Errorf("latest profile card: %w", err)
	}
	if c.Refs, err = refsOf(refs); err != nil {
		return model.ProfileCard{}, fmt.Errorf("latest profile card: %w", err)
	}
	c.CreatedAt = timeOf(created)
	return c, nil
}

func (r memoryStateRepo) AddProfile(ctx context.Context, c model.ProfileCard) (model.ProfileCard, error) {
	if !validDay(c.Day) {
		return model.ProfileCard{}, FailField(ErrInvalid, "day", "a profile card's day is YYYY-MM-DD, not %q", c.Day)
	}
	refs, err := refsJSON(c.Refs)
	if err != nil {
		return model.ProfileCard{}, fmt.Errorf("add profile card: %w", err)
	}
	if c.ID == "" {
		c.ID = model.NewID()
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = r.db.Now()
	}
	c.CreatedAt = timeOf(c.CreatedAt.UnixMilli())
	if c.Refs == nil {
		c.Refs = map[string]string{}
	}
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		const qAddProfile = `INSERT INTO profile_cards (id, day, text, refs, created_at) VALUES (?, ?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, qAddProfile, c.ID, c.Day, c.Text, refs, c.CreatedAt.UnixMilli()); err != nil {
			return classify(err)
		}
		const qPruneProfiles = `DELETE FROM profile_cards WHERE id NOT IN
			(SELECT id FROM profile_cards ORDER BY created_at DESC, rowid DESC LIMIT ?)`
		_, err := tx.ExecContext(ctx, qPruneProfiles, keepProfiles)
		return err
	})
	if err != nil {
		return model.ProfileCard{}, fmt.Errorf("add profile card: %w", err)
	}
	return c, nil
}

// Jobs.

const jobCols = `id, kind, subject, payload, state, attempts, run_after, last_error, created_at, updated_at`

func scanJob(s scanner) (model.MemoryJob, error) {
	var (
		j                         model.MemoryJob
		payload                   string
		runAfter, created, update int64
	)
	if err := s.Scan(&j.ID, &j.Kind, &j.Subject, &payload, &j.State, &j.Attempts, &runAfter, &j.LastError, &created,
		&update); err != nil {
		return model.MemoryJob{}, err
	}
	j.Payload, j.RunAfter, j.CreatedAt, j.UpdatedAt = []byte(payload), timeOf(runAfter), timeOf(created), timeOf(update)
	return j, nil
}

func (r memoryStateRepo) Enqueue(ctx context.Context, jobs ...NewJob) error {
	if len(jobs) == 0 {
		return nil
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		for _, j := range jobs {
			if err := insertJob(ctx, tx, r.db.Now(), j); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("enqueue jobs: %w", err)
	}
	return nil
}

func (r memoryStateRepo) NextJob(ctx context.Context, now time.Time, kinds ...string) (model.MemoryJob, error) {
	var j model.MemoryJob
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		q := `SELECT ` + jobCols + ` FROM memory_jobs WHERE state = 'pending' AND run_after <= ?`
		args := []any{now.UnixMilli()}
		if len(kinds) > 0 {
			q += ` AND kind IN (` + placeholders(len(kinds)) + `)`
			args = append(args, stringArgs(kinds)...)
		}
		var err error
		j, err = scanJob(tx.QueryRowContext(ctx, q+` ORDER BY run_after, id LIMIT 1`, args...))
		if errors.Is(err, sql.ErrNoRows) {
			return Fail(ErrNotFound, "no memory job is due")
		}
		if err != nil {
			return err
		}
		j.State, j.Attempts, j.UpdatedAt = model.JobRunning, j.Attempts+1, r.db.Now()
		const qRunJob = `UPDATE memory_jobs SET state = 'running', attempts = ?, updated_at = ? WHERE id = ?`
		_, err = tx.ExecContext(ctx, qRunJob, j.Attempts, j.UpdatedAt.UnixMilli(), j.ID)
		return err
	})
	if err != nil {
		return model.MemoryJob{}, fmt.Errorf("next memory job: %w", err)
	}
	return j, nil
}

// endJob runs an UPDATE of one running job, whose last two arguments are
// updated_at and the id, and says why when no running job matched.
func (r memoryStateRepo) endJob(ctx context.Context, id int64, query string, args ...any) error {
	res, err := r.db.sql.ExecContext(ctx, query, append(args, r.db.Now().UnixMilli(), id)...)
	if err != nil {
		return classify(err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 1 {
		return err
	}
	var state string
	err = r.db.sql.QueryRowContext(ctx, `SELECT state FROM memory_jobs WHERE id = ?`, id).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return Fail(ErrNotFound, "no memory job %d", id)
	}
	if err != nil {
		return err
	}
	return Fail(ErrConflict, "memory job %d is %s, not running", id, state)
}

func (r memoryStateRepo) FinishJob(ctx context.Context, id int64) error {
	const qFinishJob = `UPDATE memory_jobs SET state = 'done', updated_at = ? WHERE id = ? AND state = 'running'`
	if err := r.endJob(ctx, id, qFinishJob); err != nil {
		return fmt.Errorf("finish memory job: %w", err)
	}
	return nil
}

func (r memoryStateRepo) FailJob(ctx context.Context, id int64, msg string, retryAt *time.Time) error {
	msg = cutRunes(msg, maxJobError)
	var err error
	if retryAt != nil {
		const qRetryJob = `UPDATE memory_jobs SET state = 'pending', run_after = ?, last_error = ?, updated_at = ?
			WHERE id = ? AND state = 'running'`
		err = r.endJob(ctx, id, qRetryJob, retryAt.UnixMilli(), msg)
	} else {
		const qFailJob = `UPDATE memory_jobs SET state = 'failed', last_error = ?, updated_at = ?
			WHERE id = ? AND state = 'running'`
		err = r.endJob(ctx, id, qFailJob, msg)
	}
	if err != nil {
		return fmt.Errorf("fail memory job: %w", err)
	}
	return nil
}

func (r memoryStateRepo) RequeueRunning(ctx context.Context) (int, error) {
	const qRequeue = `UPDATE memory_jobs SET state = 'pending', updated_at = ? WHERE state = 'running'`
	res, err := r.db.sql.ExecContext(ctx, qRequeue, r.db.Now().UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("requeue memory jobs: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func (r memoryStateRepo) PruneJobs(ctx context.Context, before time.Time) (int, error) {
	const qPruneJobs = `DELETE FROM memory_jobs WHERE state = 'done' AND updated_at < ?`
	res, err := r.db.sql.ExecContext(ctx, qPruneJobs, before.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("prune memory jobs: %w", err)
	}
	n, err := res.RowsAffected()
	return int(n), err
}

func (r memoryStateRepo) JobCounts(ctx context.Context) (map[string]int, error) {
	const qJobCounts = `SELECT state, count(*) FROM memory_jobs GROUP BY state`
	return countBy(ctx, r.db.sql, "count memory jobs", qJobCounts)
}

// Backends.

const backendCols = `backend, item_id, external_id, status, content_hash, error, synced_at, updated_at`

func scanBackend(s scanner) (model.MemoryBackend, error) {
	var (
		b       model.MemoryBackend
		synced  sql.NullInt64
		updated int64
	)
	if err := s.Scan(&b.Backend, &b.ItemID, &b.ExternalID, &b.Status, &b.ContentHash, &b.Error, &synced,
		&updated); err != nil {
		return model.MemoryBackend{}, err
	}
	b.SyncedAt, b.UpdatedAt = timePtr(synced), timeOf(updated)
	return b, nil
}

func (r memoryStateRepo) Backend(ctx context.Context, backend, itemID string) (model.MemoryBackend, error) {
	const qBackend = `SELECT ` + backendCols + ` FROM memory_backends WHERE backend = ? AND item_id = ?`
	b, err := scanBackend(r.db.sql.QueryRowContext(ctx, qBackend, backend, itemID))
	if err != nil {
		return model.MemoryBackend{}, fmt.Errorf("memory backend: %w", notFound(err, backend+" item", itemID))
	}
	return b, nil
}

func (r memoryStateRepo) SetBackend(ctx context.Context, b model.MemoryBackend) error {
	const qSetBackend = `INSERT INTO memory_backends (` + backendCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (backend, item_id) DO UPDATE SET external_id = excluded.external_id, status = excluded.status,
			content_hash = excluded.content_hash, error = excluded.error, synced_at = excluded.synced_at,
			updated_at = excluded.updated_at`
	_, err := r.db.sql.ExecContext(ctx, qSetBackend, b.Backend, b.ItemID, b.ExternalID, b.Status, b.ContentHash,
		b.Error, nullMillis(b.SyncedAt), r.db.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("set memory backend: %w", classify(err))
	}
	return nil
}

func (r memoryStateRepo) Backends(ctx context.Context, backend, status string) ([]model.MemoryBackend, error) {
	const qBackends = `SELECT ` + backendCols + ` FROM memory_backends WHERE backend = ? AND (? = '' OR status = ?)
		ORDER BY item_id`
	out, err := queryList(ctx, r.db.sql, scanBackend, qBackends, backend, status, status)
	if err != nil {
		return nil, fmt.Errorf("memory backends: %w", err)
	}
	return out, nil
}

func (r memoryStateRepo) BackendByExternal(ctx context.Context, backend,
	externalID string) (model.MemoryBackend, error) {
	if externalID == "" {
		return model.MemoryBackend{}, Fail(ErrNotFound, "no %s item without an external id", backend)
	}
	const qBackendByExternal = `SELECT ` + backendCols + ` FROM memory_backends WHERE backend = ? AND external_id = ?
		ORDER BY updated_at DESC LIMIT 1`
	b, err := scanBackend(r.db.sql.QueryRowContext(ctx, qBackendByExternal, backend, externalID))
	if err != nil {
		return model.MemoryBackend{}, fmt.Errorf("memory backend: %w", notFound(err, backend+" item", externalID))
	}
	return b, nil
}

// Usage.

// usageChunk bounds the ids of one query.
const usageChunk = 500

func (r memoryStateRepo) TouchClaims(ctx context.Context, ids []string, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	seen := map[string]bool{}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		const qTouchClaim = `INSERT INTO memory_usage (claim_id, uses, last_used_at) VALUES (?, 1, ?)
			ON CONFLICT (claim_id) DO UPDATE SET uses = uses + 1, last_used_at = max(last_used_at, excluded.last_used_at)`
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			if _, err := tx.ExecContext(ctx, qTouchClaim, id, at.UnixMilli()); err != nil {
				return classify(err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("count claim uses: %w", err)
	}
	return nil
}

func (r memoryStateRepo) Usage(ctx context.Context, ids []string) (map[string]ClaimUsage, error) {
	out := map[string]ClaimUsage{}
	for start := 0; start < len(ids); start += usageChunk {
		chunk := ids[start:min(start+usageChunk, len(ids))]
		q := `SELECT claim_id, uses, last_used_at FROM memory_usage WHERE claim_id IN (` + placeholders(len(chunk)) + `)`
		rows, err := r.db.sql.QueryContext(ctx, q, stringArgs(chunk)...)
		if err != nil {
			return nil, fmt.Errorf("claim uses: %w", err)
		}
		for rows.Next() {
			var (
				id   string
				u    ClaimUsage
				last int64
			)
			if err := rows.Scan(&id, &u.Uses, &last); err != nil {
				rows.Close()
				return nil, fmt.Errorf("claim uses: %w", err)
			}
			u.LastUsedAt = timeOf(last)
			out[id] = u
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, fmt.Errorf("claim uses: %w", err)
		}
	}
	return out, nil
}

// Resources.

const resourceCols = `id, title, path, mime, size, content_hash, episode_id, document_at, added_at, deleted_at`

func scanResource(s scanner) (model.Resource, error) {
	var (
		res               model.Resource
		episode           sql.NullString
		document, deleted sql.NullInt64
		added             int64
	)
	if err := s.Scan(&res.ID, &res.Title, &res.Path, &res.Mime, &res.Size, &res.ContentHash, &episode, &document,
		&added, &deleted); err != nil {
		return model.Resource{}, err
	}
	res.EpisodeID, res.DocumentAt, res.AddedAt, res.DeletedAt = stringPtr(episode), timePtr(document), timeOf(added),
		timePtr(deleted)
	return res, nil
}

func getResource(ctx context.Context, q Querier, id string) (model.Resource, error) {
	const qResource = `SELECT ` + resourceCols + ` FROM resources WHERE id = ? AND deleted_at IS NULL`
	res, err := scanResource(q.QueryRowContext(ctx, qResource, id))
	if err != nil {
		return model.Resource{}, fmt.Errorf("get document: %w", notFound(err, "document", id))
	}
	return res, nil
}

// AddResource stores text as an episode of kind resource, titled as the
// resource when it has no title of its own and naming the resource as its
// source when it has none.
func (r memoryStateRepo) AddResource(ctx context.Context, res model.Resource,
	text model.Episode) (model.Resource, error) {
	if res.ID == "" {
		res.ID = model.NewID()
	}
	if res.AddedAt.IsZero() {
		res.AddedAt = r.db.Now()
	}
	res.AddedAt, res.DeletedAt = timeOf(res.AddedAt.UnixMilli()), nil
	if res.DocumentAt != nil {
		d := timeOf(res.DocumentAt.UnixMilli())
		res.DocumentAt = &d
	}
	text.Kind = model.EpisodeResource
	if text.Title == "" {
		text.Title = res.Title
	}
	if text.Source == "" {
		text.Source = res.ID
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		ep, err := insertEpisode(ctx, tx, r.db, text)
		if err != nil {
			return err
		}
		res.EpisodeID = &ep.ID
		if res.ContentHash == "" {
			res.ContentHash = ep.ContentHash
		}
		const qAddResource = `INSERT INTO resources (` + resourceCols + `) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)`
		_, err = tx.ExecContext(ctx, qAddResource, res.ID, res.Title, res.Path, res.Mime, res.Size, res.ContentHash,
			ep.ID, nullMillis(res.DocumentAt), res.AddedAt.UnixMilli())
		return classify(err)
	})
	if err != nil {
		return model.Resource{}, fmt.Errorf("add document: %w", err)
	}
	return res, nil
}

func (r memoryStateRepo) Resources(ctx context.Context) ([]model.Resource, error) {
	const qResources = `SELECT ` + resourceCols + ` FROM resources WHERE deleted_at IS NULL
		ORDER BY added_at DESC, id DESC`
	out, err := queryList(ctx, r.db.sql, scanResource, qResources)
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	return out, nil
}

func (r memoryStateRepo) Resource(ctx context.Context, id string) (model.Resource, error) {
	return getResource(ctx, r.db.sql, id)
}

// RemoveResource keeps the resource as a tombstone whose title and path are
// erased, and erases its episode.
func (r memoryStateRepo) RemoveResource(ctx context.Context, id string, ledger LedgerItem) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		res, err := getResource(ctx, tx, id)
		if err != nil {
			return err
		}
		const qRemoveResource = `UPDATE resources SET title = '', path = '', deleted_at = ? WHERE id = ?`
		if _, err := tx.ExecContext(ctx, qRemoveResource, r.db.Now().UnixMilli(), id); err != nil {
			return err
		}
		if res.EpisodeID != nil {
			if err := eraseEpisode(ctx, tx, r.db, *res.EpisodeID); err != nil {
				return err
			}
		}
		if err := optimizeIndex(ctx, tx, "episodes_fts"); err != nil {
			return err
		}
		if ledger.Data == nil {
			ledger.Data = map[string]any{"action": "resource.remove", "resource": id, "hash": res.ContentHash}
		}
		return appendLedger(ctx, tx, r.db, ledger)
	})
	if err != nil {
		return fmt.Errorf("remove document %s: %w", id, err)
	}
	checkpoint(ctx, r.db, "document removed")
	return nil
}
