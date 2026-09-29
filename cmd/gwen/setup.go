package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
)

// Setup is the per-user half of installing Gwen (docs/09-packaging.md#setup):
// the systemd user unit, the tray's autostart entry, and phone push. Every
// identifier here starts with "setup" so that it cannot collide with the rest
// of package main.

// setupHealthWait is how long `gwen setup service` waits for the daemon.
const setupHealthWait = 5 * time.Second

// setupEnv is everything setup touches outside the process; tests replace it.
type setupEnv struct {
	api        func(socket string) client.API
	run        func(ctx context.Context, name string, args ...string) error // runs to completion
	start      func(path string) error                                      // starts detached
	stdin      io.Reader
	stdout     io.Writer
	clk        clock.Clock
	configHome string // $XDG_CONFIG_HOME
	shareDir   string // /usr/share/gwen
	exeDir     string // the directory of the running gwen
	credDir    string
}

// setupDefaultEnv is the real environment.
func setupDefaultEnv() (setupEnv, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return setupEnv{}, err
	}
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	return setupEnv{
		api: func(socket string) client.API { return client.New(socket) },
		run: func(ctx context.Context, name string, args ...string) error {
			out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
			if err != nil {
				return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
			}
			return nil
		},
		start: func(path string) error {
			cmd := exec.Command(path)
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Process.Release()
		},
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		clk:        clock.Real(),
		configHome: paths.ConfigHome,
		shareDir:   "/usr/share/gwen",
		exeDir:     exeDir,
		credDir:    paths.CredentialsDir(),
	}, nil
}

// newSetupCmd is `gwen setup`, which the root command registers.
func newSetupCmd() *cobra.Command { return setupCommand(setupDefaultEnv) }

func setupCommand(envFn func() (setupEnv, error)) *cobra.Command {
	var yes bool
	// begin prepares a run of one command.
	begin := func(cmd *cobra.Command) (*setupRun, error) {
		env, err := envFn()
		if err != nil {
			return nil, err
		}
		socket, _ := cmd.Flags().GetString("socket") // inherited from the root when present
		if socket == "" {
			if socket, err = config.DefaultSocketPath(); err != nil {
				return nil, err
			}
		}
		return &setupRun{env: env, ctx: cmd.Context(), api: env.api(socket), yes: yes,
			in: bufio.NewReader(env.stdin), out: env.stdout}, nil
	}

	root := &cobra.Command{
		Use:   "setup",
		Short: "Set Gwen up: start at login, the tray, and phone push",
		Long: "Runs each setup step in turn: the service, tracking thresholds, the tray, and phone push.\n" +
			"Every step is also its own subcommand and safe to run again.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			steps := []func() error{
				func() error { return s.service(false) },
				s.tracking,
				func() error { return s.autostart(false) },
				func() error { return s.phone("", "", "") },
				s.summary,
			}
			for _, step := range steps {
				if err := step(); err != nil {
					return err
				}
			}
			return nil
		},
	}
	root.PersistentFlags().BoolVar(&yes, "yes", false, "accept every default without prompting")

	var disableService bool
	service := &cobra.Command{
		Use:   "service",
		Short: "Start gwend now and at every login",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			return s.service(disableService)
		},
	}
	service.Flags().BoolVar(&disableService, "disable", false, "stop gwend and do not start it at login")

	tracking := &cobra.Command{
		Use:   "tracking",
		Short: "Set the daily target and the idle thresholds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			return s.tracking()
		},
	}

	var disableAutostart bool
	autostart := &cobra.Command{
		Use:   "autostart",
		Short: "Show the tray icon at every login",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			return s.autostart(disableAutostart)
		},
	}
	autostart.Flags().BoolVar(&disableAutostart, "disable", false, "remove the tray from autostart")

	var server, fallback, token string
	phone := &cobra.Command{
		Use:   "phone",
		Short: "Send nudges to your phone through ntfy",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := begin(cmd)
			if err != nil {
				return err
			}
			return s.phone(server, fallback, token)
		},
	}
	phone.Flags().StringVar(&server, "server", "", "ntfy server URL")
	phone.Flags().StringVar(&fallback, "fallback", "", "ntfy server to try when the first fails")
	phone.Flags().StringVar(&token, "token", "", "ntfy access token, stored in the credentials directory")

	root.AddCommand(service, tracking, autostart, phone, setupCalendarCmd(begin), setupLLMCmd(begin))
	return root
}

// setupRun is one run of a setup command.
type setupRun struct {
	env setupEnv
	ctx context.Context
	api client.API
	yes bool
	in  *bufio.Reader
	out io.Writer
}

func (s *setupRun) say(format string, args ...any) { fmt.Fprintf(s.out, format+"\n", args...) }

// ask prompts with the default in brackets; an empty answer, end of input, or
// --yes takes the default.
func (s *setupRun) ask(label, def string) (string, error) {
	if s.yes {
		s.say("%s: %s", label, def)
		return def, nil
	}
	fmt.Fprintf(s.out, "%s [%s]: ", label, def)
	line, err := s.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if line = strings.TrimSpace(line); line == "" {
		return def, nil
	}
	return line, nil
}

