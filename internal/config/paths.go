package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Paths are Gwen's filesystem locations (docs/02-architecture.md#filesystem),
// resolved from the XDG base directories.
type Paths struct {
	ConfigHome string // $XDG_CONFIG_HOME, e.g. ~/.config
	ConfigFile string // $XDG_CONFIG_HOME/gwen/config.toml
	DataDir    string // $XDG_DATA_HOME/gwen
	StateDir   string // $XDG_STATE_HOME/gwen
	RuntimeDir string // $XDG_RUNTIME_DIR/gwen
}

// DefaultPaths resolves Paths from the environment with the standard
// env-var-then-default rules: a variable counts only if it is an absolute path.
// Without XDG_RUNTIME_DIR the runtime directory falls back to a per-user
// directory under os.TempDir.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	configHome := xdg("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	runtime := filepath.Join(os.TempDir(), fmt.Sprintf("gwen-%d", os.Getuid()))
	if dir := xdg("XDG_RUNTIME_DIR", ""); dir != "" {
		runtime = filepath.Join(dir, "gwen")
	}
	return Paths{
		ConfigHome: configHome,
		ConfigFile: filepath.Join(configHome, "gwen", "config.toml"),
		DataDir:    filepath.Join(xdg("XDG_DATA_HOME", filepath.Join(home, ".local", "share")), "gwen"),
		StateDir:   filepath.Join(xdg("XDG_STATE_HOME", filepath.Join(home, ".local", "state")), "gwen"),
		RuntimeDir: runtime,
	}, nil
}

func xdg(name, fallback string) string {
	if v := os.Getenv(name); filepath.IsAbs(v) {
		return v
	}
	return fallback
}

// DatabaseFile is the SQLite database.
func (p Paths) DatabaseFile() string { return filepath.Join(p.DataDir, "gwen.db") }

// CredentialsDir holds the secrets of docs/07-integrations.md#credentials.
func (p Paths) CredentialsDir() string { return CredentialsDir(p.DataDir) }

// LogFile is the daemon's rotated JSON log.
func (p Paths) LogFile() string { return filepath.Join(p.StateDir, "gwend.log") }

// SocketPath is the daemon's API socket.
func (p Paths) SocketPath() string { return filepath.Join(p.RuntimeDir, "gwend.sock") }

// DefaultSocketPath is Paths.SocketPath for the current environment.
func DefaultSocketPath() (string, error) {
	p, err := DefaultPaths()
	if err != nil {
		return "", err
	}
	return p.SocketPath(), nil
}
