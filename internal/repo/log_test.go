package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// history builds:
//
//   - merge   (main, HEAD)   merge feature
//     |\
//     | * feat2 (feature)      feature two: rename a.txt to b.txt
//     | * feat1                feature one (by Grace)
//   - | main2                main two: binary file
//     |/
//   - root    (tag v1)       root
type history struct {
	*fixture
	root, feat1, feat2, main2, merge string
}

func newHistory(t *testing.T) history {
	t.Helper()
	h := history{fixture: newRepo(t)}
	h.write("a.txt", lines(5, "a"))
	h.write("docs/readme.md", "# readme\n")
	h.root = h.commitAll("root\n\nThe first commit.\nIt has a body.")
	h.git("tag", "v1")
	h.git("checkout", "-q", "-b", "feature")
	h.write("feature.txt", "feature\n")
	h.git("add", "-A")
	h.feat1 = h.commit("feature one", "Grace Hopper", "grace@example.com")
	h.git("mv", "a.txt", "b.txt")
	h.write("b.txt", lines(5, "a")+"a 6\n")
	h.feat2 = h.commitAll("feature two: rename a.txt to b.txt")
	h.git("checkout", "-q", "main")
	h.write("img.bin", "\x00\x01\x02binary")
	h.write("docs/readme.md", "# readme\n\nmore\n")
	h.main2 = h.commitAll("main two: binary file")
	date := []string{"GIT_AUTHOR_DATE=@1767232800 +0000", "GIT_COMMITTER_DATE=@1767232800 +0000"}
	h.gitEnv(date, "merge", "-q", "--no-ff", "-m", "merge feature", "feature")
	h.merge = strings.TrimSpace(h.git("rev-parse", "HEAD"))
	return h
}

func hashes(cs []Commit) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Hash)
	}
	return out
}

