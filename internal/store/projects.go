package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/kzark/gwen/internal/model"
)

// Palette is the colours assigned to projects created without one
// (docs/04-api-contract.md#project).
var Palette = []string{"#3b82f6", "#10b981", "#f59e0b", "#ef4444", "#8b5cf6", "#ec4899", "#14b8a6", "#f97316"}

// Archive filters for listing projects.
const (
	ArchivedExclude = "false" // live, not archived (the default)
	ArchivedOnly    = "true"
	ArchivedAll     = "all"
)

// ProjectPatch changes a project; nil fields are unchanged.
type ProjectPatch struct {
	Name     *string
	Color    *string
	Archived *bool
	Rev      *int64
}

// ProjectRepo reads and writes projects.
type ProjectRepo interface {
	// List returns live projects matching the archive filter, by name ignoring case.
	List(ctx context.Context, archived string) ([]model.Project, error)
	Get(ctx context.Context, id string) (model.Project, error)
	// Create inserts a project; a nil color takes the next palette colour.
	Create(ctx context.Context, name string, color *string) (model.Project, error)
	Update(ctx context.Context, id string, p ProjectPatch) (model.Project, error)
	// Delete soft-deletes the project, its tasks, and its goals in one
	// transaction (docs/06-planner.md#deleting).
	Delete(ctx context.Context, id string) (Changes, error)
}

type projectRepo struct{ db *DB }

// NewProjectRepo returns the SQLite ProjectRepo.
func NewProjectRepo(db *DB) ProjectRepo { return projectRepo{db} }

const projectCols = `id, name, color, archived_at, ` + envelopeCols

func scanProject(s scanner) (model.Project, error) {
	var (
		p        model.Project
		archived sql.NullInt64
		env      envelopeScan
	)
	if err := s.Scan(append([]any{&p.ID, &p.Name, &p.Color, &archived}, env.dest()...)...); err != nil {
		return model.Project{}, err
	}
	p.ArchivedAt = timePtr(archived)
	p.Envelope = env.envelope()
	return p, nil
}

