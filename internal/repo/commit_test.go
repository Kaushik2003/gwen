package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCommitFilesCommitsExactlyTheListedFiles(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("edit.txt", "one\n")
	r.write("delete.txt", "bye\n")
	r.write("rm.txt", "rm\n")
	r.write("old.txt", "rename me\n")
	r.write("other.txt", "other\n")
	r.write("staged.txt", "staged\n")
	r.commitAll("first")

	r.write("edit.txt", "one\ntwo\n")
	require.NoError(t, os.Remove(filepath.Join(r.dir, "delete.txt")))
	r.git("rm", "-q", "rm.txt")
	r.git("mv", "old.txt", "new.txt")
	r.write("brand new.txt", "new\n")
	r.write("other.txt", "other, not committed\n")
	r.write("staged.txt", "staged, not committed\n")
	r.git("add", "staged.txt")
	r.write("left.txt", "untracked, not committed\n")

	short, err := CommitFiles(ctx(), r.dir, "-m looks like a flag but is the message", []string{
		"edit.txt", "delete.txt", "rm.txt", "old.txt", "new.txt", "brand new.txt", "./edit.txt",
	})
	require.NoError(t, err)
	head := strings.TrimSpace(r.git("rev-parse", "HEAD"))
	require.True(t, strings.HasPrefix(head, short))
	require.Equal(t, "-m looks like a flag but is the message", strings.TrimSpace(r.git("log", "-1", "--format=%B")))

	d, err := Show(ctx(), r.dir, head)
	require.NoError(t, err)
	require.Equal(t, []FileChange{
		{Path: "brand new.txt", Status: "A", Added: 1},
		{Path: "delete.txt", Status: "D", Removed: 1},
		{Path: "edit.txt", Status: "M", Added: 1},
		{Path: "new.txt", OldPath: "old.txt", Status: "R"},
		{Path: "rm.txt", Status: "D", Removed: 1},
	}, d.Changes)

	// Everything else is as it was: changed, staged, untracked.
	tree, err := ListFiles(ctx(), r.dir)
	require.NoError(t, err)
	changed := map[string]TreeFile{}
	for _, f := range tree.Files {
		if f.Status != "" {
			changed[f.Path] = f
		}
	}
	require.Equal(t, map[string]TreeFile{
		"other.txt":  {Path: "other.txt", Status: "M"},
		"staged.txt": {Path: "staged.txt", Status: "M", Staged: true},
		"left.txt":   {Path: "left.txt", Status: "U"},
	}, changed)
}

func TestCommitFilesRefuses(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.commitAll("first")
	r.write("a.txt", "b\n")
	outside := filepath.Join(t.TempDir(), "x.txt")
	require.NoError(t, os.WriteFile(outside, []byte("x"), 0o644))
	require.NoError(t, os.Symlink(filepath.Dir(outside), filepath.Join(r.dir, "out")))

	_, err := CommitFiles(ctx(), r.dir, "  \n", []string{"a.txt"})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = CommitFiles(ctx(), r.dir, "msg", nil)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = CommitFiles(ctx(), r.dir, "msg", []string{"a.txt", "../escape.txt"})
	require.ErrorIs(t, err, ErrOutside)
	_, err = CommitFiles(ctx(), r.dir, "msg", []string{"out/x.txt"})
	require.ErrorIs(t, err, ErrOutside)
	_, err = CommitFiles(ctx(), r.dir, "msg", []string{"."})
	require.ErrorIs(t, err, ErrInvalid)
	_, err = CommitFiles(ctx(), r.dir, "msg", []string{"never-existed.txt"})
	require.Error(t, err)
	require.Equal(t, "1\n", r.git("rev-list", "--count", "HEAD"), "nothing committed")

	needGit(t)
	_, err = CommitFiles(ctx(), t.TempDir(), "msg", []string{"a.txt"})
	require.ErrorIs(t, err, ErrNotRepo)
}

func TestCommitFilesLeavesNewFilesUntrackedWhenTheCommitFails(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.commitAll("first")
	r.write("new.txt", "new\n")
	hook := filepath.Join(r.dir, ".git", "hooks", "pre-commit")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\necho no >&2\nexit 1\n"), 0o755))

	_, err := CommitFiles(ctx(), r.dir, "msg", []string{"new.txt"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "no")
	require.Equal(t, "?? new.txt\n", r.git("status", "--porcelain"))
}

func TestCommitFilesFirstCommit(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("a.txt", "a\n")
	r.write("b.txt", "b\n")
	short, err := CommitFiles(ctx(), r.dir, "first", []string{"a.txt"})
	require.NoError(t, err)
	require.NotEmpty(t, short)
	require.Equal(t, "a.txt\n", r.git("ls-tree", "--name-only", "HEAD"))
	require.Equal(t, "?? b.txt\n", r.git("status", "--porcelain"))
}
