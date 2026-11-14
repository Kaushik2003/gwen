// Package repo reads a project folder for the Gwen window's workbench
// (plan.md 3.15): its files with their git status, a file's text, gutter
// marks and blame, the history, a commit's files and diffs, and it commits a
// chosen set of files. Every function takes the folder as an absolute path,
// runs git there with machine-readable output, and keeps nothing between
// calls. Paths in and out are relative to the folder, slash-separated, and
// must stay inside it.
package repo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	// ErrOutside is a path that leaves the folder, directly or through a symlink.
	ErrOutside = errors.New("path is outside the folder")
	// ErrNotRepo is a folder git does not know, for the calls that need git.
	ErrNotRepo = errors.New("not a git repository")
	// ErrBadRev is a commit id that is not hex, or names no commit.
	ErrBadRev = errors.New("not a commit id")
	// ErrNotFile is a path that is not a regular file, such as a folder.
	ErrNotFile = errors.New("not a regular file")
	// ErrInvalid is a request that cannot be carried out as asked.
	ErrInvalid = errors.New("invalid request")
)

const (
	// MaxTreeFiles caps ListFiles; the rest are left out and Tree.Truncated is set.
	MaxTreeFiles = 20000
	// MaxFileBytes caps ReadFile and the line counts of untracked files.
	MaxFileBytes = 1 << 20
	// MaxDiffBytes caps one file's diff.
	MaxDiffBytes = 512 << 10
	// DefaultLogLimit is History's page size when LogQuery.Limit is 0.
	DefaultLogLimit = 200
	// MaxLogLimit is the largest page History returns.
	MaxLogLimit = 1000

	readTimeout   = 30 * time.Second
	commitTimeout = 2 * time.Minute // commit hooks may run tests
)

// baseArgs make git's output the same whatever the user's configuration:
// raw path names, no colour, a/ and b/ prefixes, no signatures, and no
// rename following that would fight --topo-order. Each diff also passes
// --no-ext-diff and --no-textconv.
var baseArgs = []string{
	"-c", "core.quotepath=off",
	"-c", "color.ui=false",
	"-c", "diff.noprefix=false",
	"-c", "diff.mnemonicPrefix=false",
	"-c", "log.showSignature=false",
	"-c", "log.follow=false",
}

// envDropped are variables that would point git at another repository or
// change how it reads pathspecs.
var envDropped = map[string]bool{
	"GIT_DIR": true, "GIT_WORK_TREE": true, "GIT_INDEX_FILE": true, "GIT_COMMON_DIR": true,
	"GIT_PREFIX": true, "GIT_NAMESPACE": true, "GIT_OBJECT_DIRECTORY": true,
	"GIT_ALTERNATE_OBJECT_DIRECTORIES": true, "GIT_GLOB_PATHSPECS": true, "GIT_NOGLOB_PATHSPECS": true,
	"GIT_ICASE_PATHSPECS": true, "GIT_LITERAL_PATHSPECS": true, "GIT_OPTIONAL_LOCKS": true,
	"GIT_EXTERNAL_DIFF": true, "GIT_PAGER": true, "LC_ALL": true,
}

func gitEnv() []string {
	var env []string
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !envDropped[k] {
			env = append(env, kv)
		}
	}
	// Literal pathspecs: a file named ":(glob)*" is that file, nothing more.
	return append(env, "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GIT_LITERAL_PATHSPECS=1", "GIT_PAGER=cat")
}

// folder is an open project folder.
type folder struct {
	dir  string // as given, cleaned
	root string // with symlinks resolved; git runs here
}

func open(dir string) (folder, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return folder{}, fmt.Errorf("%w: the folder must be an absolute path, not %q", ErrInvalid, dir)
	}
	clean := filepath.Clean(dir)
	root, err := filepath.EvalSymlinks(clean)
	if err != nil {
		return folder{}, fmt.Errorf("folder %s: %w", clean, err)
	}
	st, err := os.Stat(root)
	if err != nil {
		return folder{}, fmt.Errorf("folder %s: %w", clean, err)
	}
	if !st.IsDir() {
		return folder{}, fmt.Errorf("%w: %s is not a folder", ErrInvalid, clean)
	}
	return folder{dir: clean, root: root}, nil
}

// Resolve returns the absolute path of path inside dir with symlinks
// resolved, for opening it with another program. The file must exist.
func Resolve(dir, path string) (string, error) {
	f, err := open(dir)
	if err != nil {
		return "", err
	}
	_, real, err := f.resolve(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(real); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return real, nil
}

// resolve checks that path, relative to the folder or absolute, stays inside
// it once symlinks are resolved. It returns the path relative to the folder
// as git names it (slash-separated, symlinks kept) and the resolved absolute
// path. A path that does not exist, such as a deleted file, is checked
// through its nearest existing parent.
func (f folder) resolve(path string) (rel, real string, err error) {
	if path == "" || strings.ContainsRune(path, 0) {
		return "", "", fmt.Errorf("%w: empty or malformed path", ErrInvalid)
	}
	lex := filepath.FromSlash(path)
	if filepath.IsAbs(lex) {
		r, ok := within(f.dir, lex)
		if !ok {
			r, ok = within(f.root, lex)
		}
		if !ok {
			return "", "", fmt.Errorf("%w: %s", ErrOutside, path)
		}
		lex = r
	}
	lex = filepath.Clean(lex)
	if lex == ".." || strings.HasPrefix(lex, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("%w: %s", ErrOutside, path)
	}
	real, err = evalExisting(filepath.Join(f.root, lex))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", path, err)
	}
	if _, ok := within(f.root, real); !ok {
		return "", "", fmt.Errorf("%w: %s", ErrOutside, path)
	}
	return filepath.ToSlash(lex), real, nil
}

