// Package docs reads the text of a document the user gives Gwen to remember:
// plain text and Markdown as they are, HTML with its tags stripped, and PDF
// through poppler's pdftotext.
package docs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/memory"
)

// MaxBytes is the most of a file, or of pdftotext's output, that is read.
const MaxBytes = 4 << 20

// MaxChars is the most text kept of a document.
const MaxChars = 200_000

// pdfTimeout bounds pdftotext.
const pdfTimeout = 30 * time.Second

// ErrUnsupported is matched by the error for a type Gwen cannot read.
var ErrUnsupported = errors.New("unsupported document type")

// ErrNoPDFReader is matched by the error for a PDF when pdftotext is
// missing.
var ErrNoPDFReader = errors.New("reading PDFs needs pdftotext; install poppler-utils")

// plain are the extensions read as they are, with their media types.
var plain = map[string]string{
	".txt":      "text/plain",
	".log":      "text/plain",
	".md":       "text/markdown",
	".markdown": "text/markdown",
	".org":      "text/org",
	".csv":      "text/csv",
	".json":     "application/json",
}

// Read returns the text of the document at path and its media type. Text is
// valid UTF-8 (invalid bytes replaced), without NUL bytes or a byte order
// mark, from at most MaxBytes of the file and cut to MaxChars characters.
// Plain text, Markdown, Org, CSV, JSON, and logs are read as they are; HTML
// has its tags stripped; a PDF is read by pdftotext (ErrNoPDFReader without
// it). Any other type is ErrUnsupported.
func Read(ctx context.Context, path string) (text, mime string, err error) {
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	ext := strings.ToLower(filepath.Ext(path))
	var raw []byte
	switch ext {
	case ".html", ".htm":
		mime = "text/html"
		if raw, err = readFile(path); err == nil {
			raw = []byte(stripHTML(clean(raw)))
		}
	case ".pdf":
		mime = "application/pdf"
		raw, err = readPDF(ctx, path)
	default:
		var ok bool
		if mime, ok = plain[ext]; !ok {
			if ext == "" {
				return "", "", fmt.Errorf("%w: a file with no extension", ErrUnsupported)
			}
			return "", "", fmt.Errorf("%w: %s files", ErrUnsupported, ext)
		}
		raw, err = readFile(path)
	}
	if err != nil {
		return "", "", err
	}
	text = clean(raw)
	if utf8.RuneCountInString(text) > MaxChars {
		text = string([]rune(text)[:MaxChars])
	}
	return text, mime, nil
}

// readFile reads at most MaxBytes of the regular file at path.
func readFile(path string) ([]byte, error) {
	if err := isFile(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, MaxBytes))
}

// isFile is nil when path names a regular file.
func isFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a file", path)
	}
	return nil
}

// readPDF runs pdftotext on the PDF at path and returns at most MaxBytes of
// its text.
func readPDF(ctx context.Context, path string) ([]byte, error) {
	if err := isFile(path); err != nil { // a missing file says so, not pdftotext
		return nil, err
	}
	bin, err := exec.LookPath("pdftotext")
	if err != nil {
		return nil, ErrNoPDFReader
	}
	abs, err := filepath.Abs(path) // never mistaken for an option
	if err != nil {
		return nil, err
	}
	parent := ctx
	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()
	out := &capped{max: MaxBytes}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, "-layout", "-enc", "UTF-8", abs, "-")
	cmd.Stdout, cmd.Stderr = out, &stderr
	if err := cmd.Run(); err != nil {
		if err := parent.Err(); err != nil {
			return nil, err
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("pdftotext took longer than %s", pdfTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("pdftotext could not read the PDF: %s", memory.Cut(msg, 200))
	}
	return out.Bytes(), nil
}

// capped keeps the first max bytes written to it and drops the rest.
type capped struct {
	bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.Len(); room > 0 {
		c.Buffer.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

// clean makes raw valid UTF-8 text: invalid bytes replaced, NUL bytes and a
// leading byte order mark dropped.
func clean(raw []byte) string {
	s := strings.ToValidUTF8(string(raw), "\uFFFD")
	s = strings.TrimPrefix(s, "\uFEFF")
	return strings.ReplaceAll(s, "\x00", "")
}

var (
	// hidden are elements whose content is not text to read. RE2 has no
	// backreferences, so any of their end tags closes one: crude, as meant.
	hidden = regexp.MustCompile(`(?is)<(script|style|noscript|template|svg|head)\b[^>]*>.*?` +
		`</(script|style|noscript|template|svg|head)\s*>`)
	// comment is an HTML comment.
	comment = regexp.MustCompile(`(?s)<!--.*?-->`)
	// block are tags that end a line of text.
	block = regexp.MustCompile(`(?i)</?(p|div|br|li|ul|ol|tr|table|h[1-6]|section|article|header|footer|` +
		`blockquote|pre|hr)\b[^>]*>`)
	// tag is any other tag.
	tag = regexp.MustCompile(`(?s)<[^>]*>`)
	// spaces are runs of spaces and tabs.
	spaces = regexp.MustCompile(`[ \t\f\v\r\x{00A0}]+`)
	// blanks are runs of blank lines.
	blanks = regexp.MustCompile(`\n\s*\n+`)
)

// stripHTML is the text of an HTML page, crudely: scripts, styles, and
// comments dropped, block tags made line breaks, other tags removed,
// entities decoded, and spaces collapsed.
func stripHTML(s string) string {
	s = hidden.ReplaceAllString(s, " ")
	s = comment.ReplaceAllString(s, " ")
	s = block.ReplaceAllString(s, "\n")
	s = tag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = spaces.ReplaceAllString(s, " ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(l)
	}
	s = blanks.ReplaceAllString(strings.Join(lines, "\n"), "\n\n")
	return strings.TrimSpace(s)
}
