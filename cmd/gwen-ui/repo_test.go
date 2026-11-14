package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/kzark/gwen/internal/repo"
	"github.com/stretchr/testify/require"
)

func TestRepoBindingsGiveTheUIErrorShape(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644))
	var started []string
	a := &App{ctx: context.Background(), host: &host{start: func(name string, args ...string) error {
		started = append([]string{name}, args...)
		return nil
	}}}

	code := func(err error) string {
		t.Helper()
		require.Error(t, err)
		var he hostError
		require.NoError(t, json.Unmarshal([]byte(err.Error()), &he))
		return he.Code
	}
	_, err := a.RepoLog(dir, repo.LogQuery{})
	require.Equal(t, codeNotRepo, code(err))
	_, err = a.RepoFile(dir, "../x")
	require.Equal(t, codeOutside, code(err))
	_, err = a.RepoCommitFiles(dir, "", []string{"a.txt"})
	require.Equal(t, "invalid_request", code(err))
	_, err = a.RepoFile(dir, "missing")
	require.Equal(t, "internal", code(err))

	f, err := a.RepoFile(dir, "a.txt")
	require.NoError(t, err)
	require.Equal(t, "a\n", f.Text)

	require.NoError(t, a.OpenPath(dir, "a.txt"))
	real, _ := filepath.EvalSymlinks(filepath.Join(dir, "a.txt"))
	require.Equal(t, []string{"xdg-open", real}, started)
	require.Equal(t, codeOutside, code(a.OpenPath(dir, "/etc/passwd")))
}
