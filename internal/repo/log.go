package repo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// LogQuery picks a page of history. Search, Author, and Path narrow it.
type LogQuery struct {
	Limit  int    `json:"limit"`  // 0: DefaultLogLimit; at most MaxLogLimit
	Skip   int    `json:"skip"`   // commits to skip, for the next page
	Search string `json:"search"` // in commit messages, ignoring case
	Author string `json:"author"` // in author names and addresses, ignoring case
	Path   string `json:"path"`   // a file or folder of the folder
}

// Log is a page of history, newest first, children before their parents.
type Log struct {
	Commits []Commit `json:"commits"`
	More    bool     `json:"more"` // another page follows
	// Uncommitted is the uncommitted work, on the first page of an
	// unfiltered (or Path-only) query; nil when there is none.
	Uncommitted *Uncommitted `json:"uncommitted"`
	// The checked-out branch and how it stands against its upstream, on the
	// first page. Branch is "" when HEAD is detached.
	Branch   string `json:"branch"`
	Upstream string `json:"upstream"`
	Ahead    int    `json:"ahead"`
	Behind   int    `json:"behind"`
}

// Commit is one commit of the history.
type Commit struct {
	Hash        string   `json:"hash"`
	Short       string   `json:"short"`
	Parents     []string `json:"parents"` // full ids; two or more for a merge
	Subject     string   `json:"subject"`
	Author      string   `json:"author"`
	AuthorEmail string   `json:"author_email"`
	When        int64    `json:"when"` // author time, unix ms
	Refs        []Ref    `json:"refs"`
	// Line and file counts against the first parent; 0 for merges in History.
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Files   int `json:"files"`
}

// Ref is a name pointing at a commit. A checked-out branch is a "head" Ref
// named HEAD followed by its "branch" Ref; a detached HEAD is the head alone.
type Ref struct {
	Name string `json:"name"` // main, origin/main, v1.0, HEAD
	Kind string `json:"kind"` // head, branch, remote, or tag
}

// Uncommitted sums up the uncommitted work.
type Uncommitted struct {
	Files   int `json:"files"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// CommitDetail is one commit with its message body and changed files
// (Changes, as Commit.Files already counts them). For the uncommitted work
// (Show with hash "") the commit fields are empty except Parents (HEAD),
// Refs (HEAD and its branch), and the counts.
type CommitDetail struct {
	Commit
	Body    string       `json:"body"`
	Changes []FileChange `json:"changes"`
}

// DiffText is one file's unified diff.
type DiffText struct {
	Path      string `json:"path"`
	Text      string `json:"text"` // git's unified diff, headers included
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"` // over MaxDiffBytes: Text is the first lines that fit
}

// commitFormat is a commit's fields for git log -z; %x01 marks a new commit.
const commitFormat = "%x01%H%x00%h%x00%P%x00%an%x00%ae%x00%at%x00%D%x00%s"

// History lists commits of every local branch, remote branch, and tag, and
// HEAD, in topological order, with their refs and line counts.
func History(ctx context.Context, dir string, q LogQuery) (Log, error) {
	f, err := open(dir)
	if err != nil {
		return Log{}, err
	}
	prefix, err := f.prefix(ctx)
	if err != nil {
		return Log{}, err
	}
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLogLimit
	}
	limit = min(limit, MaxLogLimit)
	skip := max(q.Skip, 0)
	var paths []string
	if q.Path != "" {
		rel, _, err := f.resolve(q.Path)
		if err != nil {
			return Log{}, err
		}
		paths = []string{rel}
	}
	_, hasHead, err := f.head(ctx)
	if err != nil {
		return Log{}, err
	}
	// --parents rewrites %P to the commits a path filter keeps, so the graph joins up.
	args := []string{"log", "-z", "--topo-order", "--decorate=full", "--parents", "--format=" + commitFormat,
		"--numstat", "-M", "--relative", "--no-ext-diff", "--no-textconv",
		"--max-count=" + strconv.Itoa(limit+1), "--skip=" + strconv.Itoa(skip)}
	if q.Search != "" || q.Author != "" {
		args = append(args, "--fixed-strings", "--regexp-ignore-case")
	}
	if q.Search != "" {
		args = append(args, "--grep="+q.Search)
	}
	if q.Author != "" {
		args = append(args, "--author="+q.Author)
	}
	args = append(args, "--branches", "--remotes", "--tags")
	if hasHead {
		args = append(args, "HEAD")
	}
	args = append(args, "--")
	if paths != nil {
		args = append(args, paths...)
	} else {
		args = append(args, scope(prefix)...)
	}
	out, err := f.git(ctx, args...)
	if err != nil {
		return Log{}, err
	}
	l := Log{Commits: parseLog(out)}
	if len(l.Commits) > limit {
		l.Commits, l.More = l.Commits[:limit], true
	}
	if skip > 0 {
		return l, nil
	}
	base, _, err := f.base(ctx)
	if err != nil {
		return Log{}, err
	}
	st, files, err := f.changes(ctx, prefix, base, paths...)
	if err != nil {
		return Log{}, err
	}
	l.Branch, l.Upstream, l.Ahead, l.Behind = st.head, st.upstream, st.ahead, st.behind
	if len(files) > 0 && q.Search == "" && q.Author == "" {
		u := &Uncommitted{Files: len(files)}
		for _, fc := range files {
			u.Added += fc.Added
			u.Removed += fc.Removed
		}
		l.Uncommitted = u
	}
	return l, nil
}

