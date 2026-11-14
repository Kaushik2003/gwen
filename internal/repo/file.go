package repo

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
)

// File is a file's text for the file viewer. Files are read-only here.
type File struct {
	Path      string `json:"path"`
	Text      string `json:"text"`   // "" when Binary
	Binary    bool   `json:"binary"` // a NUL in the first 8000 bytes
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"` // over MaxFileBytes: Text is the first lines that fit
	Lines     int    `json:"lines"`     // lines in Text
	Lang      string `json:"lang"`      // see Lang
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
	out := File{Path: rel, Size: st.Size(), Lang: Lang(rel)}
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
	out.Text = strings.ToValidUTF8(string(b), "�")
	out.Lines = countLines(b)
	return out, nil
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
