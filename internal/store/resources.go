package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/kzark/gwen/internal/model"
)

// Limits on resources and routines.
const (
	MaxResources    = 30
	MaxRoutineSteps = 12
	maxResourceText = 4000
)

// Resource is a link, folder, file, or note attached to a subject.
type Resource struct {
	ID       string
	Subject  string
	Kind     string // url, folder, file, or note
	Title    string
	Value    string
	Position int
	model.Envelope
}

// NewResource is one resource of a subject's new set.
type NewResource struct {
	Kind  string
	Title string
	Value string
}

// RoutineStep is one "when it starts" step: a capability and its input.
type RoutineStep struct {
	Title      string          `json:"title,omitempty"`
	Capability string          `json:"capability"`
	Input      json.RawMessage `json:"input"`
}

// Routine is what a subject's start does.
type Routine struct {
	ID        string
	Subject   string
	Steps     []RoutineStep
	AutoStart bool
	model.Envelope
}

// SubjectRepo keeps the resources and routines of subjects. Both sync.
type SubjectRepo interface {
	// Resources are a subject's resources in order.
	Resources(ctx context.Context, subject string) ([]Resource, error)
	// PutResources replaces a subject's resources with these, in order.
	PutResources(ctx context.Context, subject string, rs []NewResource) ([]Resource, error)
	// Routine is a subject's routine, ErrNotFound when it has none.
	Routine(ctx context.Context, subject string) (Routine, error)
	// PutRoutine sets a subject's routine; no steps removes it.
	PutRoutine(ctx context.Context, subject string, steps []RoutineStep, autoStart bool) (Routine, error)
}

type subjectRepo struct{ db *DB }

// NewSubjectRepo returns the SQLite SubjectRepo.
func NewSubjectRepo(db *DB) SubjectRepo { return subjectRepo{db} }

const subjectResourceCols = `id, subject, kind, title, value, position, ` + envelopeCols

func (r subjectRepo) Resources(ctx context.Context, subject string) ([]Resource, error) {
	return resourcesOf(ctx, r.db.sql, subject)
}

func resourcesOf(ctx context.Context, q Querier, subject string) ([]Resource, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+subjectResourceCols+` FROM subject_resources WHERE subject = ? AND deleted_at IS NULL
		ORDER BY position, id`, subject)
	if err != nil {
		return nil, fmt.Errorf("list resources: %w", err)
	}
	defer rows.Close()
	out := []Resource{}
	for rows.Next() {
		var (
			res Resource
			env envelopeScan
		)
		if err := rows.Scan(append([]any{&res.ID, &res.Subject, &res.Kind, &res.Title, &res.Value, &res.Position},
			env.dest()...)...); err != nil {
			return nil, fmt.Errorf("list resources: %w", err)
		}
		res.Envelope = env.envelope()
		out = append(out, res)
	}
	return out, rows.Err()
}

func validResource(i int, n *NewResource) error {
	field := fmt.Sprintf("resources[%d]", i)
	n.Title, n.Value = strings.TrimSpace(n.Title), strings.TrimSpace(n.Value)
	if n.Value == "" || len(n.Value) > maxResourceText || len([]rune(n.Title)) > 300 {
		return FailField(ErrInvalid, field, "a resource has a value of 1 to %d characters and a title of at most 300", maxResourceText)
	}
	switch n.Kind {
	case "url":
		u, err := url.Parse(n.Value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return FailField(ErrInvalid, field, "%q is not an http or https link", n.Value)
		}
	case "folder", "file":
		if !filepath.IsAbs(n.Value) && !strings.HasPrefix(n.Value, "~/") {
			return FailField(ErrInvalid, field, "%q is not a full path", n.Value)
		}
	case "note":
	default:
		return FailField(ErrInvalid, field, "kind must be url, folder, file, or note")
	}
	return nil
}