// service enables and starts the user unit, then waits for the daemon to
// answer; or disables and stops it.
func (s *setupRun) service(disable bool) error {
	if disable {
		if err := s.env.run(s.ctx, "systemctl", "--user", "disable", "--now", "gwend.service"); err != nil {
			return err
		}
		s.say("Gwen's daemon is stopped and will not start at login.")
		return nil
	}
	if err := s.env.run(s.ctx, "systemctl", "--user", "enable", "--now", "gwend.service"); err != nil {
		return err
	}
	deadline := s.env.clk.Now().Add(setupHealthWait)
	for {
		if _, err := s.api.Health(s.ctx); err == nil {
			s.say("Gwen is running and starts at login.")
			return nil
		}
		if !s.env.clk.Now().Before(deadline) {
			return errors.New("gwend did not answer within 5 s; see: journalctl --user -u gwend")
		}
		t := s.env.clk.NewTimer(250 * time.Millisecond)
		select {
		case <-s.ctx.Done():
			t.Stop()
			return s.ctx.Err()
		case <-t.C():
		}
	}
}

func (s *setupRun) tracking() error {
	cfg, err := s.api.GetConfig(s.ctx)
	if err != nil {
		return err
	}
	target, err := s.ask("Daily target", cfg.Tracking.DailyTarget)
	if err != nil {
		return err
	}
	soft, err := s.ask("Nudge after no input for", cfg.Tracking.SoftIdle)
	if err != nil {
		return err
	}
	hard, err := s.ask("Count as a break after", cfg.Tracking.HardIdle)
	if err != nil {
		return err
	}
	if _, err := s.api.PatchConfig(s.ctx, wire.ConfigPatch{"tracking": {
		"daily_target": target, "soft_idle": soft, "hard_idle": hard,
	}}); err != nil {
		return err
	}
	s.say("Tracking: %s a day; nudge after %s idle, break after %s.", target, soft, hard)
	return nil
}

func (s *setupRun) autostartPath() string {
	return filepath.Join(s.env.configHome, "autostart", "gwen-tray.desktop")
}

// autostart copies the packaged tray entry into the autostart directory and
// starts the tray now. Without a packaged entry, as after make install-dev, it
// writes one that runs the gwen-tray beside this gwen.
func (s *setupRun) autostart(disable bool) error {
	dst := s.autostartPath()
	if disable {
		if err := os.Remove(dst); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		s.say("The tray no longer starts at login.")
		return nil
	}
	tray := "gwen-tray"
	entry, err := os.ReadFile(filepath.Join(s.env.shareDir, "gwen-tray.desktop"))
	if errors.Is(err, fs.ErrNotExist) {
		tray = filepath.Join(s.env.exeDir, "gwen-tray")
		entry = []byte(setupTrayEntry(tray))
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, entry, 0o644); err != nil {
		return err
	}
	if err := s.env.start(tray); err != nil {
		s.say("The tray starts at your next login (starting it now failed: %v).", err)
		return nil
	}
	s.say("The tray is running and starts at login.")
	return nil
}

// setupTrayEntry is packaging/gwen-tray.desktop with an absolute Exec.
func setupTrayEntry(tray string) string {
	return "[Desktop Entry]\nType=Application\nName=Gwen tray\nComment=Gwen's tray icon\n" +
		"Exec=" + tray + "\nIcon=gwen\nTerminal=false\nNoDisplay=true\nX-GNOME-Autostart-enabled=true\n"
}

// phone turns on phone push: a topic, the servers, the optional token, and a
// test notification.
func (s *setupRun) phone(server, fallback, token string) error {
	cfg, err := s.api.GetConfig(s.ctx)
	if err != nil {
		return err
	}
	topic := cfg.Ntfy.Topic
	if topic == "" {
		topic = config.NewTopic()
	}
	if server == "" {
		if server, err = s.ask("ntfy server", cfg.Ntfy.Server); err != nil {
			return err
		}
	}
	if fallback == "" {
		if fallback, err = s.ask("Fallback ntfy server (empty for none)", cfg.Ntfy.FallbackServer); err != nil {
			return err
		}
	}
	if token != "" {
		if err := config.WriteCredential(s.env.credDir, config.CredNtfyToken, []byte(token+"\n")); err != nil {
			return err
		}
	}
	if _, err := s.api.PatchConfig(s.ctx, wire.ConfigPatch{
		"ntfy":  {"server": server, "fallback_server": fallback, "topic": topic},
		"nudge": {"phone": true},
	}); err != nil {
		return err
	}
	s.say("Phone push is on. Install the free ntfy app (Android: Play Store or F-Droid; iOS: App Store)")
	s.say("and subscribe to:")
	s.say("  %s/%s", strings.TrimRight(server, "/"), topic)
	if fallback != "" {
		s.say("  %s/%s", strings.TrimRight(fallback, "/"), topic)
	}
	s.say("The topic is private: anyone who knows it can read your nudges.")
	res, err := s.api.NotifyTest(s.ctx)
	if err != nil {
		return err
	}
	s.say("Test notification: desktop %s, phone %s.", res.Desktop, res.Phone)
	return nil
}

func (s *setupRun) summary() error {
	cfg, err := s.api.GetConfig(s.ctx)
	if err != nil {
		return err
	}
	s.say("")
	s.say("Gwen is set up.")
	daemon := "not answering"
	if _, err := s.api.Health(s.ctx); err == nil {
		daemon = "running, starts at login"
	}
	s.say("  Daemon:     %s", daemon)
	tray := "off"
	if _, err := os.Stat(s.autostartPath()); err == nil {
		tray = "starts at login"
	}
	s.say("  Tray:       %s", tray)
	phone := "off"
	if cfg.Nudge.Phone && cfg.Ntfy.Topic != "" {
		phone = "on, " + strings.TrimRight(cfg.Ntfy.Server, "/") + "/" + cfg.Ntfy.Topic
	}
	s.say("  Phone push: %s", phone)
	s.say("  Dashboard:  open Gwen from your app menu, or run gwen-ui")
	return nil
}
