package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/model"
)

// attachmentsVersion is the schema version that added task_attachments.
const attachmentsVersion = 15

// Limits on what an attachment holds.
const (
	maxAttachmentTitle = 300
	maxAttachmentURL   = 4000
	// MaxAttachmentBody is the longest note or app snapshot kept, in runes;
	// longer text is cut there.
	MaxAttachmentBody = 20000
)

// AttachmentApps are the apps an attachment can come from, by Composio slug.
var AttachmentApps = []string{"gmail", "googlecalendar", "github", "notion"}

// NewAttachment is an attachment to add to a task. An empty Title takes one
// from the rest: the link, the note's first line, or the file's name.
type NewAttachment struct {
	TaskID    string
	Kind      string
	Title     string
	URL       string
	Path      string
	App       string
	Ref       string
	Body      string
	FetchedAt *time.Time
}

// AttachmentPatch changes an attachment; unset fields are unchanged. A
// refreshed app item sets Body and FetchedAt together.
type AttachmentPatch struct {
	Title     *string
	URL       *string
	Body      *string
	FetchedAt *time.Time
	Rev       *int64
}

// AttachmentRepo reads and writes task attachments.
type AttachmentRepo interface {
	// List returns a task's live attachments, oldest first.
	List(ctx context.Context, taskID string) ([]model.Attachment, error)
	// Counts returns how many live attachments each of these tasks has;
	// tasks with none are left out.
	Counts(ctx context.Context, taskIDs []string) (map[string]int, error)
	// Of returns the live attachments of these tasks by task, oldest first.
	Of(ctx context.Context, taskIDs []string) (map[string][]model.Attachment, error)
	Get(ctx context.Context, id string) (model.Attachment, error)
	Create(ctx context.Context, a NewAttachment) (model.Attachment, error)
	Update(ctx context.Context, id string, p AttachmentPatch) (model.Attachment, error)
	Delete(ctx context.Context, id string) error
}

type attachmentRepo struct{ db *DB }

// NewAttachmentRepo returns the SQLite AttachmentRepo.
func NewAttachmentRepo(db *DB) AttachmentRepo { return attachmentRepo{db} }

const attachmentCols = `id, task_id, kind, title, url, path, app, ref, body, fetched_at, ` + envelopeCols

func scanAttachment(s scanner) (model.Attachment, error) {
	var (
		a       model.Attachment
		fetched sql.NullInt64
		env     envelopeScan
	)
	dest := append([]any{&a.ID, &a.TaskID, &a.Kind, &a.Title, &a.URL, &a.Path, &a.App, &a.Ref, &a.Body, &fetched},
		env.dest()...)
	if err := s.Scan(dest...); err != nil {
		return model.Attachment{}, err
	}
	a.FetchedAt = timePtr(fetched)
	a.Envelope = env.envelope()
	return a, nil
}