// parseLog reads git log -z output made with commitFormat and --numstat.
func parseLog(out []byte) []Commit {
	commits := []Commit{}
	toks := splitZ(out)
	for i := 0; i < len(toks); {
		t := toks[i]
		if !strings.HasPrefix(t, "\x01") || i+8 > len(toks) {
			i++
			continue
		}
		at, _ := strconv.ParseInt(toks[i+5], 10, 64)
		c := Commit{
			Hash:        t[1:],
			Short:       toks[i+1],
			Parents:     strings.Fields(toks[i+2]),
			Author:      toks[i+3],
			AuthorEmail: toks[i+4],
			When:        at * 1000,
			Refs:        parseRefs(toks[i+6]),
			Subject:     toks[i+7],
		}
		if c.Parents == nil {
			c.Parents = []string{}
		}
		i += 8
		for i < len(toks) && !strings.HasPrefix(toks[i], "\x01") {
			f := strings.SplitN(strings.TrimPrefix(toks[i], "\n"), "\t", 3)
			i++
			if len(f) < 3 {
				continue
			}
			if f[2] == "" { // a rename: old and new names follow
				i += 2
			}
			c.Files++
			a, _ := strconv.Atoi(f[0])
			r, _ := strconv.Atoi(f[1])
			c.Added += a
			c.Removed += r
		}
		commits = append(commits, c)
	}
	return commits
}

// parseRefs reads %D made with --decorate=full.
func parseRefs(d string) []Ref {
	refs := []Ref{}
	if d == "" {
		return refs
	}
	for _, item := range strings.Split(d, ", ") {
		if branch, ok := strings.CutPrefix(item, "HEAD -> "); ok {
			refs = append(refs, Ref{Name: "HEAD", Kind: "head"})
			item = branch
		}
		switch {
		case item == "HEAD":
			refs = append(refs, Ref{Name: "HEAD", Kind: "head"})
		case strings.HasPrefix(item, "tag: refs/tags/"):
			refs = append(refs, Ref{Name: strings.TrimPrefix(item, "tag: refs/tags/"), Kind: "tag"})
		case strings.HasPrefix(item, "refs/heads/"):
			refs = append(refs, Ref{Name: strings.TrimPrefix(item, "refs/heads/"), Kind: "branch"})
		case strings.HasPrefix(item, "refs/remotes/"):
			name := strings.TrimPrefix(item, "refs/remotes/")
			if !strings.HasSuffix(name, "/HEAD") { // origin/HEAD only repeats the default branch
				refs = append(refs, Ref{Name: name, Kind: "remote"})
			}
		}
	}
	return refs
}

// Show reads one commit's message and files, with each file's line counts
// against the first parent. hash "" is the uncommitted work.
func Show(ctx context.Context, dir, hash string) (CommitDetail, error) {
	f, err := open(dir)
	if err != nil {
		return CommitDetail{}, err
	}
	prefix, err := f.prefix(ctx)
	if err != nil {
		return CommitDetail{}, err
	}
	var d CommitDetail
	if hash == "" {
		base, head, err := f.base(ctx)
		if err != nil {
			return CommitDetail{}, err
		}
		st, files, err := f.changes(ctx, prefix, base)
		if err != nil {
			return CommitDetail{}, err
		}
		d.Parents, d.Refs = []string{}, []Ref{{Name: "HEAD", Kind: "head"}}
		if head != "" {
			d.Parents = []string{head}
		}
		if st.head != "" {
			d.Refs = append(d.Refs, Ref{Name: st.head, Kind: "branch"})
		}
		d.Changes = files
	} else {
		id, err := f.commitID(ctx, hash)
		if err != nil {
			return CommitDetail{}, err
		}
		out, err := f.git(ctx, "show", "-s", "-z", "--decorate=full", "--format="+commitFormat+"%x00%b", id)
		if err != nil {
			return CommitDetail{}, err
		}
		toks := splitZ(out)
		commits := parseLog([]byte(strings.Join(toks[:min(8, len(toks))], "\x00")))
		if len(commits) != 1 || len(toks) < 9 {
			return CommitDetail{}, fmt.Errorf("git show %s: unexpected output", hash)
		}
		d.Commit = commits[0]
		d.Body = strings.TrimRight(toks[8], "\n ")
		out, err = f.git(ctx, append([]string{"show", "--format=", "--raw", "--numstat", "-z", "-M", "--diff-merges=first-parent",
			"--relative", "--no-ext-diff", "--no-textconv", id, "--"}, scope(prefix)...)...)
		if err != nil {
			return CommitDetail{}, err
		}
		d.Changes = parseRawNumstat(out)
	}
	if d.Changes == nil {
		d.Changes = []FileChange{}
	}
	d.Added, d.Removed, d.Files = 0, 0, len(d.Changes)
	for _, fc := range d.Changes {
		d.Added += fc.Added
		d.Removed += fc.Removed
	}
	return d, nil
}

