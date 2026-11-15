package repo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// ErrChanged is a save whose file changed on disk since it was read.
var ErrChanged = errors.New("the file changed on disk since it was opened")

// MaxSaveBytes caps the text SaveFile writes.
const MaxSaveBytes = 16 << 20

// File is a file's text for the file editor.
type File struct {
	Path      string `json:"path"`
	Text      string `json:"text"`   // "" when Binary
	Binary    bool   `json:"binary"` // a NUL in the first 8000 bytes
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"` // over MaxFileBytes: Text is the first lines that fit
	Lines     int    `json:"lines"`     // lines in Text
	Lang      string `json:"lang"`      // see Lang
	// Hash is the SHA-256 of the file's bytes, set only when the file can be
	// edited: whole (not Truncated), not Binary, and valid UTF-8, so Text is
	// exactly what is on disk. SaveFile takes it back to refuse overwriting a
	// file that changed meanwhile.
	Hash string `json:"hash"`
	// Stamp is the file's size and modification time when it was read;
	// FileStamp returns the same text until the file changes.
	Stamp string `json:"stamp"`
}

// ReadFile reads a file of the folder from disk.
func ReadFile(ctx context.Context, dir, file string) (File, error) {
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	f, err := open(dir)
	if err != nil {
		return File{}, err
	}
	rel, real, err := f.resolve(file)
	if err != nil {
		return File{}, err
	}
	st, err := os.Stat(real)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", rel, err)
	}
	if !st.Mode().IsRegular() {
		return File{}, fmt.Errorf("%w: %s", ErrNotFile, rel)
	}
	out := File{Path: rel, Size: st.Size(), Lang: Lang(rel), Stamp: stampOf(st)}
	b, err := readHead(real, MaxFileBytes)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", rel, err)
	}
	if isBinary(b) {
		out.Binary = true
		return out, nil
	}
	if len(b) > MaxFileBytes {
		out.Truncated = true
		b = cutAtLine(b, MaxFileBytes)
	}
	if !out.Truncated && utf8.Valid(b) {
		out.Hash = hashOf(b)
	}
	out.Text = strings.ToValidUTF8(string(b), "�")
	out.Lines = countLines(b)
	return out, nil
}

func stampOf(st os.FileInfo) string {
	return fmt.Sprintf("%d:%d", st.Size(), st.ModTime().UnixNano())
}

// FileStamp is a file's Stamp now, for noticing cheaply that it changed on
// disk since it was read.
func FileStamp(ctx context.Context, dir, file string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f, err := open(dir)
	if err != nil {
		return "", err
	}
	rel, real, err := f.resolve(file)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(real)
	if err != nil {
		return "", fmt.Errorf("%s: %w", rel, err)
	}
	return stampOf(st), nil
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// SaveFile replaces the text of an existing file of the folder and returns it
// read again. base is the Hash the text was edited from; when the file on disk
// no longer has it, nothing is written and the error is ErrChanged. The new
// text goes to a temporary file beside it that is renamed over it, so a
// failed save leaves the old file whole; the file keeps its permissions.
func SaveFile(ctx context.Context, dir, file, text, base string) (File, error) {
	if err := ctx.Err(); err != nil {
		return File{}, err
	}
	if base == "" {
		return File{}, fmt.Errorf("%w: this file can't be edited here", ErrInvalid)
	}
	if len(text) > MaxSaveBytes {
		return File{}, fmt.Errorf("%w: more than %d MB of text", ErrInvalid, MaxSaveBytes>>20)
	}
	if !utf8.ValidString(text) {
		return File{}, fmt.Errorf("%w: the text is not valid UTF-8", ErrInvalid)
	}
	f, err := open(dir)
	if err != nil {
		return File{}, err
	}
	rel, real, err := f.resolve(file)
	if err != nil {
		return File{}, err
	}
	st, err := os.Stat(real)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", rel, err)
	}
	if !st.Mode().IsRegular() {
		return File{}, fmt.Errorf("%w: %s", ErrNotFile, rel)
	}
	cur, err := os.ReadFile(real)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", rel, err)
	}
	if hashOf(cur) != base {
		return File{}, fmt.Errorf("%w: %s", ErrChanged, rel)
	}
	if err := replaceFile(real, []byte(text), st.Mode().Perm()); err != nil {
		return File{}, fmt.Errorf("%s: %w", rel, err)
	}
	return ReadFile(ctx, dir, rel)
}

// replaceFile writes b to a temporary file in path's folder and renames it
// over path.
func replaceFile(path string, b []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".gwen-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = tmp.Close()
			_ = os.Remove(name)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	return nil
}

var langByExt = map[string]string{
	".go": "go", ".ts": "ts", ".mts": "ts", ".cts": "ts", ".tsx": "tsx", ".js": "js", ".mjs": "js", ".cjs": "js",
	".jsx": "jsx", ".json": "json", ".jsonc": "json", ".md": "md", ".markdown": "md", ".css": "css", ".scss": "css",
	".html": "html", ".htm": "html", ".vue": "html", ".svelte": "html", ".xml": "xml", ".svg": "xml",
	".sh": "sh", ".bash": "sh", ".zsh": "sh", ".fish": "sh", ".py": "py", ".rs": "rs", ".toml": "toml",
	".yaml": "yaml", ".yml": "yaml", ".sql": "sql", ".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp",
	".hpp": "cpp", ".java": "java", ".kt": "kotlin", ".swift": "swift", ".rb": "ruby", ".lua": "lua",
	".proto": "proto", ".ini": "ini", ".conf": "ini", ".cfg": "ini", ".desktop": "ini", ".service": "ini",
	".txt": "text", ".lock": "text", ".sum": "text", ".mod": "gomod", ".diff": "diff", ".patch": "diff",
}

var langByName = map[string]string{
	"makefile": "make", "gnumakefile": "make", "dockerfile": "docker", "containerfile": "docker",
	"go.mod": "gomod", "go.work": "gomod", ".gitignore": "ignore", ".dockerignore": "ignore",
	".bashrc": "sh", ".zshrc": "sh", ".profile": "sh", ".env": "sh", ".editorconfig": "ini",
	".gitattributes": "ignore", ".gitmodules": "ini", "pkgbuild": "sh",
}

// Lang names a file's language from its name: go, ts, tsx, js, jsx, json,
// md, css, html, xml, sh, py, rs, toml, yaml, sql, c, cpp, java, kotlin,
// swift, ruby, lua, proto, ini, make, docker, gomod, ignore, diff, text, or
// "" when unknown.
func Lang(file string) string {
	base := strings.ToLower(path.Base(file))
	if l, ok := langByName[base]; ok {
		return l
	}
	return langByExt[path.Ext(base)]
}
