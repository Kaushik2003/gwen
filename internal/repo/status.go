package repo

import (
	"context"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// statusEntry is one path from git status --porcelain=v2.
type statusEntry struct {
	path string // relative to the folder
	orig string // a rename's old path, relative to the folder
	kind byte   // '1' changed, '2' renamed or copied, 'u' unmerged, '?' untracked
	x, y byte   // index and worktree letters; '.' is unchanged
}

// letter is the status the tree shows: M, A, U (untracked), D, or R.
func (e statusEntry) letter() string {
	switch {
	case e.kind == '?':
		return "U"
	case e.kind == 'u':
		return "M"
	case e.y == 'D' || (e.y == '.' && e.x == 'D'):
		return "D"
	case e.x == 'R':
		return "R"
	case e.x == 'A' || e.x == 'C':
		return "A"
	}
	return "M"
}

// staged reports whether part of the change is in the index.
func (e statusEntry) staged() bool { return e.kind != '?' && e.x != '.' }

type status struct {
	head     string // the branch, "" when detached
	upstream string
	ahead    int
	behind   int
	entries  []statusEntry
}

// status runs git status for the folder, or for the given folder-relative
// paths, with paths made relative to the folder.
func (f folder) status(ctx context.Context, prefix string, paths ...string) (status, error) {
	args := []string{"status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all", "--"}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	out, err := f.git(ctx, append(args, paths...)...)
	if err != nil {
		return status{}, err
	}
	return parseStatus(out, prefix), nil
}

func parseStatus(out []byte, prefix string) status {
	var st status
	toks := splitZ(out)
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "" {
			continue
		}
		if h, ok := strings.CutPrefix(t, "# "); ok {
			key, val, _ := strings.Cut(h, " ")
			switch key {
			case "branch.head":
				if val != "(detached)" {
					st.head = val
				}
			case "branch.upstream":
				st.upstream = val
			case "branch.ab":
				a, b, _ := strings.Cut(val, " ")
				st.ahead, _ = strconv.Atoi(strings.TrimPrefix(a, "+"))
				st.behind, _ = strconv.Atoi(strings.TrimPrefix(b, "-"))
			}
			continue
		}
		var e statusEntry
		switch t[0] {
		case '1':
			f := strings.SplitN(t, " ", 9)
			if len(f) < 9 {
				continue
			}
			e = statusEntry{kind: '1', x: f[1][0], y: f[1][1], path: f[8]}
		case '2':
			f := strings.SplitN(t, " ", 10)
			if len(f) < 10 || i+1 >= len(toks) {
				continue
			}
			i++
			e = statusEntry{kind: '2', x: f[1][0], y: f[1][1], path: f[9], orig: toks[i]}
		case 'u':
			f := strings.SplitN(t, " ", 11)
			if len(f) < 11 {
				continue
			}
			e = statusEntry{kind: 'u', x: f[1][0], y: f[1][1], path: f[10]}
		case '?':
			e = statusEntry{kind: '?', x: '?', y: '?', path: strings.TrimPrefix(t, "? ")}
		default: // '!' ignored
			continue
		}
		// A nested repository shows as "dir/"; it is not a file of this one.
		if strings.HasSuffix(e.path, "/") {
			continue
		}
		p, ok := strings.CutPrefix(e.path, prefix)
		if !ok {
			continue
		}
		e.path = p
		if e.orig != "" {
			e.orig, _ = strings.CutPrefix(e.orig, prefix)
		}
		st.entries = append(st.entries, e)
	}
	return st
}

// FileChange is one file of a commit, or of the uncommitted work.
type FileChange struct {
	Path    string `json:"path"`
	OldPath string `json:"old_path"`
	// Status is A, M, D, R, C or T; in the uncommitted work also U, a new
	// file git does not track yet.
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Binary  bool   `json:"binary"`
}

// parseRawNumstat reads git's --raw --numstat -z output: the raw entries
// give each file's status and names, the numstat entries its line counts.
func parseRawNumstat(out []byte) []FileChange {
	toks := splitZ(out)
	var files []FileChange
	at := map[string]int{}
	i := 0
	for i < len(toks) && strings.HasPrefix(toks[i], ":") {
		f := strings.Fields(toks[i])
		i++
		if len(f) < 5 || i >= len(toks) {
			break
		}
		fc := FileChange{Status: f[4][:1]}
		if fc.Status == "R" || fc.Status == "C" {
			if i+1 >= len(toks) {
				break
			}
			fc.OldPath, fc.Path = toks[i], toks[i+1]
			i += 2
		} else {
			fc.Path = toks[i]
			i++
		}
		if fc.Status == "U" || fc.Status == "X" { // unmerged, unknown: changed
			fc.Status = "M"
		}
		at[fc.Path] = len(files)
		files = append(files, fc)
	}
	for i < len(toks) {
		t := strings.TrimPrefix(toks[i], "\n")
		i++
		f := strings.SplitN(t, "\t", 3)
		if len(f) < 3 {
			continue
		}
		path := f[2]
		if path == "" { // a rename: old and new names follow
			if i+1 >= len(toks) {
				break
			}
			path = toks[i+1]
			i += 2
		}
		k, ok := at[path]
		if !ok {
			continue
		}
		if f[0] == "-" {
			files[k].Binary = true
			continue
		}
		files[k].Added, _ = strconv.Atoi(f[0])
		files[k].Removed, _ = strconv.Atoi(f[1])
	}
	return files
}

// changes is the uncommitted work against base (HEAD, or the empty tree):
// tracked changes with their line counts, then untracked files with theirs.
// paths limits it to some folder-relative paths.
func (f folder) changes(ctx context.Context, prefix, base string, paths ...string) (status, []FileChange, error) {
	st, err := f.status(ctx, prefix, paths...)
	if err != nil {
		return status{}, nil, err
	}
	args := []string{"diff", base, "--raw", "--numstat", "-z", "-M", "--relative", "--no-ext-diff", "--no-textconv", "--"}
	if len(paths) == 0 {
		paths = scope(prefix)
	}
	out, err := f.git(ctx, append(args, paths...)...)
	if err != nil {
		return status{}, nil, err
	}
	files := parseRawNumstat(out)
	tracked := map[string]bool{}
	for _, fc := range files {
		tracked[fc.Path] = true
	}
	untracked := 0
	for _, e := range st.entries {
		if e.kind != '?' || tracked[e.path] {
			continue
		}
		fc := FileChange{Path: e.path, Status: "U"}
		// Counting lines reads the file; past a few thousand new files
		// (a build folder nobody ignored) only the names are worth it.
		if untracked++; untracked <= 2000 {
			if b, err := readHead(filepath.Join(f.root, filepath.FromSlash(e.path)), MaxFileBytes); err == nil {
				if isBinary(b) {
					fc.Binary = true
				} else {
					fc.Added = countLines(cutAtLine(b, MaxFileBytes))
				}
			}
		}
		files = append(files, fc)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return st, files, nil
}

// lineCount counts a file's lines, up to MaxFileBytes; binary files have none.
func lineCount(path string) int {
	b, err := readHead(path, MaxFileBytes)
	if err != nil || isBinary(b) {
		return 0
	}
	return countLines(cutAtLine(b, MaxFileBytes))
}
