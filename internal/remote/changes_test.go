package remote

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
	"github.com/stretchr/testify/require"
)

// Changes runs the real git on temporary repositories (plan.md 3.14.9).
// Git reads no config but the repository's own, so the tests can't see the
// person's ~/.gitconfig or global ignores; that takes t.Setenv, so these
// tests don't run in parallel.

// w27cGitEnv isolates git from the machine's config and from any repository
// above the temporary folders.
func w27cGitEnv(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
}

// w27cRepo is a fresh repository on branch main, with no commits.
func w27cRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	w27cGit(t, dir, "init", "-q", "-b", "main")
	w27cGit(t, dir, "config", "user.name", "Gwen Test")
	w27cGit(t, dir, "config", "user.email", "gwen@example.invalid")
	w27cGit(t, dir, "config", "commit.gpgsign", "false")
	w27cGit(t, dir, "config", "core.excludesFile", os.DevNull)
	return dir
}

func w27cGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
}

func w27cWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

// w27cCommitted is a repository with one commit of a few files.
func w27cCommitted(t *testing.T) string {
	t.Helper()
	dir := w27cRepo(t)
	w27cWrite(t, dir, "main.go", "package main\n\nfunc main() {\n}\n")
	w27cWrite(t, dir, "README.md", "# App\n\nOld words.\n")
	w27cWrite(t, dir, "old-name.txt", "one\ntwo\nthree\nfour\nfive\n")
	w27cWrite(t, dir, "delete-me.txt", "bye\n")
	w27cWrite(t, dir, "logo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x01")
	w27cWrite(t, dir, ".gitignore", "*.log\n")
	w27cGit(t, dir, "add", "-A")
	w27cGit(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func TestChangesInARepository(t *testing.T) {
	w27cGitEnv(t)
	dir := w27cCommitted(t)
	w27cWrite(t, dir, "main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(1)\n}\n")
	require.NoError(t, os.Remove(filepath.Join(dir, "delete-me.txt")))
	w27cGit(t, dir, "mv", "old-name.txt", "new-name.txt")
	w27cWrite(t, dir, "logo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x02\x00")
	w27cWrite(t, dir, "notes.txt", "a\nb\nc\n")
	w27cWrite(t, dir, "pkg/deep/new.go", "package deep\n")
	w27cWrite(t, dir, "blob.bin", "\x00\x01\x02 binary")
	w27cWrite(t, dir, "build.log", "ignored\n")
	w27cWrite(t, dir, "spaced name.txt", "x\n")

	got := changes(t.Context(), store.CodeSession{Cwd: dir})
	require.True(t, got.Git)
	require.Equal(t, "main", got.Branch)
	byPath := map[string]wire.CodeChange{}
	for _, c := range got.Files {
		byPath[c.Path] = c
	}
	require.Equal(t, map[string]wire.CodeChange{
		"main.go":         {Path: "main.go", Status: "modified", Added: 3, Removed: 0},
		"delete-me.txt":   {Path: "delete-me.txt", Status: "deleted", Removed: 1},
		"new-name.txt":    {Path: "new-name.txt", OldPath: "old-name.txt", Status: "renamed"},
		"logo.png":        {Path: "logo.png", Status: "modified", Binary: true},
		"notes.txt":       {Path: "notes.txt", Status: "untracked", Added: 3},
		"pkg/deep/new.go": {Path: "pkg/deep/new.go", Status: "untracked", Added: 1},
		"blob.bin":        {Path: "blob.bin", Status: "untracked", Binary: true},
		"spaced name.txt": {Path: "spaced name.txt", Status: "untracked", Added: 1},
	}, byPath, "an ignored file is no change")
	require.Len(t, got.Files, len(byPath), "each file once")
}

// Before the first commit everything is compared with git's empty tree.
func TestChangesBeforeTheFirstCommit(t *testing.T) {
	w27cGitEnv(t)
	dir := w27cRepo(t)
	w27cWrite(t, dir, "staged.go", "package x\n\nvar A = 1\n")
	w27cGit(t, dir, "add", "staged.go")
	w27cWrite(t, dir, "loose.txt", "one\ntwo\n")

	got := changes(t.Context(), store.CodeSession{Cwd: dir})
	require.Equal(t, wire.CodeChanges{Git: true, Branch: "main", Files: []wire.CodeChange{
		{Path: "staged.go", Status: "added", Added: 3},
		{Path: "loose.txt", Status: "untracked", Added: 2},
	}}, got)

	d, ok := diff(t.Context(), store.CodeSession{Cwd: dir}, "staged.go")
	require.True(t, ok)
	require.Contains(t, d.Diff, "+var A = 1\n")
}

// What was already changed when the session started is marked, so its own
// work stands out (plan.md 3.14.9).
func TestChangesMarksWhatWasChangedBeforeTheSession(t *testing.T) {
	w27cGitEnv(t)
	dir := w27cCommitted(t)
	w27cWrite(t, dir, "README.md", "# App\n\nNew words.\n")
	w27cWrite(t, dir, "scratch.txt", "mine\n")
	w27cGit(t, dir, "mv", "old-name.txt", "renamed-before.txt")
	start := startStatus(t.Context(), dir)
	require.Equal(t, map[string]bool{"README.md": true, "scratch.txt": true, "renamed-before.txt": true}, start,
		"a rename counts by its new name")

	// Then the session works.
	w27cWrite(t, dir, "README.md", "# App\n\nNewer words.\n")
	w27cWrite(t, dir, "main.go", "package main\n\nfunc main() {\n\tprintln()\n}\n")
	w27cWrite(t, dir, "added.txt", "x\n")

	got := changes(t.Context(), store.CodeSession{Cwd: dir, StartStatus: start})
	before := map[string]bool{}
	for _, c := range got.Files {
		before[c.Path] = c.BeforeSession
	}
	require.Equal(t, map[string]bool{"README.md": true, "scratch.txt": true, "renamed-before.txt": true, "main.go": false, "added.txt": false}, before)
}

// A session in a subfolder sees the whole repository's changes, with paths
// from its top, the same paths git status gave at the start.
func TestChangesFromASubfolder(t *testing.T) {
	w27cGitEnv(t)
	dir := w27cCommitted(t)
	sub := filepath.Join(dir, "pkg")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	w27cWrite(t, dir, "pkg/a.go", "package pkg\n")
	start := startStatus(t.Context(), sub)
	w27cWrite(t, dir, "README.md", "# App\n")

	got := changes(t.Context(), store.CodeSession{Cwd: sub, StartStatus: start})
	require.Equal(t, []wire.CodeChange{
		{Path: "README.md", Status: "modified", Added: 0, Removed: 2},
		{Path: "pkg/a.go", Status: "untracked", Added: 1, BeforeSession: true},
	}, got.Files)
}

// Outside git, Changes lists the files the session edited, without diffs.
func TestChangesOutsideGit(t *testing.T) {
	w27cGitEnv(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	kept := filepath.Join(dir, "kept.txt")
	require.NoError(t, os.WriteFile(kept, []byte("x"), 0o644))
	gone := filepath.Join(dir, "gone.txt")

	got := changes(t.Context(), store.CodeSession{Cwd: dir, Touched: []string{kept, gone}})
	require.Equal(t, wire.CodeChanges{Files: []wire.CodeChange{
		{Path: kept, Status: "modified"},
		{Path: gone, Status: "deleted"},
	}}, got)
	_, ok := diff(t.Context(), store.CodeSession{Cwd: dir}, "kept.txt")
	require.False(t, ok, "no diffs outside git")
	require.Nil(t, startStatus(t.Context(), dir))
}

// A folder that isn't git and with nothing edited has an empty list, never
// a null one, which the phone would trip on.
func TestChangesOutsideGitWithNothingEdited(t *testing.T) {
	w27cGitEnv(t)
	got := changes(t.Context(), store.CodeSession{Cwd: t.TempDir()})
	require.NotNil(t, got.Files)
	require.Empty(t, got.Files)
	require.False(t, got.Git)
}

func TestDiffOfOneChangedFile(t *testing.T) {
	w27cGitEnv(t)
	dir := w27cCommitted(t)
	sess := store.CodeSession{Cwd: dir}
	w27cWrite(t, dir, "README.md", "# App\n\nNew words.\n")
	w27cWrite(t, dir, "notes.txt", "a\nb\n")
	w27cWrite(t, dir, "logo.png", "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x02\x00")
	w27cWrite(t, dir, "blob.bin", "\x00\x01\x02")
	var long bytes.Buffer
	for range 2500 {
		long.WriteString("line\n")
	}
	w27cWrite(t, dir, "long.txt", long.String())

	d, ok := diff(t.Context(), sess, "README.md")
	require.True(t, ok)
	require.Contains(t, d.Diff, "-Old words.\n+New words.\n")
	require.False(t, d.Binary)

	// An untracked file is all new.
	d, ok = diff(t.Context(), sess, "notes.txt")
	require.True(t, ok)
	require.Contains(t, d.Diff, "+a\n+b\n")

	// A binary file returns its size only.
	d, ok = diff(t.Context(), sess, "logo.png")
	require.True(t, ok)
	require.Equal(t, wire.CodeDiff{Path: "logo.png", Binary: true, Size: 19}, d)
	d, ok = diff(t.Context(), sess, "blob.bin")
	require.True(t, ok)
	require.Equal(t, wire.CodeDiff{Path: "blob.bin", Binary: true, Size: 3}, d)

	// A long diff is cut at 2,000 lines.
	d, ok = diff(t.Context(), sess, "long.txt")
	require.True(t, ok)
	require.True(t, d.Truncated)
	require.Equal(t, maxDiffLines, strings.Count(d.Diff, "\n"))

	// Nothing to show, and nothing outside the repository.
	for _, path := range []string{"missing.txt", "", "../outside.txt", "/etc/passwd", "pkg/../../x"} {
		_, ok := diff(t.Context(), sess, path)
		require.False(t, ok, path)
	}
}

// Through the service: a session's folder is the one its card names.
func TestChangesOfASession(t *testing.T) {
	w27cGitEnv(t)
	dir := w27cCommitted(t)
	w27cWrite(t, dir, "notes.txt", "a\n")
	svc, _ := w27cService(t)
	svc.mu.Lock()
	svc.sessions["cs_changes001"] = &session{CodeSession: store.CodeSession{ID: "cs_changes001", Cwd: dir, State: store.CodeIdle}}
	svc.mu.Unlock()

	got, err := svc.Changes(t.Context(), "cs_changes001")
	require.NoError(t, err)
	require.Equal(t, []wire.CodeChange{{Path: "notes.txt", Status: "untracked", Added: 1}}, got.Files)
	d, err := svc.Diff(t.Context(), "cs_changes001", "notes.txt")
	require.NoError(t, err)
	require.Contains(t, d.Diff, "+a\n")

	_, err = svc.Diff(t.Context(), "cs_changes001", "../elsewhere.txt")
	var e *Error
	require.ErrorAs(t, err, &e)
	require.Equal(t, wire.CodeNotFound, e.Code)
	_, err = svc.Changes(t.Context(), "cs_nosuch0000")
	require.ErrorAs(t, err, &e)
	require.Equal(t, wire.CodeNotFound, e.Code)
}