func TestHistory(t *testing.T) {
	t.Parallel()
	h := newHistory(t)

	l, err := History(ctx(), h.dir, LogQuery{})
	require.NoError(t, err)
	require.False(t, l.More)
	require.Nil(t, l.Uncommitted, "clean")
	require.Equal(t, "main", l.Branch)
	require.Equal(t, []string{h.merge, h.feat2, h.feat1, h.main2, h.root}, hashes(l.Commits), "topological, the feature branch kept together")

	merge := l.Commits[0]
	require.Equal(t, []string{h.main2, h.feat2}, merge.Parents)
	require.Equal(t, []Ref{{"HEAD", "head"}, {"main", "branch"}}, merge.Refs)
	require.Equal(t, "merge feature", merge.Subject)
	require.Zero(t, merge.Files, "merges carry no counts in the list")

	feat2 := l.Commits[1]
	require.Equal(t, []Ref{{"feature", "branch"}}, feat2.Refs)
	require.Equal(t, Commit{Hash: h.feat2, Short: h.feat2[:7], Parents: []string{h.feat1}, Subject: "feature two: rename a.txt to b.txt",
		Author: "Ada Lovelace", AuthorEmail: "ada@example.com", When: (day0 + 120) * 1000, Refs: feat2.Refs, Added: 1, Removed: 0, Files: 1}, feat2)

	main2 := l.Commits[3]
	require.Equal(t, 2, main2.Files, "binary files count, without lines")
	require.Equal(t, 2, main2.Added)

	root := l.Commits[4]
	require.Equal(t, []Ref{{"v1", "tag"}}, root.Refs)
	require.Equal(t, []string{}, root.Parents)
	require.Equal(t, "root", root.Subject)

	// Pages.
	l, err = History(ctx(), h.dir, LogQuery{Limit: 2})
	require.NoError(t, err)
	require.True(t, l.More)
	require.Equal(t, []string{h.merge, h.feat2}, hashes(l.Commits))
	l, err = History(ctx(), h.dir, LogQuery{Limit: 2, Skip: 4})
	require.NoError(t, err)
	require.False(t, l.More)
	require.Equal(t, []string{h.root}, hashes(l.Commits))
	require.Empty(t, l.Branch, "branch facts come with the first page")

	// Filters.
	l, err = History(ctx(), h.dir, LogQuery{Search: "FEATURE T"})
	require.NoError(t, err)
	require.Equal(t, []string{h.feat2}, hashes(l.Commits), "message search ignores case, takes text as it is")
	l, err = History(ctx(), h.dir, LogQuery{Search: "a.txt"})
	require.NoError(t, err)
	require.Equal(t, []string{h.feat2}, hashes(l.Commits), "a dot is a dot, not any character")
	l, err = History(ctx(), h.dir, LogQuery{Author: "grace"})
	require.NoError(t, err)
	require.Equal(t, []string{h.feat1}, hashes(l.Commits))
	l, err = History(ctx(), h.dir, LogQuery{Path: "docs/readme.md"})
	require.NoError(t, err)
	require.Equal(t, []string{h.main2, h.root}, hashes(l.Commits))
	require.Equal(t, []string{h.root}, l.Commits[0].Parents, "parents rewritten to the commits the filter keeps")
	_, err = History(ctx(), h.dir, LogQuery{Path: "../elsewhere"})
	require.ErrorIs(t, err, ErrOutside)
	l, err = History(ctx(), h.dir, LogQuery{Search: "-nothing like this"})
	require.NoError(t, err)
	require.Empty(t, l.Commits, "a search is never an option")
	require.NotNil(t, l.Commits)

	// Uncommitted work shows on the first page.
	h.write("b.txt", "changed\n")
	h.write("new.txt", "one\ntwo\n")
	l, err = History(ctx(), h.dir, LogQuery{})
	require.NoError(t, err)
	require.Equal(t, &Uncommitted{Files: 2, Added: 3, Removed: 6}, l.Uncommitted)
	l, err = History(ctx(), h.dir, LogQuery{Path: "new.txt"})
	require.NoError(t, err)
	require.Equal(t, &Uncommitted{Files: 1, Added: 2}, l.Uncommitted)
	l, err = History(ctx(), h.dir, LogQuery{Search: "root"})
	require.NoError(t, err)
	require.Nil(t, l.Uncommitted, "a search is about commits")

	// Remote branches and their HEAD.
	h.git("update-ref", "refs/remotes/origin/main", h.main2)
	h.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	l, err = History(ctx(), h.dir, LogQuery{})
	require.NoError(t, err)
	require.Equal(t, []Ref{{"origin/main", "remote"}}, l.Commits[3].Refs)
}

func TestHistoryEdges(t *testing.T) {
	t.Parallel()
	needGit(t)
	_, err := History(ctx(), t.TempDir(), LogQuery{})
	require.ErrorIs(t, err, ErrNotRepo)

	r := newRepo(t)
	l, err := History(ctx(), r.dir, LogQuery{})
	require.NoError(t, err)
	require.Empty(t, l.Commits, "no commits yet")
	require.Nil(t, l.Uncommitted)
	r.write("a.txt", "a\n")
	l, err = History(ctx(), r.dir, LogQuery{})
	require.NoError(t, err)
	require.Equal(t, &Uncommitted{Files: 1, Added: 1}, l.Uncommitted)
	require.Equal(t, "main", l.Branch)

	r.commitAll("one")
	r.commitAll("two")
	r.commitAll("three")
	l, err = History(ctx(), r.dir, LogQuery{Limit: -1, Skip: -5})
	require.NoError(t, err)
	require.Len(t, l.Commits, 3, "bad numbers mean the defaults")
}

