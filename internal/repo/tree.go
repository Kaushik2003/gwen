package repo

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
)

// Tree is a folder's files for the Files panel (plan.md 3.15, board 5).
type Tree struct {
	Root      string     `json:"root"`     // the folder, as given
	Git       bool       `json:"git"`      // false: a plain walk, no statuses
	Branch    string     `json:"branch"`   // "" when detached or without git
	Upstream  string     `json:"upstream"` // such as origin/main; "" without one
	Ahead     int        `json:"ahead"`
	Behind    int        `json:"behind"`
	Files     []TreeFile `json:"files"` // sorted by path
	Truncated bool       `json:"truncated"`
}

// TreeFile is one file. Folders are implied by the paths.
type TreeFile struct {
	Path string `json:"path"`
	// Status is "" (unchanged), "M" changed, "A" added, "U" new and
	// untracked, "D" deleted, or "R" renamed.
	Status  string `json:"status"`
	Staged  bool   `json:"staged"`
	OldPath string `json:"old_path"` // a rename's old path
}

// ListFiles lists the folder's files: in a repository, the tracked and
// untracked files git does not ignore (git ls-files -co --exclude-standard),
// the deleted ones, and every file's status; elsewhere, a walk that skips
// .git and node_modules. At most MaxTreeFiles are returned, changed files
// first.
func ListFiles(ctx context.Context, dir string) (Tree, error) {
	return listFiles(ctx, dir, MaxTreeFiles)
}

func listFiles(ctx context.Context, dir string, limit int) (Tree, error) {
	f, err := open(dir)
	if err != nil {
		return Tree{}, err
	}
	t := Tree{Root: f.dir, Files: []TreeFile{}}
	prefix, err := f.prefix(ctx)
	if errors.Is(err, ErrNotRepo) {
		return walk(f, t, limit)
	}
	if err != nil {
		return Tree{}, err
	}
	t.Git = true
	st, err := f.status(ctx, prefix)
	if err != nil {
		return Tree{}, err
	}
	t.Branch, t.Upstream, t.Ahead, t.Behind = st.head, st.upstream, st.ahead, st.behind
	out, err := f.git(ctx, "ls-files", "-co", "--exclude-standard", "-z")
	if err != nil {
		return Tree{}, err
	}
	at := map[string]int{}
	add := func(path string) int {
		if k, ok := at[path]; ok {
			return k
		}
		at[path] = len(t.Files)
		t.Files = append(t.Files, TreeFile{Path: path})
		return len(t.Files) - 1
	}
	for _, p := range splitZ(out) {
		if p != "" && p[len(p)-1] != '/' { // "dir/" is a nested repository
			add(p)
		}
	}
	for _, e := range st.entries {
		k := add(e.path)
		t.Files[k].Status, t.Files[k].Staged, t.Files[k].OldPath = e.letter(), e.staged(), e.orig
	}
	sort.Slice(t.Files, func(i, j int) bool { return t.Files[i].Path < t.Files[j].Path })
	if len(t.Files) > limit {
		t.Truncated = true
		kept := make([]TreeFile, 0, limit)
		room := limit
		for _, tf := range t.Files {
			if tf.Status != "" {
				room--
			}
		}
		for _, tf := range t.Files {
			if tf.Status != "" || room > 0 {
				if tf.Status == "" {
					room--
				}
				kept = append(kept, tf)
			}
		}
		t.Files = kept
	}
	return t, nil
}

// walk lists a folder without git.
func walk(f folder, t Tree, limit int) (Tree, error) {
	err := filepath.WalkDir(f.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == f.root {
				return err
			}
			return nil // unreadable: leave it out
		}
		if p == f.root {
			return nil
		}
		if d.Name() == ".git" || (d.IsDir() && d.Name() == "node_modules") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if len(t.Files) >= limit {
			t.Truncated = true
			return fs.SkipAll
		}
		rel, err := filepath.Rel(f.root, p)
		if err != nil {
			return nil
		}
		t.Files = append(t.Files, TreeFile{Path: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		return Tree{}, err
	}
	sort.Slice(t.Files, func(i, j int) bool { return t.Files[i].Path < t.Files[j].Path })
	return t, nil
}
