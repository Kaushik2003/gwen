package remote

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// untrackedStat reads a file in pieces but must answer as reading it whole did:
// binary on a NUL in the first 8000 bytes, else countLines of the whole text.
func TestUntrackedStatMatchesWholeRead(t *testing.T) {
	big := strings.Repeat("a line of text\n", 200_000) // ~3 MB, many reads
	cases := map[string]string{
		"empty":            "",
		"one line":         "hello",
		"one line, eol":    "hello\n",
		"just a newline":   "\n",
		"two lines":        "a\nb",
		"two lines, eol":   "a\nb\n",
		"blank lines":      "\n\n\n",
		"big":              big,
		"big, no eol":      big + "tail",
		"nul at start":     "\x00rest",
		"nul at 7999":      strings.Repeat("x", 7999) + "\x00",
		"nul at 8000 only": strings.Repeat("x", 8000) + "\x00\n",
		"nul in big, late": big + "\x00",
	}
	dir := t.TempDir()
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, strings.ReplaceAll(name, " ", "_"))
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			data := []byte(content)
			wantBinary := bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0
			wantLines := 0
			if !wantBinary {
				wantLines = countLines(content)
			}
			binary, lines, err := untrackedStat(path)
			if err != nil {
				t.Fatal(err)
			}
			if binary != wantBinary || lines != wantLines {
				t.Errorf("untrackedStat = (binary %v, lines %d), want (%v, %d)", binary, lines, wantBinary, wantLines)
			}
		})
	}
	if _, _, err := untrackedStat(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing file should be an error")
	}
}