func (r subjectRepo) PutResources(ctx context.Context, subject string, rs []NewResource) ([]Resource, error) {
	if len(rs) > MaxResources {
		return nil, FailField(ErrInvalid, "resources", "a subject has at most %d resources", MaxResources)
	}
	for i := range rs {
		if err := validResource(i, &rs[i]); err != nil {
			return nil, err
		}
	}
	var out []Resource
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		now := r.db.Now().UnixMilli()
		if _, err := tx.ExecContext(ctx, `UPDATE subject_resources SET deleted_at = ?, updated_at = ?, device_id = ?, rev = rev + 1
			WHERE subject = ? AND deleted_at IS NULL`, now, now, r.db.DeviceID(), subject); err != nil {
			return err
		}
		for i, n := range rs {
			const q = `INSERT INTO subject_resources (id, subject, kind, title, value, position, created_at, updated_at,
				device_id, rev) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`
			if _, err := tx.ExecContext(ctx, q, model.NewID(), subject, n.Kind, n.Title, n.Value, i, now, now,
				r.db.DeviceID()); err != nil {
				return classify(err)
			}
		}
		var err error
		out, err = resourcesOf(ctx, tx, subject)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("put resources: %w", err)
	}
	return out, nil
}

const routineCols = `id, subject, steps, auto_start, ` + envelopeCols

func scanRoutine(s scanner) (Routine, error) {
	var (
		rt    Routine
		steps string
		auto  int
		env   envelopeScan
	)
	if err := s.Scan(append([]any{&rt.ID, &rt.Subject, &steps, &auto}, env.dest()...)...); err != nil {
		return Routine{}, err
	}
	if err := json.Unmarshal([]byte(steps), &rt.Steps); err != nil || rt.Steps == nil {
		rt.Steps = []RoutineStep{}
	}
	rt.AutoStart, rt.Envelope = auto == 1, env.envelope()
	return rt, nil
}

func (r subjectRepo) Routine(ctx context.Context, subject string) (Routine, error) {
	rt, err := scanRoutine(r.db.sql.QueryRowContext(ctx, `SELECT `+routineCols+` FROM routines WHERE subject = ? AND deleted_at IS NULL`,
		subject))
	if err != nil {
		return Routine{}, fmt.Errorf("get routine: %w", notFound(err, "routine for", subject))
	}
	return rt, nil
}

func (r subjectRepo) PutRoutine(ctx context.Context, subject string, steps []RoutineStep, autoStart bool) (Routine, error) {
	if len(steps) > MaxRoutineSteps {
		return Routine{}, FailField(ErrInvalid, "steps", "a routine has at most %d steps", MaxRoutineSteps)
	}
	for i, st := range steps {
		if st.Capability == "" || (len(st.Input) > 0 && !json.Valid(st.Input)) || len([]rune(st.Title)) > 200 {
			return Routine{}, FailField(ErrInvalid, fmt.Sprintf("steps[%d]", i), "a step names a capability and its input")
		}
		if len(st.Input) == 0 {
			steps[i].Input = json.RawMessage(`{}`)
		}
	}
	raw, err := json.Marshal(steps)
	if err != nil {
		return Routine{}, err
	}
	var rt Routine
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		now := r.db.Now().UnixMilli()
		cur, err := scanRoutine(tx.QueryRowContext(ctx, `SELECT `+routineCols+` FROM routines WHERE subject = ? AND deleted_at IS NULL`,
			subject))
		switch {
		case err == sql.ErrNoRows && len(steps) == 0:
			rt = Routine{Subject: subject, Steps: []RoutineStep{}}
			return nil
		case err == sql.ErrNoRows:
			const q = `INSERT INTO routines (id, subject, steps, auto_start, created_at, updated_at, device_id, rev)
				VALUES (?, ?, ?, ?, ?, ?, ?, 1)`
			if _, err := tx.ExecContext(ctx, q, model.NewID(), subject, string(raw), boolInt(autoStart), now, now,
				r.db.DeviceID()); err != nil {
				return classify(err)
			}
		case err != nil:
			return err
		case len(steps) == 0:
			if err := softDelete(ctx, tx, r.db, "routines", cur.ID); err != nil {
				return err
			}
			rt = Routine{Subject: subject, Steps: []RoutineStep{}}
			return nil
		default:
			if _, err := tx.ExecContext(ctx, `UPDATE routines SET steps = ?, auto_start = ?, updated_at = ?, device_id = ?,
				rev = rev + 1 WHERE id = ?`, string(raw), boolInt(autoStart), now, r.db.DeviceID(), cur.ID); err != nil {
				return classify(err)
			}
		}
		rt, err = scanRoutine(tx.QueryRowContext(ctx, `SELECT `+routineCols+` FROM routines WHERE subject = ? AND deleted_at IS NULL`,
			subject))
		return err
	})
	if err != nil {
		return Routine{}, fmt.Errorf("put routine: %w", err)
	}
	return rt, nil
}
