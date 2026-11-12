package remote

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/wire"
)

// ErrOutsideRoots is a folder a phone may not use.
var ErrOutsideRoots = errors.New("that folder is outside the folders Gwen may open from the phone")

// roots are the configured roots as real paths.
func (s *Service) roots() []string {
	var out []string
	for _, r := range s.cfg().Remote.Roots {
		p := ExpandHome(r, s.home)
		if real, err := filepath.EvalSymlinks(p); err == nil {
			out = append(out, real)
		}
	}
	return out
}

// within reports whether path is root or below it.
func within(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// allowedDir resolves dir and checks it against plan.md 3.14.7: inside a
// root, not inside a hidden folder, and not one of Gwen's own folders.
func (s *Service) allowedDir(dir string) (string, error) {
	dir = ExpandHome(strings.TrimSpace(dir), s.home)
	if !filepath.IsAbs(dir) {
		return "", ErrOutsideRoots
	}
	real, err := filepath.EvalSymlinks(filepath.Clean(dir))
	if err != nil {
		return "", err
	}
	for _, own := range s.ownDirs {
		if own != "" && within(real, own) {
			return "", ErrOutsideRoots
		}
	}
	for _, root := range s.roots() {
		if !within(real, root) {
			continue
		}
		rel, _ := filepath.Rel(root, real)
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			if strings.HasPrefix(part, ".") && part != "." {
				return "", ErrOutsideRoots
			}
		}
		return real, nil
	}
	return "", ErrOutsideRoots
}

// Folders lists the subfolders of path, or of the first root when path is "".
func (s *Service) Folders(path string) (wire.CodeFolders, error) {
	roots := s.roots()
	out := wire.CodeFolders{Folders: []wire.CodeFolder{}, Roots: roots}
	if len(roots) == 0 {
		return out, ErrOutsideRoots
	}
	if path == "" {
		path = roots[0]
	}
	dir, err := s.allowedDir(path)
	if err != nil {
		return out, err
	}
	out.Path = dir
	if parent := filepath.Dir(dir); parent != dir {
		if _, err := s.allowedDir(parent); err == nil {
			out.Parent = parent
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dir, name)
		if !e.IsDir() {
			// Follow a link to a folder, but only one that stays allowed.
			if e.Type()&os.ModeSymlink == 0 {
				continue
			}
			if st, err := os.Stat(full); err != nil || !st.IsDir() {
				continue
			}
			if _, err := s.allowedDir(full); err != nil {
				continue
			}
		}
		branch, isGit := branchOf(full)
		out.Folders = append(out.Folders, wire.CodeFolder{Name: name, Path: full, Git: isGit, Branch: branch})
	}
	sort.Slice(out.Folders, func(i, j int) bool {
		return strings.ToLower(out.Folders[i].Name) < strings.ToLower(out.Folders[j].Name)
	})
	return out, nil
}

// newestTranscript is the most recently written transcript in a Claude
// project folder.
func newestTranscript(dir string) (string, time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", time.Time{}
	}
	var best string
	var at time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(at) {
			best, at = filepath.Join(dir, e.Name()), info.ModTime()
		}
	}
	return best, at
}

// RecentFolders are the folders Claude Code last worked in: the working
// folder of the newest transcript of each project, the newest 12 that still
// exist inside a root.
func (s *Service) RecentFolders() wire.CodeFolders {
	out := wire.CodeFolders{Folders: []wire.CodeFolder{}, Roots: s.roots()}
	projects, err := os.ReadDir(claudeProjectsDir(s.home))
	if err != nil {
		return out
	}
	type recent struct {
		path string
		at   time.Time
	}
	var found []recent
	for _, p := range projects {
		if !p.IsDir() {
			continue
		}
		file, at := newestTranscript(filepath.Join(claudeProjectsDir(s.home), p.Name()))
		if file == "" {
			continue
		}
		found = append(found, recent{file, at})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].at.After(found[j].at) })
	seen := map[string]bool{}
	for _, f := range found {
		if len(out.Folders) == 12 {
			break
		}
		cwd := transcriptCwd(f.path)
		if cwd == "" {
			continue
		}
		dir, err := s.allowedDir(cwd)
		if err != nil || seen[dir] {
			continue
		}
		seen[dir] = true
		branch, isGit := branchOf(dir)
		out.Folders = append(out.Folders, wire.CodeFolder{Name: filepath.Base(dir), Path: dir, Git: isGit, Branch: branch})
	}
	return out
}

// projectDirName is the folder Claude Code keeps a working folder's
// transcripts in: the path with every character that is not a letter or
// digit turned into "-".
func projectDirName(cwd string) string {
	b := []byte(cwd)
	for i, c := range b {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			b[i] = '-'
		}
	}
	return string(b)
}

// conversationFiles are the transcripts whose working folder is dir, newest
// first.
func (s *Service) conversationFiles(dir string) []string {
	type file struct {
		path string
		at   time.Time
	}
	var files []file
	// The folder Claude Code derives from the path holds them; check each
	// transcript's own cwd, since different paths can share a folder name.
	entries, _ := os.ReadDir(filepath.Join(claudeProjectsDir(s.home), projectDirName(dir)))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, file{filepath.Join(claudeProjectsDir(s.home), projectDirName(dir), e.Name()), info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].at.After(files[j].at) })
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.path)
	}
	return out
}
