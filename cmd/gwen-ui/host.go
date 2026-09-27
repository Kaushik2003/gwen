package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
)

// host performs the file and systemd operations the first-run and Settings
// screens need, the same ones `gwen setup` performs (docs/09-packaging.md#setup).
type host struct {
	api        client.API
	clk        clock.Clock
	run        func(ctx context.Context, name string, args ...string) error // runs to completion
	start      func(name string, args ...string) error                      // starts detached
	configHome string
	shareDir   string
	exeDir     string
	credDir    string
}

// credentials are the files SetCredential may write.
var credentials = []string{config.CredNtfyToken, config.CredGoogleClient, config.CredGoogleToken,
	config.CredLLMAPIKey, config.CredSyncToken}

func (h *host) newTopic() string { return config.NewTopic() }

func (h *host) setCredential(name, value string) error {
	if !slices.Contains(credentials, name) {
		return fmt.Errorf("%q is not a credential Gwen knows", name)
	}
	return config.WriteCredential(h.credDir, name, []byte(value+"\n"))
}

func (h *host) openURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("only http and https links open")
	}
	return h.start("xdg-open", raw)
}

// enableService runs systemctl --user enable --now gwend.service and waits
// up to 5 s for the daemon to answer.
func (h *host) enableService(ctx context.Context) error {
	if err := h.run(ctx, "systemctl", "--user", "enable", "--now", "gwend.service"); err != nil {
		return err
	}
	deadline := h.clk.Now().Add(5 * time.Second)
	for {
		if _, err := h.api.Health(ctx); err == nil {
			return nil
		}
		if !h.clk.Now().Before(deadline) {
			return errors.New("gwend did not answer within 5 s; see: journalctl --user -u gwend")
		}
		t := h.clk.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C():
		}
	}
}

func (h *host) autostartPath() string {
	return filepath.Join(h.configHome, "autostart", "gwen-tray.desktop")
}

func (h *host) autostart() bool {
	_, err := os.Stat(h.autostartPath())
	return err == nil
}

// setAutostart copies the packaged tray entry into autostart and starts the
// tray, or removes the entry. Without a packaged entry, as after make
// install-dev, it writes one for the gwen-tray beside gwen-ui.
func (h *host) setAutostart(enable bool) error {
	dst := h.autostartPath()
	if !enable {
		if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	tray := "gwen-tray"
	entry, err := os.ReadFile(filepath.Join(h.shareDir, "gwen-tray.desktop"))
	if errors.Is(err, fs.ErrNotExist) {
		tray = filepath.Join(h.exeDir, "gwen-tray")
		entry = []byte("[Desktop Entry]\nType=Application\nName=Gwen tray\nComment=Gwen's tray icon\n" +
			"Exec=" + tray + "\nIcon=gwen\nTerminal=false\nNoDisplay=true\nX-GNOME-Autostart-enabled=true\n")
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, entry, 0o644); err != nil {
		return err
	}
	return h.start(tray)
}

func runCommand(ctx context.Context, name string, args ...string) error {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w: %s", name, err, out)
	}
	return nil
}

func startCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
