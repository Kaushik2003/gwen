package main

import (
	"errors"

	"github.com/kzark/gwen/internal/repo"
	"github.com/kzark/gwen/internal/wire"
)

// A project's files, history, and changes for the workbench (plan.md 3.15,
// boards 2, 5 and 6): thin bindings over internal/repo. dir is the
// project's folder; paths are relative to it.

// Error codes the workbench tells apart, beside wire's.
const (
	codeNotRepo = "not_repo" // the folder has no git repository
	codeOutside = "outside"  // a path leaves the folder
	codeChanged = "changed"  // a save found the file changed on disk
)

// repoError gives a repo failure the JSON shape wrap gives API failures.
func repoError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, repo.ErrNotRepo):
		return &hostError{Code: codeNotRepo, Message: "This folder isn't a git repository", Details: map[string]any{}}
	case errors.Is(err, repo.ErrChanged):
		return &hostError{Code: codeChanged, Message: err.Error(), Details: map[string]any{}}
	case errors.Is(err, repo.ErrOutside):
		return &hostError{Code: codeOutside, Message: err.Error(), Details: map[string]any{}}
	case errors.Is(err, repo.ErrInvalid), errors.Is(err, repo.ErrBadRev), errors.Is(err, repo.ErrNotFile):
		return &hostError{Code: wire.CodeInvalidRequest, Message: err.Error(), Details: map[string]any{}}
	}
	return wrap(err)
}

func repoCall[T any](v T, err error) (*T, error) {
	if err != nil {
		return nil, repoError(err)
	}
	return &v, nil
}

// RepoTree lists the folder's files with their git status.
func (a *App) RepoTree(dir string) (*repo.Tree, error) { return repoCall(repo.ListFiles(a.ctx, dir)) }

// RepoFile reads one file's text.
func (a *App) RepoFile(dir, path string) (*repo.File, error) {
	return repoCall(repo.ReadFile(a.ctx, dir, path))
}

// RepoFileStamp tells whether a file changed on disk: its text changes when the file does.
func (a *App) RepoFileStamp(dir, path string) (string, error) {
	stamp, err := repo.FileStamp(a.ctx, dir, path)
	return stamp, repoError(err)
}

// RepoSaveFile writes a file's edited text; base is the hash it was read with.
func (a *App) RepoSaveFile(dir, path, text, base string) (*repo.File, error) {
	return repoCall(repo.SaveFile(a.ctx, dir, path, text, base))
}

// RepoMarks finds the lines to mark in a file's gutter.
func (a *App) RepoMarks(dir, path string) (*repo.Marks, error) {
	return repoCall(repo.LineMarks(a.ctx, dir, path))
}

// RepoBlame finds who last changed one line of a file.
func (a *App) RepoBlame(dir, path string, line int) (*repo.BlameLine, error) {
	return repoCall(repo.Blame(a.ctx, dir, path, line))
}

// RepoLog reads a page of history.
func (a *App) RepoLog(dir string, q repo.LogQuery) (*repo.Log, error) {
	return repoCall(repo.History(a.ctx, dir, q))
}

// RepoCommit reads one commit's message and files; hash "" is the uncommitted work.
func (a *App) RepoCommit(dir, hash string) (*repo.CommitDetail, error) {
	return repoCall(repo.Show(a.ctx, dir, hash))
}

// RepoDiff reads one file's diff in a commit, or in the uncommitted work when hash is "".
func (a *App) RepoDiff(dir, hash, path string) (*repo.DiffText, error) {
	return repoCall(repo.Diff(a.ctx, dir, hash, path))
}

// RepoCommitFiles commits exactly the listed files and returns the new short commit id.
func (a *App) RepoCommitFiles(dir, message string, paths []string) (string, error) {
	short, err := repo.CommitFiles(a.ctx, dir, message, paths)
	return short, repoError(err)
}

// OpenPath opens a file of the folder in the desktop's default app.
func (a *App) OpenPath(dir, path string) error {
	abs, err := repo.Resolve(dir, path)
	if err != nil {
		return repoError(err)
	}
	return wrap(a.host.start("xdg-open", abs))
}
