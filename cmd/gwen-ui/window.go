package main

import (
	"os"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The Gwen window's own small helpers (plan.md 3.15).

// PickProjectFolder asks for a project's folder in a folder dialog and
// returns it as an absolute path, or "" when the dialog was cancelled.
func (a *App) PickProjectFolder(start string) (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Choose the project's folder",
		DefaultDirectory:     start,
		CanCreateDirectories: true,
	})
	return path, wrap(err)
}

// PickFile asks for a file to attach to a task and returns its absolute
// path, or "" when the dialog was cancelled.
func (a *App) PickFile() (string, error) {
	home, _ := os.UserHomeDir()
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Attach a file", DefaultDirectory: home})
	return path, wrap(err)
}

// HomeDir is the user's home folder, so the window can show paths as "~/...".
func (a *App) HomeDir() string {
	home, _ := os.UserHomeDir()
	return home
}