// within reports whether p is root or below it, lexically, and returns p
// relative to root.
func within(root, p string) (string, bool) {
	r, err := filepath.Rel(root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return "", false
	}
	return r, true
}

// evalExisting resolves symlinks in p, or in its nearest existing parent
// when p does not exist, keeping the missing tail as written.
func evalExisting(p string) (string, error) {
	var tail []string
	cur := p
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(append([]string{real}, tail...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		tail = append([]string{filepath.Base(cur)}, tail...)
		cur = parent
	}
}

// git runs git in the folder and returns its standard output.
func (f folder) git(ctx context.Context, args ...string) ([]byte, error) {
	out, _, err := f.run(ctx, readTimeout, 0, args...)
	return out, err
}

// run runs git with a timeout. With max > 0 it keeps at most max bytes of
// output plus one, and stops git once it has them; capped reports that.
func (f folder) run(ctx context.Context, timeout time.Duration, max int, args ...string) (out []byte, capped bool, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append(append([]string{}, baseArgs...), args...)...)
	cmd.Dir = f.root
	cmd.Env = gitEnv()
	var stderr bytes.Buffer
	stdout := &capWriter{max: max, cancel: cancel}
	cmd.Stdout = stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if stdout.full {
		return stdout.buf.Bytes(), true, nil
	}
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			msg = "took too long"
		}
		return stdout.buf.Bytes(), false, fmt.Errorf("git %s: %w: %s", args[0], err, msg)
	}
	return stdout.buf.Bytes(), false, nil
}

// capWriter collects output up to max+1 bytes (unbounded when max is 0) and
// then cancels the command instead of reading the rest.
type capWriter struct {
	buf    bytes.Buffer
	max    int
	full   bool
	cancel context.CancelFunc
}

func (w *capWriter) Write(p []byte) (int, error) {
	if w.max <= 0 {
		return w.buf.Write(p)
	}
	if room := w.max + 1 - w.buf.Len(); room > 0 {
		w.buf.Write(p[:min(room, len(p))])
	}
	if w.buf.Len() > w.max && !w.full {
		w.full = true
		w.cancel()
	}
	return len(p), nil
}

// exitCode is git's exit status, or -1 when it did not run to an exit.
func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	return -1
}

// prefix is the folder's place inside its repository ("" at the top,
// "sub/dir/" below it), or ErrNotRepo.
func (f folder) prefix(ctx context.Context) (string, error) {
	out, err := f.git(ctx, "rev-parse", "--show-prefix")
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("%w: %s: %w", ErrNotRepo, f.dir, err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

// head is HEAD's commit id; ok is false on a branch with no commits yet.
func (f folder) head(ctx context.Context) (hash string, ok bool, err error) {
	out, err := f.git(ctx, "rev-parse", "--verify", "-q", "HEAD^{commit}")
	if err != nil {
		if exitCode(err) == 1 {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

// base is what uncommitted work is compared with: HEAD, or the empty tree
// before the first commit.
func (f folder) base(ctx context.Context) (rev string, head string, err error) {
	head, ok, err := f.head(ctx)
	if err != nil {
		return "", "", err
	}
	if ok {
		return head, head, nil
	}
	out, err := f.git(ctx, "hash-object", "-t", "tree", os.DevNull)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(out)), "", nil
}

// commitID checks that hash is hex and names a commit, and returns its full id.
func (f folder) commitID(ctx context.Context, hash string) (string, error) {
	if len(hash) < 4 || len(hash) > 64 || strings.Trim(hash, "0123456789abcdefABCDEF") != "" {
		return "", fmt.Errorf("%w: %q", ErrBadRev, hash)
	}
	out, err := f.git(ctx, "rev-parse", "--verify", "-q", hash+"^{commit}")
	if err != nil {
		if exitCode(err) == 1 || exitCode(err) == 128 {
			return "", fmt.Errorf("%w: %s", ErrBadRev, hash)
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// scope is the pathspec that keeps git inside the folder when the folder is
// below the top of its repository.
func scope(prefix string) []string {
	if prefix == "" {
		return nil
	}
	return []string{"."}
}

// splitZ splits NUL-terminated output, dropping the empty tail.
func splitZ(out []byte) []string {
	s := string(out)
	s = strings.TrimSuffix(s, "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

// isBinary is git's test: a NUL in the first 8000 bytes.
func isBinary(b []byte) bool {
	return bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0
}

// cutAtLine trims b to at most max bytes, ending at a line break when there is one.
func cutAtLine(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	b = b[:max]
	if i := bytes.LastIndexByte(b, '\n'); i >= 0 {
		return b[:i+1]
	}
	return b
}

// countLines counts text lines; a last line without a newline counts.
func countLines(b []byte) int {
	n := bytes.Count(b, []byte{'\n'})
	if len(b) > 0 && b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// readHead reads at most max bytes of a file, plus one to tell whether there is more.
func readHead(path string, max int) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return io.ReadAll(io.LimitReader(fh, int64(max)+1))
}
