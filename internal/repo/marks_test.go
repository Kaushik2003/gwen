package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMarks(t *testing.T) {
	t.Parallel()
	diff := "diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n" +
		"@@ -0,0 +1,2 @@\n+a\n+b\n" + // added at the top
		"@@ -5 +7 @@ func x() {\n-old\n+new\n" + // one line changed, counts left out
		"@@ -9,2 +10,0 @@\n-gone\n-gone\n" + // deleted after line 10
		"@@ -20,0 +21 @@\n+one more\n"
	require.Equal(t, []Mark{
		{Line: 1, Kind: "add"}, {Line: 2, Kind: "add"},
		{Line: 7, Kind: "mod"},
		{Line: 10, Kind: "del"},
		{Line: 21, Kind: "add"},
	}, parseMarks([]byte(diff)))
	require.Empty(t, parseMarks(nil))
}

func TestLineMarks(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", lines(10, "line"))
	r.write("other.txt", "other\n")
	r.commitAll("first")
	content := strings.Replace(lines(10, "line"), "line 3\n", "line three\n", 1)
	content = strings.Replace(content, "line 6\nline 7\n", "", 1)
	content += "line 11\nline 12\n"
	r.write("f.txt", content)
	second := r.commitAll("second: edit f")
	r.write("other.txt", "other\nmore\n")
	r.commitAll("third: other only")

	// Clean: the lines of the last commit that touched the file.
	m, err := LineMarks(ctx(), r.dir, "f.txt")
	require.NoError(t, err)
	require.Equal(t, "commit", m.Source)
	require.Equal(t, second, m.Commit)
	require.Equal(t, second[:7], m.Short)
	require.Equal(t, "second: edit f", m.Subject)
	require.EqualValues(t, (day0+60)*1000, m.When)
	require.Equal(t, []Mark{{3, "mod"}, {5, "del"}, {9, "add"}, {10, "add"}}, m.Lines)

	// Uncommitted changes win.
	r.write("f.txt", "new top\n"+content)
	m, err = LineMarks(ctx(), r.dir, "f.txt")
	require.NoError(t, err)
	require.Equal(t, Marks{Source: "uncommitted", Lines: []Mark{{1, "add"}}}, m)

	// A new file: every line added.
	r.write("new.txt", "a\nb\nc")
	m, err = LineMarks(ctx(), r.dir, "new.txt")
	require.NoError(t, err)
	require.Equal(t, Marks{Source: "uncommitted", Lines: []Mark{{1, "add"}, {2, "add"}, {3, "add"}}}, m)

	// A file renamed in its last commit marks only what changed.
	r.git("checkout", "-q", "--", "f.txt")
	r.git("mv", "f.txt", "g.txt")
	r.write("g.txt", content+"line 13\n")
	r.git("add", "g.txt")
	r.commit("rename f to g")
	m, err = LineMarks(ctx(), r.dir, "g.txt")
	require.NoError(t, err)
	require.Equal(t, "commit", m.Source)
	require.Equal(t, []Mark{{11, "add"}}, m.Lines)

	_, err = LineMarks(ctx(), r.dir, "../x")
	require.ErrorIs(t, err, ErrOutside)
}

func TestLineMarksWithoutGitOrCommits(t *testing.T) {
	t.Parallel()
	needGit(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644))
	m, err := LineMarks(ctx(), dir, "a.txt")
	require.NoError(t, err)
	require.Equal(t, Marks{Lines: []Mark{}}, m)

	r := newRepo(t)
	r.write("a.txt", "a\nb\n")
	r.git("add", "a.txt")
	m, err = LineMarks(ctx(), r.dir, "a.txt")
	require.NoError(t, err)
	require.Equal(t, Marks{Source: "uncommitted", Lines: []Mark{{1, "add"}, {2, "add"}}}, m)
}

func TestBlame(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("f.txt", "one\ntwo\n")
	first := r.commitAll("first")
	r.write("f.txt", "one\nTWO\n")
	r.git("add", "-A")
	second := r.commit("second", "Grace Hopper", "grace@example.com")
	r.write("f.txt", "one\nTWO\nthree\n")

	b, err := Blame(ctx(), r.dir, "f.txt", 1)
	require.NoError(t, err)
	require.Equal(t, BlameLine{Commit: first, Short: first[:7], Author: "Ada Lovelace", Summary: "first", When: day0 * 1000}, b)
	b, err = Blame(ctx(), r.dir, "f.txt", 2)
	require.NoError(t, err)
	require.Equal(t, BlameLine{Commit: second, Short: second[:7], Author: "Grace Hopper", Summary: "second", When: (day0 + 60) * 1000}, b)
	b, err = Blame(ctx(), r.dir, "f.txt", 3)
	require.NoError(t, err)
	require.Equal(t, BlameLine{Uncommitted: true}, b)

	r.write("new.txt", "fresh\n")
	b, err = Blame(ctx(), r.dir, "new.txt", 1)
	require.NoError(t, err)
	require.Equal(t, BlameLine{Uncommitted: true}, b)

	_, err = Blame(ctx(), r.dir, "f.txt", 0)
	require.ErrorIs(t, err, ErrInvalid)
	_, err = Blame(ctx(), r.dir, "f.txt", 99)
	require.Error(t, err)
	_, err = Blame(ctx(), t.TempDir(), "x", 1)
	require.ErrorIs(t, err, ErrNotRepo)
}
