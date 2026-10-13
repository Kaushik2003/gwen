package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// The memory screen and Settings, Memory: the client's memory methods, plus
// the file dialogs, the export file, and the memory folder.

func init() {
	// Settings, Memory saves the optional memory services' API keys.
	credentials = append(credentials, config.CredMem0APIKey, config.CredSupermemoryAPIKey)
}

func (a *App) AssistantSelfHistory(limit int) (*wire.SelfHistory, error) {
	return call(a.api.AssistantSelfHistory(a.ctx, limit))
}
func (a *App) MemoryStatus() (*wire.MemoryStatus, error) { return call(a.api.MemoryStatus(a.ctx)) }
func (a *App) ListClaims(q wire.MemoryClaimQuery) (*wire.MemoryClaimList, error) {
	return call(a.api.ListClaims(a.ctx, q))
}
func (a *App) GetClaim(id string) (*wire.MemoryClaim, error) { return call(a.api.GetClaim(a.ctx, id)) }
func (a *App) EditClaim(id string, req wire.EditClaimRequest) (*wire.MemoryClaimsResult, error) {
	return call(a.api.EditClaim(a.ctx, id, req))
}
func (a *App) ConfirmClaim(id string) (*wire.MemoryClaim, error) {
	return call(a.api.ConfirmClaim(a.ctx, id))
}
func (a *App) RejectClaim(id string) (*wire.MemoryClaim, error) {
	return call(a.api.RejectClaim(a.ctx, id))
}
func (a *App) HideClaim(id string, req wire.HideClaimRequest) (*wire.MemoryClaim, error) {
	return call(a.api.HideClaim(a.ctx, id, req))
}
func (a *App) ForgetClaim(id string) (*wire.ForgetReceipt, error) {
	return call(a.api.ForgetClaim(a.ctx, id))
}
func (a *App) ExplainMemory(ref string) (*wire.MemoryExplanation, error) {
	return call(a.api.ExplainMemory(a.ctx, ref))
}
func (a *App) Remember(req wire.RememberRequest, channel string) (*wire.MemoryClaimsResult, error) {
	return call(a.api.Remember(a.ctx, req, channel))
}
func (a *App) RecallMemory(req wire.RecallRequest) (*wire.RecallResult, error) {
	return call(a.api.RecallMemory(a.ctx, req))
}
func (a *App) ListConflicts(status string) (*wire.MemoryConflictList, error) {
	return call(a.api.ListConflicts(a.ctx, status))
}
func (a *App) ResolveConflict(id string, req wire.ResolveConflictRequest) (*wire.MemoryConflict, error) {
	return call(a.api.ResolveConflict(a.ctx, id, req))
}
func (a *App) ListEntities() (*wire.MemoryEntityList, error) { return call(a.api.ListEntities(a.ctx)) }
func (a *App) MemoryProfile() (*wire.ProfileCard, error)     { return call(a.api.MemoryProfile(a.ctx)) }
func (a *App) ListResources() (*wire.MemoryResourceList, error) {
	return call(a.api.ListResources(a.ctx))
}
func (a *App) AddResource(req wire.AddResourceRequest) (*wire.MemoryResource, error) {
	return call(a.api.AddResource(a.ctx, req))
}
func (a *App) RemoveResource(id string) (*wire.ForgetReceipt, error) {
	return call(a.api.RemoveResource(a.ctx, id))
}
func (a *App) RebuildMemory(req wire.RebuildRequest) error {
	return wrap(a.api.RebuildMemory(a.ctx, req))
}
func (a *App) SleepMemory() (*wire.SleepReport, error)   { return call(a.api.SleepMemory(a.ctx)) }
func (a *App) ExportMemory() (*wire.MemoryExport, error) { return call(a.api.ExportMemory(a.ctx)) }
func (a *App) ForgetAllMemory(confirm string) (*wire.ForgetReceipt, error) {
	return call(a.api.ForgetAllMemory(a.ctx, confirm))
}

// documentPattern is the files the daemon can read the text of.
const documentPattern = "*.txt;*.md;*.markdown;*.org;*.csv;*.json;*.log;*.html;*.htm;*.pdf"

// PickDocument asks for a document for Gwen to remember in a file dialog; it
// is "" when the dialog was cancelled.
func (a *App) PickDocument() (string, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose a document for Gwen to remember",
		Filters: []runtime.FileFilter{
			{DisplayName: "Documents (text, Markdown, HTML, PDF)", Pattern: documentPattern},
		},
	})
	return path, wrap(err)
}

// PickFolder asks for the readable memory folder in a folder dialog. A folder
// in the home directory comes back as "~/...", as the config keeps it; ""
// means the dialog was cancelled.
func (a *App) PickFolder() (string, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:                "Choose the folder for Gwen's memory files",
		CanCreateDirectories: true,
	})
	if err != nil || path == "" {
		return "", wrap(err)
	}
	home, _ := os.UserHomeDir()
	return homeRelative(path, home), nil
}

// SaveMemoryExport asks where to save everything memory holds, then writes it
// there as JSON readable only by the user. It returns the path, or "" when
// the dialog was cancelled.
func (a *App) SaveMemoryExport() (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save Gwen's memory",
		DefaultFilename: "gwen-memory-" + time.Now().Format(time.DateOnly) + ".json",
		Filters:         []runtime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}},
	})
	if err != nil || path == "" {
		return "", wrap(err)
	}
	export, err := a.api.ExportMemory(a.ctx)
	if err != nil {
		return "", wrap(err)
	}
	return path, wrap(writeExport(path, export))
}

// OpenMemoryFolder opens the readable memory folder of the config in the file
// manager.
func (a *App) OpenMemoryFolder() error {
	cfg, err := a.api.GetConfig(a.ctx)
	if err != nil {
		return wrap(err)
	}
	home, _ := os.UserHomeDir()
	dir := expandHome(cfg.Memory.FilesDir, home)
	if dir == "" {
		return wrap(errors.New("the memory folder is off; turn it on first"))
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return wrap(fmt.Errorf("%s isn't there yet. Gwen makes it once she has something to write", cfg.Memory.FilesDir))
	}
	return wrap(a.host.start("xdg-open", dir))
}

// writeExport writes the export as indented JSON with mode 0600, replacing
// what is at path.
func writeExport(path string, export *wire.MemoryExport) error {
	b, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	// A file that was there keeps its mode on open; memory is private.
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// homeRelative is path as "~/..." when it is inside home, else path.
func homeRelative(path, home string) string {
	if home == "" {
		return path
	}
	rel, err := filepath.Rel(home, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return "~/" + filepath.ToSlash(rel)
}

// expandHome is dir with a leading "~/" replaced by home.
func expandHome(dir, home string) string {
	if rest, ok := strings.CutPrefix(dir, "~/"); ok && home != "" {
		return filepath.Join(home, rest)
	}
	return dir
}
