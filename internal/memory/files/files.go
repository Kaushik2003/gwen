// Package files keeps the memory folder: a readable copy, in Markdown, of the
// claims Gwen holds, which the user can edit to correct her (harness 4.5).
//
// Project writes the folder from the claims; Scan reads back what the user
// changed since. The service scans before it projects, so an edit is never
// overwritten before it is read. Each line names its claim with a mark at its
// end, `<!-- c:<claim id> -->`, which is how a changed line is matched to the
// claim it was.
//
// The last projection is kept twice: in <Dir>/.gwen/projection.json, which
// Scan compares against, and, when git is on PATH, as a commit in the
// folder's own repository, which gives the user a history to look back on.
// Scan falls back to the last commit only when the snapshot is missing,
// because a commit the user makes by hand would otherwise hide their edits.
package files

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/memory"
	"github.com/kzark/gwen/internal/model"
)

// The files of the folder, relative to it.
const (
	ReadmeFile  = "README.md"
	ProfileFile = "profile.md"
	OtherFile   = "other.md"
	PeopleDir   = "people"
)

// snapshotFile is the last projection, relative to the folder.
const snapshotFile = ".gwen/projection.json"

// maxFile is the largest file Scan reads; a bigger one is ignored.
const maxFile = 1 << 20

// gitTimeout bounds each git command.
const gitTimeout = 10 * time.Second

// Change kinds.
const (
	ChangeEdit   = "edit"   // the user changed a line's words
	ChangeRemove = "remove" // the user deleted a line
	ChangeAdd    = "add"    // the user wrote a new line
)

// Change is something the user changed in the folder since the last
// projection.
type Change struct {
	Kind string // ChangeEdit, ChangeRemove, or ChangeAdd
	// ClaimID is the claim the line was; "" for an added line.
	ClaimID string
	// Text is the line's words now, or for a removal the words it had.
	Text string
	// Subject is the person a people file is about, or the name of the
	// section of other.md the line sits under; "" for lines about the user.
	Subject string
	// Section is the "## " heading the line sits under, such as "Works at";
	// "" when none.
	Section string
	// Path is the file, relative to the folder, such as "people/anna.md".
	Path string
}

// Options configure a Folder.
type Options struct {
	Dir   string // the folder, absolute
	Clock clock.Clock
	Loc   *time.Location // the zone of commit dates
}

// Folder is the memory folder.
type Folder struct {
	dir   string
	clock clock.Clock
	loc   *time.Location
	git   string // the git binary; "" when git is not on PATH

	mu sync.Mutex // one projection or scan at a time
}

// New returns the folder at o.Dir. Nothing is written until Project.
func New(o Options) *Folder {
	f := &Folder{dir: o.Dir, clock: o.Clock, loc: o.Loc}
	if f.clock == nil {
		f.clock = clock.Real()
	}
	if f.loc == nil {
		f.loc = time.Local
	}
	if git, err := exec.LookPath("git"); err == nil {
		f.git = git
	}
	return f
}

// Dir is the folder.
func (f *Folder) Dir() string { return f.dir }

// Project writes the folder from claims: README.md, profile.md (claims about
// the user), people/<name>.md, and other.md (organisations, places, and
// topics). Only active claims that are neither suppressed nor sensitive are
// written. A file is replaced only when its content changed, and people files
// no claim needs any more are removed. When git is on PATH a projection that
// changed anything is committed; a git failure is logged, not returned, since
// the files themselves were written.
func (f *Folder) Project(ctx context.Context, claims []memory.ClaimView, entities []model.Entity) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dir == "" {
		return errors.New("memory files: no folder is set")
	}
	for _, d := range []string{f.dir, filepath.Join(f.dir, PeopleDir), filepath.Join(f.dir, filepath.Dir(snapshotFile))} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return fmt.Errorf("memory files: %w", err)
		}
		if err := os.Chmod(d, 0o700); err != nil {
			return fmt.Errorf("memory files: %w", err)
		}
	}
	files := render(claims, entities)
	all := map[string]string{ReadmeFile: readme, ".gitignore": ".gwen/\n"}
	maps.Copy(all, files)
	for _, p := range slices.Sorted(maps.Keys(all)) {
		if err := writeIfChanged(filepath.Join(f.dir, filepath.FromSlash(p)), all[p]); err != nil {
			return fmt.Errorf("memory files: %w", err)
		}
	}
	if err := f.removeStale(files); err != nil {
		return fmt.Errorf("memory files: %w", err)
	}
	snap, err := json.Marshal(snapshot{Version: 1, Files: files})
	if err != nil {
		return err
	}
	if err := writeIfChanged(filepath.Join(f.dir, snapshotFile), string(snap)); err != nil {
		return fmt.Errorf("memory files: %w", err)
	}
	if err := f.commit(ctx); err != nil {
		slog.Warn("memory files: git commit failed", "dir", f.dir, "err", err)
	}
	return nil
}