func TestHistoryInASubfolder(t *testing.T) {
	t.Parallel()
	r := newRepo(t)
	r.write("app/a.txt", "a\n")
	r.write("other/b.txt", "b\n")
	inApp := r.commitAll("both")
	r.write("other/b.txt", "b\nb\n")
	r.commitAll("other only")
	r.write("app/a.txt", "a\na\n")
	r.write("other/b.txt", "b\nb\nb\n")

	l, err := History(ctx(), filepath.Join(r.dir, "app"), LogQuery{})
	require.NoError(t, err)
	require.Equal(t, []string{inApp}, hashes(l.Commits), "only commits that touch the folder")
	require.Equal(t, 1, l.Commits[0].Files, "counted inside the folder")
	require.Equal(t, &Uncommitted{Files: 1, Added: 1}, l.Uncommitted)

	d, err := Show(ctx(), filepath.Join(r.dir, "app"), inApp)
	require.NoError(t, err)
	require.Equal(t, []FileChange{{Path: "a.txt", Status: "A", Added: 1}}, d.Changes)
	diff, err := Diff(ctx(), filepath.Join(r.dir, "app"), "", "a.txt")
	require.NoError(t, err)
	require.Contains(t, diff.Text, "--- a/a.txt\n+++ b/a.txt\n")
}

func TestShow(t *testing.T) {
	t.Parallel()
	h := newHistory(t)

	d, err := Show(ctx(), h.dir, h.root)
	require.NoError(t, err)
	require.Equal(t, "root", d.Subject)
	require.Equal(t, "The first commit.\nIt has a body.", d.Body)
	require.Equal(t, []Ref{{"v1", "tag"}}, d.Refs)
	require.Equal(t, []FileChange{{Path: "a.txt", Status: "A", Added: 5}, {Path: "docs/readme.md", Status: "A", Added: 1}}, d.Changes)
	require.Equal(t, 6, d.Added)
	require.Equal(t, 2, d.Files)

	d, err = Show(ctx(), h.dir, h.feat2[:10])
	require.NoError(t, err)
	require.Equal(t, h.feat2, d.Hash, "a short id finds the commit")
	require.Equal(t, []FileChange{{Path: "b.txt", OldPath: "a.txt", Status: "R", Added: 1}}, d.Changes)

	d, err = Show(ctx(), h.dir, h.main2)
	require.NoError(t, err)
	require.Equal(t, []FileChange{{Path: "docs/readme.md", Status: "M", Added: 2}, {Path: "img.bin", Status: "A", Binary: true}}, d.Changes)

	d, err = Show(ctx(), h.dir, h.merge)
	require.NoError(t, err)
	require.Equal(t, []string{h.main2, h.feat2}, d.Parents)
	require.Equal(t, []FileChange{
		{Path: "b.txt", OldPath: "a.txt", Status: "R", Added: 1},
		{Path: "feature.txt", Status: "A", Added: 1},
	}, d.Changes, "a merge against its first parent")

	for _, bad := range []string{"--all", "HEAD", "main", "abc", "zzzzzzz", strings.Repeat("0", 40)} {
		_, err = Show(ctx(), h.dir, bad)
		require.ErrorIs(t, err, ErrBadRev, bad)
	}

	// The uncommitted work.
	d, err = Show(ctx(), h.dir, "")
	require.NoError(t, err)
	require.Empty(t, d.Changes)
	require.Equal(t, []string{h.merge}, d.Parents)
	h.write("b.txt", lines(5, "a"))
	h.write("fresh.txt", "x\ny\n")
	h.write("blob.bin", "\x00\x00")
	require.NoError(t, os.Remove(filepath.Join(h.dir, "feature.txt")))
	h.git("mv", "docs/readme.md", "docs/README.md")
	d, err = Show(ctx(), h.dir, "")
	require.NoError(t, err)
	require.Equal(t, []FileChange{
		{Path: "b.txt", Status: "M", Removed: 1},
		{Path: "blob.bin", Status: "U", Binary: true},
		{Path: "docs/README.md", OldPath: "docs/readme.md", Status: "R"},
		{Path: "feature.txt", Status: "D", Removed: 1},
		{Path: "fresh.txt", Status: "U", Added: 2},
	}, d.Changes)
	require.Equal(t, []Ref{{"HEAD", "head"}, {"main", "branch"}}, d.Refs)
	require.Equal(t, 2, d.Added)
	require.Equal(t, 2, d.Removed)
	require.Equal(t, 5, d.Files)
}

