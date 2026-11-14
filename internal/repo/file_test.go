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
	require.Equal(t, File{Path: "cmd/main.go", Text: "package main\n\nfunc main() {}\n", Size: 29, Lines: 3, Lang: "go"}, f)

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
