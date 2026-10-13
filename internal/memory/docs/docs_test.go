package docs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

func TestReadsPlainText(t *testing.T) {
	ctx := context.Background()
	for name, mime := range map[string]string{
		"notes.txt": "text/plain", "app.LOG": "text/plain", "plan.md": "text/markdown",
		"plan.markdown": "text/markdown", "todo.org": "text/org", "table.csv": "text/csv", "data.json": "application/json",
	} {
		text, got, err := Read(ctx, writeFile(t, name, "line one\nline two\n"))
		require.NoError(t, err, name)
		require.Equal(t, mime, got, name)
		require.Equal(t, "line one\nline two\n", text, name)
	}
}

func TestTextIsCleanUTF8(t *testing.T) {
	text, _, err := Read(context.Background(), writeFile(t, "a.txt", "\uFEFFcaf\xe9 \x00ok\n日本語"))
	require.NoError(t, err)
	require.Equal(t, "caf\uFFFD ok\n日本語", text, "byte order mark and NUL dropped, invalid byte replaced")
	require.True(t, utf8.ValidString(text))
}

func TestReadIsBounded(t *testing.T) {
	long := strings.Repeat("é", MaxChars+500)
	text, _, err := Read(context.Background(), writeFile(t, "long.md", long))
	require.NoError(t, err)
	require.Equal(t, MaxChars, utf8.RuneCountInString(text))

	big := writeFile(t, "big.txt", strings.Repeat("a", MaxBytes+1000))
	raw, err := readFile(big)
	require.NoError(t, err)
	require.Len(t, raw, MaxBytes)
}

func TestStripsHTML(t *testing.T) {
	page := `<!doctype html><html><head><title>Lease</title><style>p { color: red }</style></head>
<body><h1>Tenancy&nbsp;agreement</h1><!-- draft -->
<p>The rent is <b>&pound;900</b> a month.</p><script>alert("x < y")</script>
<ul><li>Deposit: two months</li><li>Pets &amp; plants allowed</li></ul><br>Signed.</body></html>`
	text, mime, err := Read(context.Background(), writeFile(t, "lease.HTM", page))
	require.NoError(t, err)
	require.Equal(t, "text/html", mime)
	require.Equal(t, "Tenancy agreement\n\nThe rent is £900 a month.\n\nDeposit: two months\n\n"+
		"Pets & plants allowed\n\nSigned.", text)
}

func TestRefusesWhatItCannotRead(t *testing.T) {
	ctx := context.Background()
	_, _, err := Read(ctx, writeFile(t, "report.docx", "PK"))
	require.ErrorIs(t, err, ErrUnsupported)
	require.ErrorContains(t, err, ".docx files")
	_, _, err = Read(ctx, writeFile(t, "Makefile", "all:"))
	require.ErrorIs(t, err, ErrUnsupported)
	require.ErrorContains(t, err, "no extension")

	_, _, err = Read(ctx, filepath.Join(t.TempDir(), "missing.txt"))
	require.ErrorIs(t, err, os.ErrNotExist)
	dir := filepath.Join(t.TempDir(), "folder.txt")
	require.NoError(t, os.Mkdir(dir, 0o700))
	_, _, err = Read(ctx, dir)
	require.ErrorContains(t, err, "is not a file")

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, _, err = Read(cancelled, writeFile(t, "a.txt", "x"))
	require.ErrorIs(t, err, context.Canceled)
}

// fakePDFToText puts a pdftotext that runs script on PATH, alone.
func fakePDFToText(t *testing.T, script string) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pdftotext"), []byte("#!/bin/sh\n"+script+"\n"), 0o700))
	t.Setenv("PATH", dir)
}

func TestPDFGoesThroughPDFToText(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep is not on PATH")
	}
	pdf := writeFile(t, "-scan.pdf", "%PDF-1.4")
	t.Chdir(filepath.Dir(pdf))
	const rel = "-scan.pdf"
	abs, err := filepath.Abs(rel)
	require.NoError(t, err)

	fakePDFToText(t, `printf '%s\n' "$@"`)
	text, mime, err := Read(context.Background(), rel)
	require.NoError(t, err)
	require.Equal(t, "application/pdf", mime)
	require.Equal(t, "-layout\n-enc\nUTF-8\n"+abs+"\n-\n", text, "the path is absolute, so never read as an option")

	fakePDFToText(t, `echo "Syntax Error: Couldn't read xref table" >&2; exit 1`)
	_, _, err = Read(context.Background(), pdf)
	require.ErrorContains(t, err, "pdftotext could not read the PDF: Syntax Error: Couldn't read xref table")

	fakePDFToText(t, "exec "+sleep+" 5")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, _, err = Read(ctx, pdf)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	t.Setenv("PATH", t.TempDir())
	_, _, err = Read(context.Background(), pdf)
	require.ErrorIs(t, err, ErrNoPDFReader)
	require.ErrorContains(t, err, "install poppler-utils")
}

func TestCappedKeepsTheStart(t *testing.T) {
	c := &capped{max: 5}
	for _, s := range []string{"abc", "defg", "hij"} {
		n, err := c.Write([]byte(s))
		require.NoError(t, err)
		require.Equal(t, len(s), n, "the writer never stalls pdftotext")
	}
	require.Equal(t, "abcde", c.String())
}

// minimalPDF is a one-page PDF that says text.
func minimalPDF(text string) []byte {
	stream := fmt.Sprintf("BT /F1 18 Tf 20 100 Td (%s) Tj ET", text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 400 144] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, o := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}

func TestReadsARealPDF(t *testing.T) {
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext is not on PATH")
	}
	p := filepath.Join(t.TempDir(), "hello.pdf")
	require.NoError(t, os.WriteFile(p, minimalPDF("Hello from a PDF"), 0o600))
	text, mime, err := Read(context.Background(), p)
	require.NoError(t, err)
	require.Equal(t, "application/pdf", mime)
	require.Equal(t, "Hello from a PDF", strings.TrimSpace(text))
}
