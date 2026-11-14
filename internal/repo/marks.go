package repo

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Marks are the lines the file viewer marks in its gutter: the uncommitted
// changes when there are any, else the lines of the last commit that touched
// the file.
type Marks struct {
	Source  string `json:"source"` // "uncommitted", "commit", or "" (nothing to mark)
	Commit  string `json:"commit"` // for "commit"
	Short   string `json:"short"`
	Subject string `json:"subject"`
	When    int64  `json:"when"` // the commit's author time, unix ms
	Lines   []Mark `json:"lines"`
}

// Mark is one marked line.
type Mark struct {
	Line int `json:"line"` // in the current file, from 1
	// Kind is "add", "mod", or "del": lines were deleted after Line (0: before the first).
	Kind string `json:"kind"`
}

// LineMarks finds the lines to mark in a file. Outside a repository there
// are none.
func LineMarks(ctx context.Context, dir, file string) (Marks, error) {
	f, err := open(dir)
	if err != nil {
		return Marks{}, err
	}
	rel, real, err := f.resolve(file)
	if err != nil {
		return Marks{}, err
	}
	m := Marks{Lines: []Mark{}}
	prefix, err := f.prefix(ctx)
	if errors.Is(err, ErrNotRepo) {
		return m, nil
	}
	if err != nil {
		return Marks{}, err
	}
	st, err := f.status(ctx, prefix, rel)
	if err != nil {
		return Marks{}, err
	}
	head, hasHead, err := f.head(ctx)
	if err != nil {
		return Marks{}, err
	}
	if len(st.entries) > 0 {
		m.Source = "uncommitted"
		if st.entries[0].kind == '?' || !hasHead {
			m.Lines = allAdded(lineCount(real))
			return m, nil
		}
		paths := []string{rel}
		if e := st.entries[0]; e.orig != "" {
			paths = []string{e.orig, rel}
		}
		out, err := f.git(ctx, append([]string{"diff", "HEAD", "-U0", "-M", "--no-ext-diff", "--no-textconv", "--no-color", "--relative", "--"}, paths...)...)
		if err != nil {
			return Marks{}, err
		}
		m.Lines = parseMarks(out)
		return m, nil
	}
	if !hasHead {
		return m, nil
	}
	out, err := f.git(ctx, "log", "-1", "--no-merges", "--format=%H%x00%h%x00%at%x00%s", head, "--", rel)
	if err != nil {
		return Marks{}, err
	}
	fields := strings.SplitN(strings.TrimRight(string(out), "\n"), "\x00", 4)
	if len(fields) < 4 {
		return m, nil // untouched by any commit but merges, or ignored
	}
	at, _ := strconv.ParseInt(fields[2], 10, 64)
	m.Source, m.Commit, m.Short, m.When, m.Subject = "commit", fields[0], fields[1], at*1000, fields[3]
	paths, err := f.withOldName(ctx, m.Commit, rel)
	if err != nil {
		return Marks{}, err
	}
	out, err = f.git(ctx, append([]string{"show", "-U0", "--format=", "-M", "--no-ext-diff", "--no-textconv", "--no-color",
		"--diff-merges=first-parent", "--relative", m.Commit, "--"}, paths...)...)
	if err != nil {
		return Marks{}, err
	}
	m.Lines = parseMarks(out)
	return m, nil
}

func allAdded(n int) []Mark {
	lines := make([]Mark, n)
	for i := range lines {
		lines[i] = Mark{Line: i + 1, Kind: "add"}
	}
	return lines
}

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// parseMarks reads the hunk headers of a -U0 diff of one file.
func parseMarks(diff []byte) []Mark {
	marks := []Mark{}
	sc := bufio.NewScanner(bytes.NewReader(diff))
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		h := hunkHeader.FindSubmatch(sc.Bytes())
		if h == nil {
			continue
		}
		oldCount, newStart, newCount := count(h[2]), atoi(h[3]), count(h[4])
		switch {
		case newCount == 0:
			marks = append(marks, Mark{Line: newStart, Kind: "del"})
		case oldCount == 0:
			for i := range newCount {
				marks = append(marks, Mark{Line: newStart + i, Kind: "add"})
			}
		default:
			for i := range newCount {
				marks = append(marks, Mark{Line: newStart + i, Kind: "mod"})
			}
		}
	}
	return marks
}

func atoi(b []byte) int { n, _ := strconv.Atoi(string(b)); return n }

// count is a hunk header's line count, which is 1 when left out.
func count(b []byte) int {
	if b == nil {
		return 1
	}
	return atoi(b)
}

// withOldName returns rel, and before it the file's old name when the
// commit renamed it, so a diff limited to it still sees the rename.
func (f folder) withOldName(ctx context.Context, commit, rel string) ([]string, error) {
	out, err := f.git(ctx, "show", "--format=", "--raw", "-z", "-M", "--diff-merges=first-parent", "--relative", commit)
	if err != nil {
		return nil, err
	}
	for _, fc := range parseRawNumstat(out) {
		if fc.Path == rel && fc.OldPath != "" {
			return []string{fc.OldPath, rel}, nil
		}
	}
	return []string{rel}, nil
}

// BlameLine is who last changed one line.
type BlameLine struct {
	Commit      string `json:"commit"`
	Short       string `json:"short"`
	Author      string `json:"author"`
	Summary     string `json:"summary"`
	When        int64  `json:"when"` // author time, unix ms
	Uncommitted bool   `json:"uncommitted"`
}

// Blame finds the commit that last changed a line of a file, counting from 1.
func Blame(ctx context.Context, dir, file string, line int) (BlameLine, error) {
	if line < 1 {
		return BlameLine{}, fmt.Errorf("%w: line %d", ErrInvalid, line)
	}
	f, err := open(dir)
	if err != nil {
		return BlameLine{}, err
	}
	rel, _, err := f.resolve(file)
	if err != nil {
		return BlameLine{}, err
	}
	prefix, err := f.prefix(ctx)
	if err != nil {
		return BlameLine{}, err
	}
	st, err := f.status(ctx, prefix, rel)
	if err != nil {
		return BlameLine{}, err
	}
	_, hasHead, err := f.head(ctx)
	if err != nil {
		return BlameLine{}, err
	}
	if !hasHead || (len(st.entries) > 0 && st.entries[0].kind == '?') {
		return BlameLine{Uncommitted: true}, nil
	}
	n := strconv.Itoa(line)
	out, err := f.git(ctx, "-c", "blame.ignoreRevsFile=", "blame", "--porcelain", "-L", n+","+n, "--", rel)
	if err != nil {
		return BlameLine{}, err
	}
	return parseBlame(out)
}

func parseBlame(out []byte) (BlameLine, error) {
	lines := strings.Split(string(out), "\n")
	first := strings.Fields(lines[0])
	if len(first) < 3 {
		return BlameLine{}, fmt.Errorf("git blame: unexpected output %q", lines[0])
	}
	var b BlameLine
	b.Commit = first[0]
	if strings.Trim(b.Commit, "0") == "" {
		return BlameLine{Uncommitted: true}, nil
	}
	b.Short = b.Commit[:min(7, len(b.Commit))]
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "\t") {
			break
		}
		key, val, _ := strings.Cut(l, " ")
		switch key {
		case "author":
			b.Author = val
		case "author-time":
			at, _ := strconv.ParseInt(val, 10, 64)
			b.When = at * 1000
		case "summary":
			b.Summary = val
		}
	}
	return b, nil
}
