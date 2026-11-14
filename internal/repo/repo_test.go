package repo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMain keeps the user's git configuration (signing, hooks, default
// branch) out of the tests.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "repo-test-home")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	os.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	os.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// fixture is a test repository.
type fixture struct {
	t    *testing.T
	dir  string
	tick int // commits so far; each is a minute after the last
}

// day0 is the first commit's time, 2026-01-01 00:00 UTC.
const day0 = 1767225600

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func newRepo(t *testing.T) *fixture {
	t.Helper()
	needGit(t)
	r := &fixture{t: t, dir: t.TempDir()}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "Ada Lovelace")
	r.git("config", "user.email", "ada@example.com")
	r.git("config", "commit.gpgsign", "false")
	r.git("config", "core.autocrlf", "false")
	return r
}

func (r *fixture) git(args ...string) string {
	r.t.Helper()
	return r.gitEnv(nil, args...)
}

func (r *fixture) gitEnv(env []string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(r.t, err, "git %v: %s", args, stderr.String())
	return string(out)
}

func (r *fixture) write(path, content string) {
	r.t.Helper()
	p := filepath.Join(r.dir, filepath.FromSlash(path))
	require.NoError(r.t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(r.t, os.WriteFile(p, []byte(content), 0o644))
}

// commit commits everything staged, a minute after the last commit.
func (r *fixture) commit(msg string, author ...string) string {
	r.t.Helper()
	date := fmt.Sprintf("@%d +0000", day0+60*r.tick)
	r.tick++
	env := []string{"GIT_AUTHOR_DATE=" + date, "GIT_COMMITTER_DATE=" + date}
	if len(author) == 2 {
		env = append(env, "GIT_AUTHOR_NAME="+author[0], "GIT_AUTHOR_EMAIL="+author[1])
	}
	r.gitEnv(env, "commit", "-q", "--allow-empty", "-m", msg)
	return strings.TrimSpace(r.git("rev-parse", "HEAD"))
}

func (r *fixture) commitAll(msg string) string {
	r.t.Helper()
	r.git("add", "-A")
	return r.commit(msg)
}

func ctx() context.Context { return context.Background() }

func lines(n int, word string) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "%s %d\n", word, i)
	}
	return sb.String()
}

func TestResolveKeepsPathsInside(t *testing.T) {
	t.Parallel()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("x"), 0o644))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(dir, "leak")))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "sub", "out")))
	require.NoError(t, os.Symlink("a.txt", filepath.Join(dir, "alias")))
	f, err := open(dir)
	require.NoError(t, err)

	for _, p := range []string{"a.txt", "sub/../a.txt", filepath.Join(dir, "a.txt"), "alias", "gone/never.txt", "sub"} {
		_, _, err := f.resolve(p)
		require.NoError(t, err, p)
	}
	rel, real, err := f.resolve("alias")
	require.NoError(t, err)
	require.Equal(t, "alias", rel, "git names the link, not its target")
	require.Equal(t, filepath.Join(f.root, "a.txt"), real)

	for _, p := range []string{"../x", "sub/../../x", "leak", "sub/out/secret", "sub/out", filepath.Join(outside, "secret"), "/etc/passwd"} {
		_, _, err := f.resolve(p)
		require.ErrorIs(t, err, ErrOutside, p)
	}
	_, _, err = f.resolve("")
	require.ErrorIs(t, err, ErrInvalid)

	_, err = open("relative/dir")
	require.ErrorIs(t, err, ErrInvalid)

	got, err := Resolve(dir, "a.txt")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(f.root, "a.txt"), got)
	_, err = Resolve(dir, "missing.txt")
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = Resolve(dir, "leak")
	require.ErrorIs(t, err, ErrOutside)
}

func TestResolveThroughALinkedFolder(t *testing.T) {
	t.Parallel()
	real := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(real, "a.txt"), []byte("a\n"), 0o644))
	link := filepath.Join(t.TempDir(), "proj")
	require.NoError(t, os.Symlink(real, link))
	file, err := ReadFile(ctx(), link, "a.txt")
	require.NoError(t, err)
	require.Equal(t, "a\n", file.Text)
	file, err = ReadFile(ctx(), link, filepath.Join(link, "a.txt"))
	require.NoError(t, err)
	require.Equal(t, "a.txt", file.Path)
}