// Diff is one file's unified diff in a commit (against its first parent),
// or in the uncommitted work when hash is "". A new untracked file is shown
// as all added.
func Diff(ctx context.Context, dir, hash, file string) (DiffText, error) {
	f, err := open(dir)
	if err != nil {
		return DiffText{}, err
	}
	rel, real, err := f.resolve(file)
	if err != nil {
		return DiffText{}, err
	}
	prefix, err := f.prefix(ctx)
	if err != nil {
		return DiffText{}, err
	}
	d := DiffText{Path: rel}
	opts := []string{"-M", "-U3", "--no-ext-diff", "--no-textconv", "--no-color", "--relative"}
	var args []string
	if hash == "" {
		base, _, err := f.base(ctx)
		if err != nil {
			return DiffText{}, err
		}
		st, err := f.status(ctx, prefix, rel)
		if err != nil {
			return DiffText{}, err
		}
		paths := []string{rel}
		if len(st.entries) > 0 {
			switch e := st.entries[0]; {
			case e.kind == '?':
				return newFileDiff(rel, real)
			case e.orig != "":
				paths = []string{e.orig, rel}
			}
		}
		args = append(append(append([]string{"diff"}, opts...), base, "--"), paths...)
	} else {
		id, err := f.commitID(ctx, hash)
		if err != nil {
			return DiffText{}, err
		}
		paths, err := f.withOldName(ctx, id, rel)
		if err != nil {
			return DiffText{}, err
		}
		args = append(append(append([]string{"show", "--format=", "--diff-merges=first-parent"}, opts...), id, "--"), paths...)
	}
	out, capped, err := f.run(ctx, readTimeout, MaxDiffBytes, args...)
	if err != nil {
		return DiffText{}, err
	}
	if capped || len(out) > MaxDiffBytes {
		d.Truncated = true
		out = cutAtLine(out, MaxDiffBytes)
	}
	d.Text = strings.ToValidUTF8(string(out), "�")
	d.Binary = isBinaryDiff(d.Text)
	return d, nil
}

// isBinaryDiff reports git's "Binary files … differ" line in a diff's header.
func isBinaryDiff(text string) bool {
	for _, l := range strings.SplitN(text, "\n", 8) {
		if strings.HasPrefix(l, "Binary files ") || strings.HasPrefix(l, "@@") {
			return strings.HasPrefix(l, "Binary files ")
		}
	}
	return false
}

// newFileDiff makes the diff git would show for an untracked file once added.
func newFileDiff(rel, real string) (DiffText, error) {
	d := DiffText{Path: rel}
	b, err := readHead(real, MaxDiffBytes)
	if err != nil {
		return DiffText{}, fmt.Errorf("%s: %w", rel, err)
	}
	head := fmt.Sprintf("diff --git a/%s b/%s\nnew file mode 100644\n", rel, rel)
	if isBinary(b) {
		d.Binary = true
		d.Text = head + fmt.Sprintf("Binary files /dev/null and b/%s differ\n", rel)
		return d, nil
	}
	text := strings.ToValidUTF8(string(b), "\uFFFD")
	if text == "" {
		d.Text = head
		return d, nil
	}
	head += fmt.Sprintf("--- /dev/null\n+++ b/%s\n", rel)
	// Take the lines that fit, leaving room for the hunk header.
	room := MaxDiffBytes - len(head) - 64
	all := strings.SplitAfter(text, "\n")
	if all[len(all)-1] == "" {
		all = all[:len(all)-1]
	}
	var body strings.Builder
	n := 0
	for _, l := range all {
		if body.Len()+len(l)+2 > room {
			d.Truncated = true
			break
		}
		body.WriteString("+")
		body.WriteString(l)
		n++
	}
	if d.Truncated || len(b) > MaxDiffBytes {
		d.Truncated = true
	} else if !strings.HasSuffix(text, "\n") {
		body.WriteString("\n\\ No newline at end of file\n")
	}
	hunk := "@@ -0,0 +1 @@\n"
	if n != 1 {
		hunk = fmt.Sprintf("@@ -0,0 +1,%d @@\n", n)
	}
	d.Text = head + hunk + body.String()
	return d, nil
}
