package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(p string, b []byte) {
		p = filepath.Join(dir, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, b, 0o644))
	}
	write("cmd/main.go", []byte("package main\n\nfunc main() {}\n"))
	write("notes", []byte("no newline"))
	write("empty.txt", nil)
	write("img.png", []byte{0x89, 'P', 'N', 'G', 0, 0, 1})
	big := strings.Repeat("0123456789abcdef\n", MaxFileBytes/17+100)
	write("big.txt", []byte(big))
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "leak.txt")))

	f, err := ReadFile(ctx(), dir, "cmd/main.go")
	require.NoError(t, err)
	require.NotEmpty(t, f.Stamp)
	stamp, err := FileStamp(ctx(), dir, "cmd/main.go")
	require.NoError(t, err)
	require.Equal(t, f.Stamp, stamp)
	f.Stamp = ""
	require.Equal(t, File{Path: "cmd/main.go", Text: "package main\n\nfunc main() {}\n", Size: 29, Lines: 3, Lang: "go", Hash: hashOf([]byte("package main\n\nfunc main() {}\n"))}, f)

	f, err = ReadFile(ctx(), dir, "notes")
	require.NoError(t, err)
	require.Equal(t, 1, f.Lines)
	require.Equal(t, "", f.Lang)

	f, err = ReadFile(ctx(), dir, "empty.txt")
	require.NoError(t, err)
	require.Equal(t, 0, f.Lines)

	f, err = ReadFile(ctx(), dir, "img.png")
	require.NoError(t, err)
	require.True(t, f.Binary)
	require.Empty(t, f.Text)
	require.EqualValues(t, 7, f.Size)

	f, err = ReadFile(ctx(), dir, "big.txt")
	require.NoError(t, err)
	require.True(t, f.Truncated)
	require.Empty(t, f.Hash, "a cut file can't be edited")
	require.LessOrEqual(t, len(f.Text), MaxFileBytes)
	require.True(t, strings.HasSuffix(f.Text, "\n"), "cut at a line break")
	require.Equal(t, strings.Count(f.Text, "\n"), f.Lines)
	require.EqualValues(t, len(big), f.Size)

	_, err = ReadFile(ctx(), dir, "leak.txt")
	require.ErrorIs(t, err, ErrOutside)
	_, err = ReadFile(ctx(), dir, "../secret.txt")
	require.ErrorIs(t, err, ErrOutside)
	_, err = ReadFile(ctx(), dir, "cmd")
	require.ErrorIs(t, err, ErrNotFile)
	_, err = ReadFile(ctx(), dir, "missing.go")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestLang(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"a/b/view.go": "go", "App.TSX": "tsx", "x.ts": "ts", "Makefile": "make", "go.mod": "gomod", "README.md": "md",
		"style.css": "css", "run.sh": "sh", "Cargo.toml": "toml", "ci.yml": "yaml", "q.sql": "sql", "x.py": "py",
		"lib.rs": "rs", "page.html": "html", "data.json": "json", ".gitignore": "ignore", "LICENSE": "", "x.unknown": "",
	} {
		require.Equal(t, want, Lang(path), path)
	}
}

func TestSaveFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	p := filepath.Join(dir, "run.sh")
	require.NoError(t, os.WriteFile(p, []byte("echo a\r\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "latin1.txt"), []byte{'c', 'a', 'f', 0xe9}, 0o644))
	outside := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(outside, []byte("secret"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, "leak.txt")))

	f, err := ReadFile(ctx(), dir, "run.sh")
	require.NoError(t, err)
	require.NotEmpty(t, f.Hash)

	saved, err := SaveFile(ctx(), dir, "run.sh", "echo b\r\necho c\r\n", f.Hash)
	require.NoError(t, err)
	require.Equal(t, "echo b\r\necho c\r\n", saved.Text)
	require.Equal(t, 2, saved.Lines)
	require.NotEqual(t, f.Hash, saved.Hash)
	stamp, err := FileStamp(ctx(), dir, "run.sh")
	require.NoError(t, err)
	require.Equal(t, saved.Stamp, stamp)
	require.NotEqual(t, f.Stamp, stamp, "the stamp tells the save apart")
	_, err = FileStamp(ctx(), dir, "leak.txt")
	require.ErrorIs(t, err, ErrOutside)
	st, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o755), st.Mode().Perm(), "keeps its permissions")
	left, err := filepath.Glob(filepath.Join(dir, ".run.sh.gwen-*"))
	require.NoError(t, err)
	require.Empty(t, left, "no temporary file left behind")

	// Saving again from the old hash would undo the first save.
	_, err = SaveFile(ctx(), dir, "run.sh", "echo stale\n", f.Hash)
	require.ErrorIs(t, err, ErrChanged)
	b, _ := os.ReadFile(p)
	require.Equal(t, "echo b\r\necho c\r\n", string(b))

	l, err := ReadFile(ctx(), dir, "latin1.txt")
	require.NoError(t, err)
	require.Empty(t, l.Hash, "not UTF-8: editing would change bytes nobody touched")
	_, err = SaveFile(ctx(), dir, "latin1.txt", "cafe", l.Hash)
	require.ErrorIs(t, err, ErrInvalid)

	_, err = SaveFile(ctx(), dir, "leak.txt", "x", hashOf([]byte("secret")))
	require.ErrorIs(t, err, ErrOutside)
	_, err = SaveFile(ctx(), dir, "../secret.txt", "x", hashOf([]byte("secret")))
	require.ErrorIs(t, err, ErrOutside)
	_, err = SaveFile(ctx(), dir, "new.txt", "x", "abc")
	require.Error(t, err, "only existing files")
	require.NoFileExists(t, filepath.Join(dir, "new.txt"))
	b, _ = os.ReadFile(outside)
	require.Equal(t, "secret", string(b))
}