func TestDiff(t *testing.T) {
	t.Parallel()
	h := newHistory(t)

	d, err := Diff(ctx(), h.dir, h.feat2, "b.txt")
	require.NoError(t, err)
	require.Equal(t, "b.txt", d.Path)
	require.Contains(t, d.Text, "rename from a.txt\nrename to b.txt\n", "the rename, not a new file")
	require.Contains(t, d.Text, "@@ -3,3 +3,4 @@ a 2\n a 3\n a 4\n a 5\n+a 6\n")
	require.False(t, d.Binary)

	d, err = Diff(ctx(), h.dir, h.main2, "img.bin")
	require.NoError(t, err)
	require.True(t, d.Binary)

	d, err = Diff(ctx(), h.dir, h.merge, "feature.txt")
	require.NoError(t, err)
	require.Contains(t, d.Text, "new file mode 100644", "a merge against its first parent")
	require.Contains(t, d.Text, "+feature\n")

	d, err = Diff(ctx(), h.dir, h.feat1, "a.txt")
	require.NoError(t, err)
	require.Empty(t, d.Text, "a file the commit left alone; it may be gone from disk too")

	// Uncommitted.
	h.write("b.txt", "a 1\nchanged\n")
	d, err = Diff(ctx(), h.dir, "", "b.txt")
	require.NoError(t, err)
	require.Contains(t, d.Text, "--- a/b.txt\n+++ b/b.txt\n")
	require.Contains(t, d.Text, "-a 2\n")
	require.Contains(t, d.Text, "+changed\n")

	h.write("new.txt", "one\ntwo")
	d, err = Diff(ctx(), h.dir, "", "new.txt")
	require.NoError(t, err)
	require.Equal(t, "diff --git a/new.txt b/new.txt\nnew file mode 100644\n--- /dev/null\n+++ b/new.txt\n@@ -0,0 +1,2 @@\n+one\n+two\n\\ No newline at end of file\n", d.Text)

	h.write("one.txt", "only\n")
	d, err = Diff(ctx(), h.dir, "", "one.txt")
	require.NoError(t, err)
	require.Contains(t, d.Text, "@@ -0,0 +1 @@\n+only\n")

	h.write("new.bin", "\x00zz")
	d, err = Diff(ctx(), h.dir, "", "new.bin")
	require.NoError(t, err)
	require.True(t, d.Binary)

	big := strings.Repeat("a long line of text for the diff\n", MaxDiffBytes/30)
	h.write("big.txt", big)
	d, err = Diff(ctx(), h.dir, "", "big.txt")
	require.NoError(t, err)
	require.True(t, d.Truncated)
	require.LessOrEqual(t, len(d.Text), MaxDiffBytes)
	h.git("add", "big.txt")
	h.commit("big")
	head := strings.TrimSpace(h.git("rev-parse", "HEAD"))
	d, err = Diff(ctx(), h.dir, head, "big.txt")
	require.NoError(t, err)
	require.True(t, d.Truncated, "capped while git writes")
	require.LessOrEqual(t, len(d.Text), MaxDiffBytes)
	require.True(t, strings.HasSuffix(d.Text, "\n"))

	h.git("rm", "-q", "feature.txt")
	d, err = Diff(ctx(), h.dir, "", "feature.txt")
	require.NoError(t, err)
	require.Contains(t, d.Text, "deleted file mode", "a deleted file's path still resolves")

	_, err = Diff(ctx(), h.dir, "", "../x")
	require.ErrorIs(t, err, ErrOutside)
	_, err = Diff(ctx(), h.dir, "-p", "b.txt")
	require.ErrorIs(t, err, ErrBadRev)
}

func TestParseRefs(t *testing.T) {
	t.Parallel()
	require.Equal(t, []Ref{{"HEAD", "head"}}, parseRefs("HEAD"))
	require.Equal(t, []Ref{
		{"HEAD", "head"}, {"w27", "branch"}, {"origin/w27", "remote"}, {"v1.0", "tag"},
	}, parseRefs("HEAD -> refs/heads/w27, refs/remotes/origin/w27, refs/remotes/origin/HEAD, tag: refs/tags/v1.0, refs/stash"))
	require.Equal(t, []Ref{}, parseRefs(""))
}