func (r projectRepo) List(ctx context.Context, archived string) ([]model.Project, error) {
	const qListProjects = `SELECT ` + projectCols + ` FROM projects WHERE deleted_at IS NULL`
	q := qListProjects
	switch archived {
	case ArchivedExclude, "":
		q += ` AND archived_at IS NULL`
	case ArchivedOnly:
		q += ` AND archived_at IS NOT NULL`
	case ArchivedAll:
	default:
		return nil, FailField(ErrInvalid, "archived", "archived must be false, true, or all")
	}
	rows, err := r.db.sql.QueryContext(ctx, q+` ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	out := []model.Project{}
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r projectRepo) Get(ctx context.Context, id string) (model.Project, error) {
	return getProject(ctx, r.db.sql, id)
}

func getProject(ctx context.Context, q Querier, id string) (model.Project, error) {
	const qGetProject = `SELECT ` + projectCols + ` FROM projects WHERE id = ? AND deleted_at IS NULL`
	p, err := scanProject(q.QueryRowContext(ctx, qGetProject, id))
	if err != nil {
		return model.Project{}, fmt.Errorf("get project: %w", notFound(err, "project", id))
	}
	return p, nil
}

func (r projectRepo) Create(ctx context.Context, name string, color *string) (model.Project, error) {
	name, err := validProjectName(name)
	if err != nil {
		return model.Project{}, err
	}
	var p model.Project
	err = r.db.InTx(ctx, func(tx *sql.Tx) error {
		c, err := r.pickColor(ctx, tx, color)
		if err != nil {
			return err
		}
		if err := checkNameFree(ctx, tx, name, ""); err != nil {
			return err
		}
		now := r.db.Now()
		p = model.Project{ID: model.NewID(), Name: name, Color: c, Envelope: model.Envelope{
			CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1,
		}}
		const qInsertProject = `INSERT INTO projects (id, name, color, archived_at, created_at, updated_at, device_id, rev)
			VALUES (?, ?, ?, NULL, ?, ?, ?, 1)`
		_, err = tx.ExecContext(ctx, qInsertProject, p.ID, p.Name, p.Color, now.UnixMilli(), now.UnixMilli(), p.DeviceID)
		return classify(err)
	})
	if err != nil {
		return model.Project{}, fmt.Errorf("create project: %w", err)
	}
	return p, nil
}

// pickColor validates a given colour, or takes the palette colour used by the
// fewest live projects, earliest in the palette on a tie.
func (r projectRepo) pickColor(ctx context.Context, tx *sql.Tx, color *string) (string, error) {
	if color != nil {
		c, ok := validColor(*color)
		if !ok {
			return "", FailField(ErrInvalid, "color", "color must look like #3b82f6")
		}
		return c, nil
	}
	const qColorUse = `SELECT color, count(*) FROM projects WHERE deleted_at IS NULL GROUP BY color`
	rows, err := tx.QueryContext(ctx, qColorUse)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	used := map[string]int{}
	for rows.Next() {
		var c string
		var n int
		if err := rows.Scan(&c, &n); err != nil {
			return "", err
		}
		used[c] = n
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	best := Palette[0]
	for _, c := range Palette[1:] {
		if used[c] < used[best] {
			best = c
		}
	}
	return best, nil
}

func validProjectName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if n := len([]rune(name)); n < 1 || n > 80 {
		return "", FailField(ErrInvalid, "name", "a project name must be 1 to 80 characters")
	}
	return name, nil
}

// checkNameFree reports a live project other than except with this name,
// ignoring case, as a conflict.
func checkNameFree(ctx context.Context, q Querier, name, except string) error {
	const qNameTaken = `SELECT count(*) FROM projects
		WHERE name = ? COLLATE NOCASE AND deleted_at IS NULL AND id <> ?`
	var n int
	if err := q.QueryRowContext(ctx, qNameTaken, name, except).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return FailField(ErrConflict, "name", "a project named %q already exists", name)
	}
	return nil
}

func (r projectRepo) Update(ctx context.Context, id string, patch ProjectPatch) (model.Project, error) {
	var p model.Project
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		var err error
		if p, err = getProject(ctx, tx, id); err != nil {
			return err
		}
		if err := checkRev(patch.Rev, p.Rev); err != nil {
			return err
		}
		now := r.db.Now()
		if patch.Name != nil {
			if p.Name, err = validProjectName(*patch.Name); err != nil {
				return err
			}
			if err := checkNameFree(ctx, tx, p.Name, id); err != nil {
				return err
			}
		}
		if patch.Color != nil {
			c, ok := validColor(*patch.Color)
			if !ok {
				return FailField(ErrInvalid, "color", "color must look like #3b82f6")
			}
			p.Color = c
		}
		if patch.Archived != nil {
			switch {
			case *patch.Archived && p.ArchivedAt == nil:
				p.ArchivedAt = &now
			case !*patch.Archived:
				p.ArchivedAt = nil
			}
		}
		p.UpdatedAt, p.DeviceID, p.Rev = now, r.db.DeviceID(), p.Rev+1
		const qUpdateProject = `UPDATE projects SET name = ?, color = ?, archived_at = ?,
			updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
		_, err = tx.ExecContext(ctx, qUpdateProject, p.Name, p.Color, nullMillis(p.ArchivedAt),
			now.UnixMilli(), p.DeviceID, p.Rev, id)
		return classify(err)
	})
	if err != nil {
		return model.Project{}, fmt.Errorf("update project %s: %w", id, err)
	}
	return p, nil
}

func (r projectRepo) Delete(ctx context.Context, id string) (Changes, error) {
	var ch Changes
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getProject(ctx, tx, id); err != nil {
			return err
		}
		const qProjectGoals = `SELECT ` + goalCols + ` FROM goals WHERE deleted_at IS NULL AND project_id = ?
			ORDER BY created_at, id`
		goals, err := queryGoals(ctx, tx, qProjectGoals, id)
		if err != nil {
			return err
		}
		for _, g := range goals {
			if err := deleteGoal(ctx, tx, r.db, g, true, &ch); err != nil {
				return err
			}
		}
		const qProjectTasks = `SELECT ` + taskCols + ` FROM tasks WHERE deleted_at IS NULL AND project_id = ?
			ORDER BY created_at, id`
		tasks, err := queryTasks(ctx, tx, qProjectTasks, id)
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if err := deleteTask(ctx, tx, r.db, t, &ch); err != nil {
				return err
			}
		}
		return softDelete(ctx, tx, r.db, "projects", id)
	})
	if err != nil {
		return Changes{}, fmt.Errorf("delete project %s: %w", id, err)
	}
	return ch, nil
}

// projectExists checks that a live project exists, for foreign keys.
func projectExists(ctx context.Context, q Querier, id string, field string) error {
	const qProjectExists = `SELECT count(*) FROM projects WHERE id = ? AND deleted_at IS NULL`
	var n int
	if err := q.QueryRowContext(ctx, qProjectExists, id).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return FailField(ErrNotFound, field, "no project %s", id)
	}
	return nil
}
