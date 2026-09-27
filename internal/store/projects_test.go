package store_test

import (
	"testing"
	"time"

	"github.com/kzark/gwen/internal/model"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestProjectCreateAndPalette(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	var colors []string
	for _, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"} {
		colors = append(colors, f.project(name).Color)
	}
	require.Equal(t, append(append([]string{}, store.Palette...), store.Palette[0], store.Palette[1]), colors,
		"the first unused colour, cycling from the start when all are used")

	p, err := f.r.Projects.Create(f.ctx, "  Custom  ", testutil.Ptr("#ABCDEF"))
	require.NoError(t, err)
	require.Equal(t, "Custom", p.Name)
	require.Equal(t, "#abcdef", p.Color)
	require.Equal(t, int64(1), p.Rev)
	require.Equal(t, fxNow, p.CreatedAt)
	require.Equal(t, f.db.DeviceID(), p.DeviceID)

	got, err := f.r.Projects.Get(f.ctx, p.ID)
	require.NoError(t, err)
	require.Equal(t, p, got)
}

func TestProjectDeletedColourIsFreeAgain(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	a := f.project("a")
	f.project("b")
	require.NoError(t, f.r.Projects.Delete(f.ctx, a.ID))
	require.Equal(t, store.Palette[0], f.project("c").Color)
}

func TestProjectValidation(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	f.project("Internship")
	tests := []struct {
		name     string
		pname    string
		color    *string
		sentinel error
		field    string
	}{
		{"empty name", "   ", nil, store.ErrInvalid, "name"},
		{"long name", string(make([]rune, 81)), nil, store.ErrInvalid, "name"},
		{"bad colour", "x", testutil.Ptr("blue"), store.ErrInvalid, "color"},
		{"duplicate name ignoring case", "INTERNSHIP", nil, store.ErrConflict, "name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.r.Projects.Create(f.ctx, tc.pname, tc.color)
			userErr(t, err, tc.sentinel, tc.field)
		})
	}
}

func TestProjectUpdateArchiveAndRev(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("Old")
	other := f.project("Other")

	f.clk.Advance(time.Minute)
	up, err := f.r.Projects.Update(f.ctx, p.ID, store.ProjectPatch{Name: testutil.Ptr("New"), Archived: testutil.Ptr(true), Rev: &p.Rev})
	require.NoError(t, err)
	require.Equal(t, "New", up.Name)
	require.Equal(t, int64(2), up.Rev)
	require.Equal(t, fxNow.Add(time.Minute), up.UpdatedAt)
	require.Equal(t, fxNow.Add(time.Minute), *up.ArchivedAt)
	require.Equal(t, p.Color, up.Color)

	_, err = f.r.Projects.Update(f.ctx, p.ID, store.ProjectPatch{Color: testutil.Ptr("#000000"), Rev: &p.Rev})
	userErr(t, err, store.ErrConflict, "rev")
	_, err = f.r.Projects.Update(f.ctx, p.ID, store.ProjectPatch{Name: testutil.Ptr("other")})
	userErr(t, err, store.ErrConflict, "name")
	_, err = f.r.Projects.Update(f.ctx, p.ID, store.ProjectPatch{Color: testutil.Ptr("#12345")})
	userErr(t, err, store.ErrInvalid, "color")
	_, err = f.r.Projects.Update(f.ctx, p.ID, store.ProjectPatch{Name: testutil.Ptr("")})
	userErr(t, err, store.ErrInvalid, "name")

	live, err := f.r.Projects.List(f.ctx, store.ArchivedExclude)
	require.NoError(t, err)
	require.Equal(t, []string{other.ID}, projectIDs(live))
	archived, err := f.r.Projects.List(f.ctx, store.ArchivedOnly)
	require.NoError(t, err)
	require.Equal(t, []string{p.ID}, projectIDs(archived))

	un, err := f.r.Projects.Update(f.ctx, p.ID, store.ProjectPatch{Archived: testutil.Ptr(false), Color: testutil.Ptr("#000000")})
	require.NoError(t, err)
	require.Nil(t, un.ArchivedAt)
	require.Equal(t, "#000000", un.Color)
	require.Equal(t, int64(3), un.Rev)

	_, err = f.r.Projects.List(f.ctx, "maybe")
	userErr(t, err, store.ErrInvalid, "archived")
}

func TestProjectListOrderIgnoresCase(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	for _, n := range []string{"beta", "Alpha", "gamma", "Delta"} {
		f.project(n)
	}
	all, err := f.r.Projects.List(f.ctx, store.ArchivedAll)
	require.NoError(t, err)
	var names []string
	for _, p := range all {
		names = append(names, p.Name)
	}
	require.Equal(t, []string{"Alpha", "beta", "Delta", "gamma"}, names)
}

func TestProjectDeleteSoftDeletesItsTasks(t *testing.T) {
	t.Parallel()
	f := newFx(t)
	p := f.project("P")
	keep := f.project("Keep")
	t1 := f.task("one", &p.ID)
	t2 := f.task("two", &keep.ID)
	f.clk.Advance(time.Hour)

	require.NoError(t, f.r.Projects.Delete(f.ctx, p.ID))
	_, err := f.r.Projects.Get(f.ctx, p.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.r.Tasks.Get(f.ctx, t1.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	_, err = f.r.Tasks.Get(f.ctx, t2.ID)
	require.NoError(t, err)

	var deletedAt, updatedAt, rev int64
	var device string
	require.NoError(t, f.db.SQL().QueryRow(`SELECT deleted_at, updated_at, rev, device_id FROM tasks WHERE id = ?`, t1.ID).
		Scan(&deletedAt, &updatedAt, &rev, &device))
	require.Equal(t, fxNow.Add(time.Hour).UnixMilli(), deletedAt)
	require.Equal(t, deletedAt, updatedAt, "a soft delete is a write")
	require.Equal(t, int64(2), rev)
	require.Equal(t, f.db.DeviceID(), device)

	require.ErrorIs(t, f.r.Projects.Delete(f.ctx, p.ID), store.ErrNotFound, "deleted rows are gone")
	f.project("P") // the name is free again
}

func projectIDs(ps []model.Project) []string {
	ids := []string{}
	for _, p := range ps {
		ids = append(ids, p.ID)
	}
	return ids
}
