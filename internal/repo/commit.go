package repo

import (
	"context"
	"fmt"
	"strings"
)

// CommitFiles commits exactly the given files as they are on disk, new,
// changed, renamed (list both names) or deleted, with message, and returns
// the new commit's short id. Other changes, staged or not, stay as they are.
func CommitFiles(ctx context.Context, dir, message string, paths []string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("%w: the commit message is empty", ErrInvalid)
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("%w: no files to commit", ErrInvalid)
	}
	f, err := open(dir)
	if err != nil {
		return "", err
	}
	var rels []string
	seen := map[string]bool{}
	for _, p := range paths {
		rel, _, err := f.resolve(p)
		if err != nil {
			return "", err
		}
		if rel == "." {
			return "", fmt.Errorf("%w: name files, not the whole folder", ErrInvalid)
		}
		if !seen[rel] {
			seen[rel] = true
			rels = append(rels, rel)
		}
	}
	if _, err := f.prefix(ctx); err != nil {
		return "", err
	}
	// git commit -- <paths> takes the files' contents from disk into a
	// commit of their own, but only files git knows: add the new ones first.
	out, err := f.git(ctx, append([]string{"ls-files", "-o", "--exclude-standard", "-z", "--"}, rels...)...)
	if err != nil {
		return "", err
	}
	added := splitZ(out)
	if len(added) > 0 {
		if _, _, err := f.run(ctx, commitTimeout, 0, append([]string{"add", "--"}, added...)...); err != nil {
			return "", err
		}
	}
	if _, _, err := f.run(ctx, commitTimeout, 0, append([]string{"commit", "--quiet", "--only", "-m", message, "--"}, rels...)...); err != nil {
		if len(added) > 0 { // leave the new files untracked again, as they were
			_, _, _ = f.run(context.WithoutCancel(ctx), readTimeout, 0, append([]string{"rm", "--cached", "--quiet", "--ignore-unmatch", "--"}, added...)...)
		}
		return "", err
	}
	out, err = f.git(ctx, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
