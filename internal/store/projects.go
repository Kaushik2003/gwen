package store

import (
	"context"
	"database/sql"
	"errors"
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

// projectParentVersion is the schema version that gave projects a parent.
const projectParentVersion = 14

// ProjectPatch changes a project; nil and unset fields are unchanged.
type ProjectPatch struct {
	Name     *string
	Color    *string
	Archived *bool
	ParentID Nullable[string]
	Rev      *int64
}

// ProjectRepo reads and writes projects.
type ProjectRepo interface {
	// List returns live projects matching the archive filter, by name ignoring case.
	List(ctx context.Context, archived string) ([]model.Project, error)
	Get(ctx context.Context, id string) (model.Project, error)
	// Create inserts a project; a nil color takes the next palette colour,
	// and a nil parentID puts it at the top level.
	Create(ctx context.Context, name string, color, parentID *string) (model.Project, error)
	Update(ctx context.Context, id string, p ProjectPatch) (model.Project, error)
	// Delete soft-deletes the project, its tasks, and its goals in one
	// transaction (docs/06-planner.md#deleting). Its sub-projects move to the
	// top level in the same transaction.
	Delete(ctx context.Context, id string) (Changes, error)
	// SetFolder sets the project's folder on this machine, or forgets it for
	// "". The folder is device-local: it changes neither rev nor sync.
	SetFolder(ctx context.Context, id, path string) (model.Project, error)
	// SetAutonomy sets how freely Gwen's agent acts in the project on this
	// machine: cautious, balanced, trusted, or "" to follow the
	// configuration (plan.md 3.16). Device-local, like the folder.
	SetAutonomy(ctx context.Context, id, autonomy string) (model.Project, error)
	// Dir is the project's folder on this machine and its test command,
	// both "" when it has none.
	Dir(ctx context.Context, id string) (path, testCommand string, err error)
	// SetDir sets both; an empty path forgets them.
	SetDir(ctx context.Context, id, path, testCommand string) error
}

type projectRepo struct{ db *DB }

// NewProjectRepo returns the SQLite ProjectRepo.
func NewProjectRepo(db *DB) ProjectRepo { return projectRepo{db} }

// projectCols reads a parent only while it is live and at the top level
// itself, so a sub-project synced before its parent, or left under one that
// another device deleted or moved, reads as top level.
const projectCols = `id, name, color, archived_at, ` + envelopeCols +
	`, COALESCE((SELECT path FROM project_dirs WHERE project_dirs.project_id = projects.id), ''),
	COALESCE((SELECT agent_autonomy FROM project_dirs WHERE project_dirs.project_id = projects.id), ''),
	(SELECT up.id FROM projects up WHERE up.id = projects.parent_id AND up.deleted_at IS NULL AND up.parent_id IS NULL)`

func scanProject(s scanner) (model.Project, error) {
	var (
		p        model.Project
		archived sql.NullInt64
		parent   sql.NullString
		env      envelopeScan
	)
	if err := s.Scan(append(append([]any{&p.ID, &p.Name, &p.Color, &archived}, env.dest()...), &p.Folder, &p.AgentAutonomy, &parent)...); err != nil {
		return model.Project{}, err
	}
	p.ArchivedAt = timePtr(archived)
	p.ParentID = stringPtr(parent)
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

func (r projectRepo) Create(ctx context.Context, name string, color, parentID *string) (model.Project, error) {
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
		if parentID != nil {
			if err := validParent(ctx, tx, "", *parentID); err != nil {
				return err
			}
		}
		now := r.db.Now()
		p = model.Project{ID: model.NewID(), Name: name, Color: c, ParentID: parentID, Envelope: model.Envelope{
			CreatedAt: now, UpdatedAt: now, DeviceID: r.db.DeviceID(), Rev: 1,
		}}
		const qInsertProject = `INSERT INTO projects (id, name, color, archived_at, parent_id, created_at, updated_at,
			device_id, rev) VALUES (?, ?, ?, NULL, ?, ?, ?, ?, 1)`
		_, err = tx.ExecContext(ctx, qInsertProject, p.ID, p.Name, p.Color, nullString(p.ParentID), now.UnixMilli(),
			now.UnixMilli(), p.DeviceID)
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

// validParent checks that the project id ("" for a new one) can sit under
// parentID: a live project other than itself that is not a sub-project, while
// id has no sub-projects of its own. Projects stay one level deep.
func validParent(ctx context.Context, q Querier, id, parentID string) error {
	if id != "" && parentID == id {
		return FailField(ErrInvalid, "parent_id", "a project cannot be its own parent")
	}
	parent, err := getProject(ctx, q, parentID)
	if errors.Is(err, ErrNotFound) {
		return FailField(ErrNotFound, "parent_id", "no project %s", parentID)
	}
	if err != nil {
		return err
	}
	if parent.ParentID != nil {
		return FailField(ErrInvalid, "parent_id", "a sub-project cannot have sub-projects of its own")
	}
	if id == "" {
		return nil
	}
	const qHasChildren = `SELECT count(*) FROM projects WHERE parent_id = ? AND deleted_at IS NULL`
	var n int
	if err := q.QueryRowContext(ctx, qHasChildren, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return FailField(ErrInvalid, "parent_id", "a project with sub-projects cannot become a sub-project")
	}
	return nil
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
		if patch.ParentID.Set {
			if v := patch.ParentID.Value; v != nil {
				if err := validParent(ctx, tx, id, *v); err != nil {
					return err
				}
			}
			p.ParentID = patch.ParentID.Value
		}
		p.UpdatedAt, p.DeviceID, p.Rev = now, r.db.DeviceID(), p.Rev+1
		const qUpdateProject = `UPDATE projects SET name = ?, color = ?, archived_at = ?, parent_id = ?,
			updated_at = ?, device_id = ?, rev = ? WHERE id = ?`
		_, err = tx.ExecContext(ctx, qUpdateProject, p.Name, p.Color, nullMillis(p.ArchivedAt), nullString(p.ParentID),
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
		const qMoveUp = `UPDATE projects SET parent_id = NULL, updated_at = ?, device_id = ?, rev = rev + 1
			WHERE parent_id = ? AND deleted_at IS NULL`
		now := r.db.Now().UnixMilli()
		if _, err := tx.ExecContext(ctx, qMoveUp, now, r.db.DeviceID(), id); err != nil {
			return err
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

func (r projectRepo) Dir(ctx context.Context, id string) (string, string, error) {
	if _, err := getProject(ctx, r.db.sql, id); err != nil {
		return "", "", fmt.Errorf("project dir: %w", err)
	}
	var path, test string
	err := r.db.sql.QueryRowContext(ctx, `SELECT path, test_command FROM project_dirs WHERE project_id = ?`, id).Scan(&path, &test)
	if err != nil && err != sql.ErrNoRows {
		return "", "", fmt.Errorf("project dir: %w", err)
	}
	return path, test, nil
}

func (r projectRepo) SetDir(ctx context.Context, id, path, testCommand string) error {
	if len(testCommand) > 500 {
		return FailField(ErrInvalid, "test_command", "a test command is at most 500 characters")
	}
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getProject(ctx, tx, id); err != nil {
			return err
		}
		if path == "" {
			return classify(forgetDir(ctx, tx, id, r.db.Now().UnixMilli()))
		}
		const q = `INSERT INTO project_dirs (project_id, path, test_command, updated_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (project_id) DO UPDATE SET path = excluded.path, test_command = excluded.test_command,
			updated_at = excluded.updated_at`
		_, err := tx.ExecContext(ctx, q, id, path, testCommand, r.db.Now().UnixMilli())
		return classify(err)
	})
	if err != nil {
		return fmt.Errorf("set dir of project %s: %w", id, err)
	}
	return nil
}

func (r projectRepo) SetFolder(ctx context.Context, id, path string) (model.Project, error) {
	var p model.Project
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getProject(ctx, tx, id); err != nil {
			return err
		}
		if path == "" {
			if err := forgetDir(ctx, tx, id, r.db.Now().UnixMilli()); err != nil {
				return classify(err)
			}
		} else {
			const qSetFolder = `INSERT INTO project_dirs (project_id, path, updated_at) VALUES (?, ?, ?)
				ON CONFLICT (project_id) DO UPDATE SET path = excluded.path, updated_at = excluded.updated_at`
			if _, err := tx.ExecContext(ctx, qSetFolder, id, path, r.db.Now().UnixMilli()); err != nil {
				return classify(err)
			}
		}
		var err error
		p, err = getProject(ctx, tx, id)
		return err
	})
	if err != nil {
		return model.Project{}, fmt.Errorf("set folder of project %s: %w", id, err)
	}
	return p, nil
}

// forgetDir forgets a project's folder on this machine. The row stays while
// it still says how freely Gwen's agent acts there.
func forgetDir(ctx context.Context, tx *sql.Tx, id string, now int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_dirs WHERE project_id = ? AND agent_autonomy = ''`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE project_dirs SET path = '', test_command = '', updated_at = ? WHERE project_id = ?`, now, id)
	return err
}

func (r projectRepo) SetAutonomy(ctx context.Context, id, autonomy string) (model.Project, error) {
	switch autonomy {
	case "", "cautious", "balanced", "trusted":
	default:
		return model.Project{}, FailField(ErrInvalid, "agent_autonomy", "autonomy is cautious, balanced, or trusted")
	}
	var p model.Project
	err := r.db.InTx(ctx, func(tx *sql.Tx) error {
		if _, err := getProject(ctx, tx, id); err != nil {
			return err
		}
		const q = `INSERT INTO project_dirs (project_id, path, agent_autonomy, updated_at) VALUES (?, '', ?, ?)
			ON CONFLICT (project_id) DO UPDATE SET agent_autonomy = excluded.agent_autonomy, updated_at = excluded.updated_at`
		if _, err := tx.ExecContext(ctx, q, id, autonomy, r.db.Now().UnixMilli()); err != nil {
			return classify(err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_dirs WHERE project_id = ? AND path = '' AND agent_autonomy = ''`, id); err != nil {
			return classify(err)
		}
		var err error
		p, err = getProject(ctx, tx, id)
		return err
	})
	if err != nil {
		return model.Project{}, fmt.Errorf("set the agent's autonomy in project %s: %w", id, err)
	}
	return p, nil
}