// removeStale deletes the people files that are not in files.
func (f *Folder) removeStale(files map[string]string) error {
	entries, err := os.ReadDir(filepath.Join(f.dir, PeopleDir))
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := path.Join(PeopleDir, e.Name())
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if _, ok := files[p]; ok {
			continue
		}
		if err := os.Remove(filepath.Join(f.dir, filepath.FromSlash(p))); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// snapshot is the last projection: each projected file's content by path.
type snapshot struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`
}

// readme is README.md.
const readme = `# Gwen's memory

Gwen keeps a readable copy of what she remembers here. profile.md is about you, the files in people/ are about
the people in your life, and other.md is about places, organisations, and topics. She rewrites these files
when her memory changes.

To correct something, edit its line. To stop her bringing something up, delete its line. To tell her something
new, add a line that starts with "- ". The <!-- c:... --> marks at the end of the lines are how she knows which
line is which, so keep them as they are.
`

// render makes the projected files, README.md aside, by path.
func render(claims []memory.ClaimView, entities []model.Entity) map[string]string {
	byID := make(map[string]model.Entity, len(entities))
	for _, e := range entities {
		byID[e.ID] = e
	}
	var mine []memory.ClaimView
	about := map[string]*group{} // entity id -> its claims
	for _, c := range claims {
		if !projected(c) {
			continue
		}
		if c.SubjectID == nil {
			mine = append(mine, c)
			continue
		}
		g := about[*c.SubjectID]
		if g == nil {
			e := byID[*c.SubjectID]
			g = &group{id: *c.SubjectID, name: e.Name, kind: e.Kind}
			if g.name == "" {
				g.name = c.Subject
			}
			if g.kind == "" {
				g.kind = memory.EntityKindOf(c.Key)
			}
			g.name = oneLine(g.name)
			if g.name == "" {
				g.name = "Someone"
			}
			about[*c.SubjectID] = g
		}
		g.claims = append(g.claims, c)
	}

	files := map[string]string{ProfileFile: renderProfile(mine)}
	var people, other []*group
	for _, g := range about {
		sortClaims(g.claims)
		if g.kind == memory.EntityPerson {
			people = append(people, g)
		} else {
			other = append(other, g)
		}
	}
	slices.SortFunc(people, func(a, b *group) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), cmp.Compare(a.id, b.id))
	})
	used := map[string]bool{}
	for _, g := range people {
		base := slug(g.name)
		name := base
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		used[name] = true
		var b strings.Builder
		fmt.Fprintf(&b, "# %s\n\n", g.name)
		for _, c := range g.claims {
			b.WriteString(line(c))
		}
		files[path.Join(PeopleDir, name+".md")] = b.String()
	}

	slices.SortFunc(other, func(a, b *group) int {
		return cmp.Or(cmp.Compare(kindRank(a.kind), kindRank(b.kind)),
			cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name)), cmp.Compare(a.id, b.id))
	})
	var b strings.Builder
	b.WriteString("# Places, organisations, and topics\n")
	for _, g := range other {
		fmt.Fprintf(&b, "\n## %s\n\n", g.name)
		for _, c := range g.claims {
			b.WriteString(line(c))
		}
	}
	files[OtherFile] = b.String()
	return files
}

// group is the claims about one entity.
type group struct {
	id, name, kind string
	claims         []memory.ClaimView
}

// renderProfile is profile.md: the user's claims under a heading per key, in
// the vocabulary's order, then other keys in order.
func renderProfile(claims []memory.ClaimView) string {
	byKey := map[string][]memory.ClaimView{}
	for _, c := range claims {
		byKey[c.Key] = append(byKey[c.Key], c)
	}
	keys := slices.SortedFunc(maps.Keys(byKey), func(a, b string) int {
		return cmp.Or(cmp.Compare(keyRank(a), keyRank(b)), cmp.Compare(a, b))
	})
	var b strings.Builder
	b.WriteString("# About you\n")
	for _, k := range keys {
		cs := byKey[k]
		sortClaims(cs)
		fmt.Fprintf(&b, "\n## %s\n\n", label(cs[0]))
		for _, c := range cs {
			b.WriteString(line(c))
		}
	}
	return b.String()
}

// projected reports whether a claim belongs in the folder: active, live,
// neither suppressed nor sensitive, and with words to show.
func projected(c memory.ClaimView) bool {
	if c.Status != model.ClaimActive || c.Suppressed || c.DeletedAt != nil {
		return false
	}
	if c.Sensitivity == model.SensitivitySensitive {
		return false
	}
	if s, ok := memory.Spec(c.Key); ok && s.Sensitive {
		return false
	}
	return oneLine(c.Text) != ""
}

// sortClaims orders claims by key, then oldest first, so a new claim adds a
// line at the end of its group.
func sortClaims(cs []memory.ClaimView) {
	slices.SortFunc(cs, func(a, b memory.ClaimView) int {
		return cmp.Or(cmp.Compare(keyRank(a.Key), keyRank(b.Key)), cmp.Compare(a.Key, b.Key),
			a.RecordedAt.Compare(b.RecordedAt), cmp.Compare(a.ID, b.ID))
	})
}

// keyRank is a key's place in the vocabulary; keys outside it come after.
func keyRank(key string) int {
	if i := slices.IndexFunc(memory.Keys, func(k memory.KeySpec) bool { return k.Key == key }); i >= 0 {
		return i
	}
	return len(memory.Keys)
}

// kindRank orders the sections of other.md.
func kindRank(kind string) int {
	if i := slices.Index([]string{memory.EntityOrg, memory.EntityPlace, memory.EntityTopic}, kind); i >= 0 {
		return i
	}
	return 3
}

// label is the heading of a claim's key.
func label(c memory.ClaimView) string {
	if c.Label != "" {
		return oneLine(c.Label)
	}
	if s, ok := memory.Spec(c.Key); ok {
		return s.Label
	}
	return c.Key
}

// line is a claim's line in a file.
func line(c memory.ClaimView) string {
	return "- " + oneLine(c.Text) + " <!-- c:" + c.ID + " -->\n"
}

// oneLine puts text on one line with single spaces, and breaks up anything
// that would open a comment, so a claim's words cannot hide a mark.
func oneLine(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	return strings.ReplaceAll(text, "<!--", "<!-")
}

// slug is a file name for a person: lower-case letters and digits joined by
// dashes.
func slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	s := b.String()
	if r := []rune(s); len(r) > 60 {
		s = strings.TrimRight(string(r[:60]), "-")
	}
	if s == "" {
		return "person"
	}
	return s
}

// writeIfChanged replaces the file at p with content (mode 0600),
// atomically, unless it already holds it.
func writeIfChanged(p, content string) (err error) {
	if old, err := os.ReadFile(p); err == nil && string(old) == content {
		return nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".gwen-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(tmp.Name())
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Scan returns what the user changed in the folder since the last
// projection: lines whose words changed (edit), lines deleted (remove), and
// new lines that start with "- " (add). Lines are matched by their claim
// mark, across files, so moving a line changes nothing. A file that is
// missing, too big, or not text is ignored with a warning rather than read as
// every line deleted; a line with a mark the last projection did not write is
// ignored. With no projection yet, only added lines count.
func (f *Folder) Scan(ctx context.Context) ([]Change, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dir == "" {
		return nil, errors.New("memory files: no folder is set")
	}
	base := f.baseline(ctx)
	paths, err := f.workingPaths()
	if err != nil {
		return nil, fmt.Errorf("memory files: %w", err)
	}
	for p := range base {
		if !slices.Contains(paths, p) {
			paths = append(paths, p)
		}
	}
	slices.Sort(paths)

	// What the last projection wrote, by claim id.
	type was struct {
		entry
		path, subject string
	}
	before := map[string]was{}
	var order []string
	for _, p := range slices.Sorted(maps.Keys(base)) {
		doc, err := parse(base[p])
		if err != nil {
			continue
		}
		for _, e := range doc.entries {
			if e.id == "" {
				continue
			}
			if _, dup := before[e.id]; !dup {
				before[e.id] = was{e, p, subjectOf(p, doc.title, e.section)}
				order = append(order, e.id)
			}
		}
	}

	var changes []Change
	skipped := map[string]bool{} // files not read, whose lines are not counted as deleted
	seen := map[string]string{}  // claim id -> its words in the folder now
	for _, p := range paths {
		doc, err := f.readWorking(p)
		if err != nil {
			if _, inBase := base[p]; inBase || !errors.Is(err, fs.ErrNotExist) {
				slog.Warn("memory files: file ignored", "file", p, "err", err)
			}
			skipped[p] = true
			continue
		}
		title := doc.title
		if title == "" {
			if old, err := parse(base[p]); err == nil {
				title = old.title
			}
		}
		for _, e := range doc.entries {
			c := Change{Text: e.text, Subject: subjectOf(p, title, e.section), Section: e.section, Path: p}
			if e.id == "" {
				if e.text != "" {
					c.Kind = ChangeAdd
					changes = append(changes, c)
				}
				continue
			}
			w, known := before[e.id]
			if !known {
				continue
			}
			if first, dup := seen[e.id]; dup {
				// A copied line with new words is something new to know.
				if e.text != "" && e.text != first {
					c.Kind = ChangeAdd
					changes = append(changes, c)
				}
				continue
			}
			seen[e.id] = e.text
			c.ClaimID = e.id
			switch {
			case e.text == "":
				c.Kind, c.Text = ChangeRemove, w.text
				changes = append(changes, c)
			case e.text != w.text:
				c.Kind = ChangeEdit
				changes = append(changes, c)
			}
		}
	}
	for _, id := range order {
		w := before[id]
		if _, ok := seen[id]; ok || skipped[w.path] {
			continue
		}
		changes = append(changes, Change{Kind: ChangeRemove, ClaimID: id, Text: w.text, Subject: w.subject,
			Section: w.section, Path: w.path})
	}
	return changes, nil
}

// baseline is the last projection's files by path: the snapshot, or without
// one the last commit; none when neither can be read.
func (f *Folder) baseline(ctx context.Context) map[string]string {
	b, err := os.ReadFile(filepath.Join(f.dir, snapshotFile))
	if err == nil {
		var s snapshot
		if err := json.Unmarshal(b, &s); err == nil && s.Files != nil {
			return s.Files
		}
		slog.Warn("memory files: snapshot unreadable", "dir", f.dir, "err", err)
	}
	if f.git == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(f.dir, ".git")); err != nil {
		return nil
	}
	out, err := f.run(ctx, "ls-tree", "-r", "-z", "--name-only", "HEAD")
	if err != nil {
		return nil
	}
	files := map[string]string{}
	for p := range strings.SplitSeq(strings.TrimRight(out, "\x00"), "\x00") {
		if !scanned(p) {
			continue
		}
		content, err := f.run(ctx, "show", "HEAD:"+p)
		if err != nil {
			slog.Warn("memory files: git show failed", "file", p, "err", err)
			continue
		}
		files[p] = content
	}
	return files
}

// workingPaths are the Markdown files in the folder and in people/ that Scan
// reads, README.md aside.
func (f *Folder) workingPaths() ([]string, error) {
	var paths []string
	for _, d := range []string{".", PeopleDir} {
		entries, err := os.ReadDir(filepath.Join(f.dir, d))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if p := path.Join(d, e.Name()); scanned(p) {
				paths = append(paths, p)
			}
		}
	}
	return paths, nil
}

// scanned reports whether Scan reads the file at p.
func scanned(p string) bool {
	dir, name := path.Split(p)
	if !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
		return false
	}
	return (dir == "" && name != ReadmeFile) || dir == PeopleDir+"/"
}

// readWorking reads and parses the file at p as it is now.
func (f *Folder) readWorking(p string) (doc, error) {
	full := filepath.Join(f.dir, filepath.FromSlash(p))
	info, err := os.Lstat(full)
	if err != nil {
		return doc{}, err
	}
	if !info.Mode().IsRegular() {
		return doc{}, errors.New("not a regular file")
	}
	if info.Size() > maxFile {
		return doc{}, fmt.Errorf("bigger than %d bytes", maxFile)
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return doc{}, err
	}
	return parse(string(b))
}

// doc is a parsed memory file.
type doc struct {
	title   string // the "# " heading
	entries []entry
}

// entry is a "- " line of a file.
type entry struct {
	id      string // its claim mark; "" for a line without one
	text    string // its words, on one line with single spaces
	section string // the "## " heading above it
}

// markPattern is a claim mark at the end of a line.
var markPattern = regexp.MustCompile(`<!--\s*c:\s*([A-Za-z0-9_-]+)\s*-->\s*$`)

// parse reads a memory file; it fails on anything that is not text.
func parse(content string) (doc, error) {
	if !utf8.ValidString(content) || strings.ContainsRune(content, 0) {
		return doc{}, errors.New("not UTF-8 text")
	}
	var d doc
	section := ""
	for raw := range strings.Lines(content) {
		l := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(l, "## "):
			section = strings.TrimSpace(l[3:])
			continue
		case strings.HasPrefix(l, "# "):
			if d.title == "" {
				d.title = strings.TrimSpace(l[2:])
			}
			continue
		case !strings.HasPrefix(l, "- ") && !strings.HasPrefix(l, "* ") && l != "-" && l != "*":
			continue
		}
		body := strings.TrimSpace(l[1:])
		e := entry{section: section}
		if m := markPattern.FindStringSubmatchIndex(body); m != nil {
			e.id = body[m[2]:m[3]]
			body = body[:m[0]]
		}
		e.text = strings.Join(strings.Fields(body), " ")
		d.entries = append(d.entries, e)
	}
	return d, nil
}

// subjectOf is who a line in the file at p is about: the person a people
// file names, the section of other.md, or "" for the user.
func subjectOf(p, title, section string) string {
	switch {
	case strings.HasPrefix(p, PeopleDir+"/"):
		if title != "" {
			return title
		}
		return strings.TrimSuffix(path.Base(p), ".md")
	case p == OtherFile:
		return section
	}
	return ""
}

// ForgetHistory drops the folder's git history, keeping the files as they
// are, so that words Gwen has forgotten do not live on in old commits. The
// service calls it after the projection that removed them. Without git there
// is no history and it does nothing.
func (f *Folder) ForgetHistory(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.git == "" || f.dir == "" {
		return nil
	}
	if err := os.RemoveAll(filepath.Join(f.dir, ".git")); err != nil {
		return fmt.Errorf("memory files: %w", err)
	}
	return f.commit(ctx)
}

// commit makes the folder a git repository if it is not one yet, and commits
// whatever changed since the last commit.
func (f *Folder) commit(ctx context.Context) error {
	if f.git == "" {
		return nil
	}
	if _, err := os.Stat(filepath.Join(f.dir, ".git")); errors.Is(err, fs.ErrNotExist) {
		if _, err := f.run(ctx, "init", "-q"); err != nil {
			return err
		}
	}
	status, err := f.run(ctx, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if _, err := f.run(ctx, "add", "-A"); err != nil {
		return err
	}
	_, err = f.run(ctx, "commit", "-q", "-m", "Gwen: update memory")
	return err
}

// run runs git in the folder with Gwen's identity, apart from the user's git
// configuration and environment, and returns what it printed.
func (f *Folder) run(ctx context.Context, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	full := append([]string{"-C", f.dir, "-c", "user.name=Gwen", "-c", "user.email=gwen@localhost",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main", "-c", "core.hooksPath=/dev/null"}, args...)
	cmd := exec.CommandContext(ctx, f.git, full...)
	cmd.Env = f.gitEnv()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, memory.Cut(stderr.String(), 200))
	}
	return string(out), nil
}

// gitEnv is the environment of git: the daemon's, without any GIT_ variable
// that could point it at another repository, and without the user's and the
// system's git configuration; commits are dated by the clock.
func (f *Folder) gitEnv() []string {
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, "GIT_") })
	now := f.clock.Now().In(f.loc)
	at := fmt.Sprintf("@%d %s", now.Unix(), now.Format("-0700"))
	return append(env, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_DATE="+at, "GIT_COMMITTER_DATE="+at)
}
