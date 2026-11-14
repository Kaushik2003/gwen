package repo

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func statuses(t Tree) map[string]TreeFile {
	m := map[string]TreeFile{}
	for _, f := range t.Files {
		m[f.Path] = f
	}
	return m
}

func TestListFilesInARepository(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write(".gitignore", "*.log\n")
	r.write("keep.txt", "keep\n")
	r.write("edit.txt", "one\n")
	r.write("gone.txt", "gone\n")
	r.write("removed.txt", "removed\n")
	r.write("old.txt", "rename me\n")
	r.write("src/deep/main.go", "package main\n")
	r.commitAll("first")

	r.write("edit.txt", "one\ntwo\n")
	require.NoError(t, os.Remove(filepath.Join(r.dir, "gone.txt")))
	r.git("rm", "-q", "removed.txt")
	r.git("mv", "old.txt", "new.txt")
	r.write("staged.txt", "staged\n")
	r.git("add", "staged.txt")
	r.write("src/untracked.go", "package src\n")
	r.write("debug.log", "ignored\n")
	r.write("ünïcode name.txt", "x\n")

	tree, err := ListFiles(ctx(), r.dir)
	require.NoError(t, err)
	require.True(t, tree.Git)
	require.Equal(t, "main", tree.Branch)
	require.Equal(t, r.dir, tree.Root)
	require.False(t, tree.Truncated)
	got := statuses(tree)
	require.NotContains(t, got, "debug.log", "ignored files are left out")
	require.NotContains(t, got, "old.txt", "a rename lists only its new name")
	want := map[string]TreeFile{
		".gitignore":       {Path: ".gitignore"},
		"keep.txt":         {Path: "keep.txt"},
		"edit.txt":         {Path: "edit.txt", Status: "M"},
		"gone.txt":         {Path: "gone.txt", Status: "D"},
		"removed.txt":      {Path: "removed.txt", Status: "D", Staged: true},
		"new.txt":          {Path: "new.txt", Status: "R", Staged: true, OldPath: "old.txt"},
		"staged.txt":       {Path: "staged.txt", Status: "A", Staged: true},
		"src/deep/main.go": {Path: "src/deep/main.go"},
		"src/untracked.go": {Path: "src/untracked.go", Status: "U"},
		"ünïcode name.txt": {Path: "ünïcode name.txt", Status: "U"},
	}
	require.Equal(t, want, got)
	for i := 1; i < len(tree.Files); i++ {
		require.Less(t, tree.Files[i-1].Path, tree.Files[i].Path, "sorted")
	}
}

func TestListFilesTracksUpstream(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.commitAll("first")
	r.git("checkout", "-q", "-b", "feature", "--track", "main")
	r.write("b.txt", "b\n")
	r.commitAll("ahead one")
	r.commitAll("ahead two")
	r.git("checkout", "-q", "main")
	r.commitAll("behind one")
	r.git("checkout", "-q", "feature")

	tree, err := ListFiles(ctx(), r.dir)
	require.NoError(t, err)
	require.Equal(t, "feature", tree.Branch)
	require.Equal(t, "main", tree.Upstream)
	require.Equal(t, 2, tree.Ahead)
	require.Equal(t, 1, tree.Behind)

	r.git("checkout", "-q", "--detach", "HEAD")
	tree, err = ListFiles(ctx(), r.dir)
	require.NoError(t, err)
	require.Equal(t, "", tree.Branch, "detached")
}

func TestListFilesInASubfolder(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("top.txt", "top\n")
	r.write("app/a.txt", "a\n")
	r.write("app/lib/b.txt", "b\n")
	r.commitAll("first")
	r.write("app/a.txt", "a\nmore\n")
	r.write("top.txt", "changed outside\n")
	r.write("app/new.txt", "new\n")

	tree, err := ListFiles(ctx(), filepath.Join(r.dir, "app"))
	require.NoError(t, err)
	require.Equal(t, map[string]TreeFile{
		"a.txt":     {Path: "a.txt", Status: "M"},
		"lib/b.txt": {Path: "lib/b.txt"},
		"new.txt":   {Path: "new.txt", Status: "U"},
	}, statuses(tree))
}

func TestListFilesWithoutGit(t *testing.T) {
	t.Parallel()
	needGit(t) // the test needs git to say "not a repository"
	dir := t.TempDir()
	for _, p := range []string{"a.txt", "src/b.go", ".env", "node_modules/x/index.js", ".git/config", "web/node_modules/y.js"} {
		p = filepath.Join(dir, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	tree, err := ListFiles(ctx(), dir)
	require.NoError(t, err)
	require.False(t, tree.Git)
	require.Equal(t, []TreeFile{{Path: ".env"}, {Path: "a.txt"}, {Path: "src/b.go"}}, tree.Files)

	tree, err = listFiles(ctx(), dir, 2)
	require.NoError(t, err)
	require.True(t, tree.Truncated)
	require.Len(t, tree.Files, 2)
}

func TestListFilesTruncatesKeepingChanges(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	for _, p := range []string{"a", "b", "c", "d", "e"} {
		r.write(p+".txt", p+"\n")
	}
	r.commitAll("first")
	r.write("e.txt", "changed\n")
	r.write("f.txt", "new\n")
	tree, err := listFiles(ctx(), r.dir, 3)
	require.NoError(t, err)
	require.True(t, tree.Truncated)
	require.Equal(t, []TreeFile{{Path: "a.txt"}, {Path: "e.txt", Status: "M"}, {Path: "f.txt", Status: "U"}}, tree.Files)
}

func TestListFilesRefusesABadFolder(t *testing.T) {
	t.Parallel()
	_, err := ListFiles(ctx(), filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = ListFiles(ctx(), "not/absolute")
	require.ErrorIs(t, err, ErrInvalid)
}
