package remote

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// gitTimeout bounds every git call; GIT_OPTIONAL_LOCKS=0 keeps Gwen from
// taking the index lock while Claude's own git calls run (plan.md 3.14.9).
const gitTimeout = 10 * time.Second

// maxDiffLines is where a diff is cut.
const maxDiffLines = 2000

// emptyTree is git's empty tree, what a folder without commits is compared to.
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "core.quotepath=off"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	return cmd.Output()
}

// gitRoot is the top of dir's work tree, "" outside git.
func gitRoot(ctx context.Context, dir string) string {
	out, err := git(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitBranch is dir's branch, a short commit when detached, "" outside git.
func gitBranch(ctx context.Context, dir string) string {
	out, err := git(ctx, dir, "symbolic-ref", "--short", "-q", "HEAD")
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	out, err = git(ctx, dir, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// branchOf reads a folder's branch from .git/HEAD without running git, for
// folder lists.
func branchOf(dir string) (branch string, isGit bool) {
	data, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD"))
	if err != nil {
		st, serr := os.Stat(filepath.Join(dir, ".git"))
		return "", serr == nil && !st.IsDir() // a worktree's .git file
	}
	head := strings.TrimSpace(string(data))
	if ref, ok := strings.CutPrefix(head, "ref: refs/heads/"); ok {
		return ref, true
	}
	if len(head) >= 7 {
		return head[:7], true
	}
	return "", true
}

// gitBase is what changes are measured against: HEAD, or the empty tree
// before the first commit.
func gitBase(ctx context.Context, dir string) string {
	if _, err := git(ctx, dir, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		return emptyTree
	}
	return "HEAD"
}

// startStatus is the set of paths already changed in dir, taken when a
// session starts so Changes can tell its work from what was there before.
func startStatus(ctx context.Context, dir string) map[string]bool {
	out, err := git(ctx, dir, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil
	}
	changed := map[string]bool{}
	fields := bytes.Split(out, []byte{0})
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		changed[string(f[3:])] = true
		if f[0] == 'R' || f[0] == 'C' { // the next field is the old path
			i++
		}
	}
	return changed
}

// changes lists what differs from the last commit in the session's folder.
func changes(ctx context.Context, s store.CodeSession) wire.CodeChanges {
	out := wire.CodeChanges{Files: []wire.CodeChange{}}
	root := gitRoot(ctx, s.Cwd)
	if root == "" {
		for _, p := range s.Touched {
			status := "modified"
			if _, err := os.Stat(p); err != nil {
				status = "deleted"
			}
			out.Files = append(out.Files, wire.CodeChange{Path: p, Status: status})
		}
		return out
	}
	out.Git, out.Branch = true, gitBranch(ctx, root)
	base := gitBase(ctx, root)
	byPath := map[string]*wire.CodeChange{}
	var order []string
	add := func(c wire.CodeChange) *wire.CodeChange {
		if existing, ok := byPath[c.Path]; ok {
			return existing
		}
		c.BeforeSession = s.StartStatus[c.Path]
		byPath[c.Path] = &c
		order = append(order, c.Path)
		return &c
	}
	if raw, err := git(ctx, root, "diff", "--name-status", "-z", "-M", base); err == nil {
		fields := bytes.Split(raw, []byte{0})
		for i := 0; i+1 < len(fields); i++ {
			code := string(fields[i])
			if code == "" {
				continue
			}
			i++
			c := wire.CodeChange{Path: string(fields[i])}
			switch code[0] {
			case 'A':
				c.Status = "added"
			case 'D':
				c.Status = "deleted"
			case 'R', 'C':
				if i+1 < len(fields) {
					c.OldPath, c.Path = c.Path, string(fields[i+1])
					i++
				}
				c.Status = "renamed"
			default:
				c.Status = "modified"
			}
			add(c)
		}
	}
	if raw, err := git(ctx, root, "diff", "--numstat", "-z", "-M", base); err == nil {
		fields := bytes.Split(raw, []byte{0})
		for i := 0; i < len(fields); i++ {
			parts := strings.SplitN(string(fields[i]), "\t", 3)
			if len(parts) < 3 {
				continue
			}
			path := parts[2]
			if path == "" && i+2 < len(fields) { // a rename: old and new follow
				path = string(fields[i+2])
				i += 2
			}
			c, ok := byPath[path]
			if !ok {
				continue
			}
			if parts[0] == "-" {
				c.Binary = true
				continue
			}
			c.Added, _ = strconv.Atoi(parts[0])
			c.Removed, _ = strconv.Atoi(parts[1])
		}
	}
	if raw, err := git(ctx, root, "ls-files", "--others", "--exclude-standard", "-z"); err == nil {
		for _, f := range bytes.Split(raw, []byte{0}) {
			if len(f) == 0 {
				continue
			}
			c := add(wire.CodeChange{Path: string(f), Status: "untracked"})
			if c.Status != "untracked" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, c.Path))
			if err != nil {
				continue
			}
			if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
				c.Binary = true
			} else {
				c.Added = countLines(string(data))
			}
		}
	}
	for _, p := range order {
		out.Files = append(out.Files, *byPath[p])
	}
	return out
}

// diff is one file's unified diff against the last commit.
func diff(ctx context.Context, s store.CodeSession, path string) (wire.CodeDiff, bool) {
	out := wire.CodeDiff{Path: path}
	root := gitRoot(ctx, s.Cwd)
	if root == "" || path == "" || filepath.IsAbs(path) {
		return out, false
	}
	full := filepath.Join(root, path)
	if rel, err := filepath.Rel(root, full); err != nil || strings.HasPrefix(rel, "..") {
		return out, false
	}
	var raw []byte
	if _, err := git(ctx, root, "ls-files", "--error-unmatch", "--", path); err != nil {
		// Untracked: everything is new. git exits 1 when the files differ.
		if _, serr := os.Lstat(full); serr != nil {
			return out, false
		}
		raw, _ = git(ctx, root, "diff", "--no-index", "--", "/dev/null", path)
	} else {
		raw, err = git(ctx, root, "diff", "-M", gitBase(ctx, root), "--", path)
		if err != nil {
			return out, false
		}
	}
	if bytes.Contains(raw[:min(len(raw), 4000)], []byte("\nBinary files ")) || bytes.HasPrefix(raw, []byte("Binary files ")) {
		out.Binary = true
		if st, err := os.Stat(full); err == nil {
			out.Size = st.Size()
		}
		return out, true
	}
	lines := strings.SplitAfter(string(raw), "\n")
	if len(lines) > maxDiffLines {
		lines, out.Truncated = lines[:maxDiffLines], true
	}
	out.Diff = strings.Join(lines, "")
	return out, true
}