func (r attachmentRepo) List(ctx context.Context, taskID string) ([]model.Attachment, error) {
	const qListAttachments = `SELECT ` + attachmentCols + ` FROM task_attachments
		WHERE task_id = ? AND deleted_at IS NULL ORDER BY created_at, id`
	rows, err := r.db.sql.QueryContext(ctx, qListAttachments, taskID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	defer rows.Close()
	out := []model.Attachment{}
	for rows.Next() {
		a, err := scanAttachment(rows)
		if err != nil {
			return nil, fmt.Errorf("list attachments: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r attachmentRepo) Counts(ctx context.Context, taskIDs []string) (map[string]int, error) {
	out := map[string]int{}
	// SQLite's default limit on bound parameters is far above a task list,
	// but keep each query well under it anyway.
	for start := 0; start < len(taskIDs); start += 500 {
		ids := taskIDs[start:min(start+500, len(taskIDs))]
		q := `SELECT task_id, count(*) FROM task_attachments WHERE deleted_at IS NULL AND task_id IN (?` +
			strings.Repeat(", ?", len(ids)-1) + `) GROUP BY task_id`
		rows, err := r.db.sql.QueryContext(ctx, q, anys(ids)...)
		if err != nil {
			return nil, fmt.Errorf("count attachments: %w", err)
		}
		for rows.Next() {
			var id string
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				rows.Close()
				return nil, fmt.Errorf("count attachments: %w", err)
			}
			out[id] = n
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("count attachments: %w", err)
		}
	}
	return out, nil
}

func (r attachmentRepo) Of(ctx context.Context, taskIDs []string) (map[string][]model.Attachment, error) {
	out := map[string][]model.Attachment{}
	for start := 0; start < len(taskIDs); start += 500 {
		ids := taskIDs[start:min(start+500, len(taskIDs))]
		q := `SELECT ` + attachmentCols + ` FROM task_attachments WHERE deleted_at IS NULL AND task_id IN (?` +
			strings.Repeat(", ?", len(ids)-1) + `) ORDER BY created_at, id`
		rows, err := r.db.sql.QueryContext(ctx, q, anys(ids)...)
		if err != nil {
			return nil, fmt.Errorf("list attachments: %w", err)
		}
		for rows.Next() {
			a, err := scanAttachment(rows)
			if err != nil {
				rows.Close()
				return nil, fmt.Errorf("list attachments: %w", err)
			}
			out[a.TaskID] = append(out[a.TaskID], a)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("list attachments: %w", err)
		}
	}
	return out, nil
}

func (r attachmentRepo) Get(ctx context.Context, id string) (model.Attachment, error) {
	return getAttachment(ctx, r.db.sql, id)
}

func getAttachment(ctx context.Context, q Querier, id string) (model.Attachment, error) {
	const qGetAttachment = `SELECT ` + attachmentCols + ` FROM task_attachments WHERE id = ? AND deleted_at IS NULL`
	a, err := scanAttachment(q.QueryRowContext(ctx, qGetAttachment, id))
	if err != nil {
		return model.Attachment{}, fmt.Errorf("get attachment: %w", notFound(err, "attachment", id))
	}
	return a, nil
}

func (r attachmentRepo) Create(ctx context.Context, na NewAttachment) (model.Attachment, error) {
	now := r.db.Now()
	a := model.Attachment{
		ID: model.NewID(), TaskID: na.TaskID, Kind: na.Kind, Title: na.Title, URL: na.URL, Path: na.Path, App: na.App,
		Ref: na.Ref, Body: na.Body, FetchedAt: na.FetchedAt,
		Envelope: model.Envelope{CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1},
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getTask(ctx, tx, a.TaskID); err != nil {
			return FailField(ErrNotFound, "task_id", "no task %s", a.TaskID)
		}
		if err := validAttachment(&a); err != nil {
			return err
		}
		const qInsertAttachment = `INSERT INTO task_attachments (id, task_id, kind, title, url, path, app, ref, body,
			fetched_at, created_at, updated_at, device_id, rev) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
		_, err := tx.ExecContext(ctx, qInsertAttachment, a.ID, a.TaskID, a.Kind, a.Title, a.URL, a.Path, a.App, a.Ref,
			a.Body, nullMillis(a.FetchedAt), now.UnixMilli(), now.UnixMilli(), a.DeviceID)
		return classify(err)
	})
	if err != nil {
		return model.Attachment{}, fmt.Errorf("create attachment: %w", err)
	}
	return a, nil
}

func (r attachmentRepo) Update(ctx context.Context, id string, p AttachmentPatch) (model.Attachment, error) {
	var a model.Attachment
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if a, err = getAttachment(ctx, tx, id); err != nil {
			return err
		}
		if err := checkRev(p.Rev, a.Rev); err != nil {
			return err
		}
		if p.Title != nil {
			a.Title = *p.Title
		}
		if p.URL != nil {
			a.URL = *p.URL
		}
		if p.Body != nil {
			a.Body = *p.Body
		}
		if p.FetchedAt != nil {
			a.FetchedAt = p.FetchedAt
		}
		if err := validAttachment(&a); err != nil {
			return err
		}
		now := r.db.Now()
		a.UpdatedAt, a.DeviceID, a.Rev = now, r.db.DeviceID(), a.Rev+1
		const qUpdateAttachment = `UPDATE task_attachments SET title = ?, url = ?, body = ?, fetched_at = ?,
			updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
		_, err = tx.ExecContext(ctx, qUpdateAttachment, a.Title, a.URL, a.Body, nullMillis(a.FetchedAt),
			now.UnixMilli(), a.DeviceID, a.Rev, a.ID)
		return classify(err)
	})
	if err != nil {
		return model.Attachment{}, fmt.Errorf("update attachment: %w", err)
	}
	return a, nil
}

func (r attachmentRepo) Delete(ctx context.Context, id string) error {
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getAttachment(ctx, tx, id); err != nil {
			return err
		}
		return softDelete(ctx, tx, r.db, "task_attachments", id)
	})
	if err != nil {
		return fmt.Errorf("delete attachment: %w", err)
	}
	return nil
}

// deleteAttachments tombstones a deleted task's attachments.
func deleteAttachments(ctx context.Context, tx *sql.Tx, db *DB, taskID string) error {
	now := db.Now().UnixMilli()
	const qDeleteAttachments = `UPDATE task_attachments SET deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1
		WHERE task_id = ? AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, qDeleteAttachments, now, now, db.DeviceID(), taskID)
	return err
}

// validAttachment checks and normalizes an attachment's fields before a write.
func validAttachment(a *model.Attachment) error {
	a.Title, a.URL, a.Path, a.App, a.Ref = strings.TrimSpace(a.Title), strings.TrimSpace(a.URL),
		strings.TrimSpace(a.Path), strings.TrimSpace(a.App), strings.TrimSpace(a.Ref)
	a.Body = cutRunes(strings.TrimSpace(a.Body), MaxAttachmentBody)
	if a.URL != "" {
		if len(a.URL) > maxAttachmentURL {
			return FailField(ErrInvalid, "url", "a link is at most %d characters", maxAttachmentURL)
		}
		u, err := url.Parse(a.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return FailField(ErrInvalid, "url", "a link must start with http:// or https://")
		}
	}
	switch a.Kind {
	case model.AttachLink:
		if a.URL == "" {
			return FailField(ErrInvalid, "url", "a link needs its address")
		}
		if a.Title == "" {
			u, _ := url.Parse(a.URL)
			a.Title = strings.TrimSuffix(u.Host+u.Path, "/")
		}
	case model.AttachNote:
		if a.Body == "" {
			return FailField(ErrInvalid, "body", "a note needs some text")
		}
		if a.Title == "" {
			a.Title, _, _ = strings.Cut(a.Body, "\n")
		}
	case model.AttachFile:
		if !filepath.IsAbs(a.Path) {
			return FailField(ErrInvalid, "path", "a file needs its full path")
		}
		if a.Title == "" {
			a.Title = filepath.Base(a.Path)
		}
	case model.AttachApp:
		if !slices.Contains(AttachmentApps, a.App) {
			return FailField(ErrInvalid, "app", "app must be one of %s", strings.Join(AttachmentApps, ", "))
		}
		if a.Ref == "" {
			return FailField(ErrInvalid, "ref", "an app item needs its id in the app")
		}
	default:
		return FailField(ErrInvalid, "kind", "kind must be link, note, file, or app")
	}
	a.Title = cutRunes(strings.TrimSpace(a.Title), maxAttachmentTitle)
	if a.Title == "" {
		return FailField(ErrInvalid, "title", "an attachment needs a title")
	}
	return nil
}
