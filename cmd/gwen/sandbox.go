package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/remote"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// `gwen sandbox [claude arguments]` runs Claude Code in the bubblewrap
// sandbox of plan.md 3.14.8, in the current folder. Gwen Terminal's agent
// menu runs it for "Claude (sandbox)", and gwend for a sandboxed session.
func (c *cli) sandboxCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sandbox [claude arguments]",
		Short: "Run Claude Code in a sandbox that only sees this folder",
		Long: "Run Claude Code under bubblewrap in the current folder. Claude can read and write this folder, and use your\n" +
			"toolchains read-only. It cannot see the rest of your home folder, your SSH keys, your keyring, your desktop,\n" +
			"your other terminals, or your other Claude conversations, and it cannot change your Claude's settings or\n" +
			"sign-in: it has a private Claude folder of its own for this project and signs in separately (gwen\n" +
			"sandbox-login). The network stays on.",
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		PersistentPreRunE:  func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
				return cmd.Help()
			}
			argv, token, err := sandboxCommand(args)
			if err != nil {
				return err
			}
			bwrap, err := exec.LookPath(argv[0])
			if err != nil {
				return errors.New("the sandbox needs bubblewrap; install it with: sudo dnf install bubblewrap")
			}
			// The sandbox signs in with its own token, passed in the
			// environment so it never appears in a command line.
			env := withoutEnv(os.Environ(), "CLAUDE_CODE_OAUTH_TOKEN", "CLAUDE_CONFIG_DIR")
			if token != "" {
				env = append(env, "CLAUDE_CODE_OAUTH_TOKEN="+token)
			}
			return syscall.Exec(bwrap, argv, env)
		},
	}
}

func withoutEnv(env []string, names ...string) []string {
	out := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(names, name) {
			out = append(out, kv)
		}
	}
	return out
}

func sandboxCommand(args []string) (argv []string, token string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	if dir, err = filepath.EvalSymlinks(dir); err != nil {
		return nil, "", err
	}
	if dir == home || dir == "/" {
		return nil, "", usagef("start the sandbox inside a project folder, not in your home folder")
	}
	if os.Getenv("GWEN_SANDBOX") == "1" {
		return nil, "", usagef("this is already a sandbox")
	}
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, "", err
	}
	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return nil, "", fmt.Errorf("read Gwen's settings: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		return nil, "", err
	}
	claude := remote.FindClaude(home)
	if _, err := os.Stat(claude); err != nil {
		return nil, "", errors.New("there is no claude on your PATH: Claude Code isn't installed")
	}
	hook := filepath.Join(paths.RuntimeDir, "hook.sock")
	if _, err := os.Stat(hook); err != nil {
		return nil, "", client.ErrDaemonNotRunning
	}
	argv, err = remote.BuildSandbox(remote.SandboxOptions{Home: home, Dir: dir, GwenBin: self, RuntimeDir: paths.RuntimeDir,
		HookSocket: hook, Claude: claude, Extra: cfg.Remote.SandboxExtra, StateDir: remote.SandboxStateDir(paths.DataDir, dir)})
	if err != nil {
		return nil, "", err
	}
	token, err = config.ReadCredentialLine(paths.CredentialsDir(), config.CredClaudeSandboxToken)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, "", fmt.Errorf("read the sandbox sign-in: %w", err)
	}
	return append(argv, args...), token, nil
}

// `gwen sandbox-login` signs sandboxed sessions in once, for every project.
// A sandbox never sees your Claude's own sign-in, so it needs one of its
// own: a token from `claude setup-token`, which can only talk to the model.
func (c *cli) sandboxLoginCmd() *cobra.Command {
	var forget bool
	cmd := &cobra.Command{
		Use:   "sandbox-login",
		Short: "Sign sandboxed Claude sessions in, once for every project",
		Long: "A sandboxed Claude never sees your own Claude's sign-in, so that it can never change it. This runs\n" +
			"`claude setup-token`, which makes a sign-in token that can only talk to the model, and stores the token in\n" +
			"Gwen's credentials for every sandboxed session to use. Without it, each sandboxed project asks you to /login\n" +
			"the first time.",
		Args:              args(0),
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE: func(cmd *cobra.Command, _ []string) error {
			paths, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			if forget {
				if err := os.Remove(filepath.Join(paths.CredentialsDir(), config.CredClaudeSandboxToken)); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return err
				}
				fmt.Fprintln(c.stdout, "Forgot the sandbox sign-in. Sandboxed sessions will ask you to /login.")
				return nil
			}
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			claude := remote.FindClaude(home)
			if _, err := os.Stat(claude); err != nil {
				return errors.New("there is no claude on your PATH: Claude Code isn't installed")
			}
			fmt.Fprintln(c.stdout, "Claude will now make a sign-in token for sandboxed sessions. Follow its steps, then copy the token it shows.")
			fmt.Fprintln(c.stdout)
			setup := exec.CommandContext(cmd.Context(), claude, "setup-token")
			setup.Stdin, setup.Stdout, setup.Stderr = os.Stdin, os.Stdout, os.Stderr
			if err := setup.Run(); err != nil {
				return fmt.Errorf("claude setup-token didn't finish: %w", err)
			}
			fmt.Fprint(c.stdout, "\nPaste the token here (it is not shown), then press Enter: ")
			var token string
			if f, ok := c.stdin.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
				raw, err := term.ReadPassword(int(f.Fd()))
				fmt.Fprintln(c.stdout)
				if err != nil {
					return err
				}
				token = string(raw)
			} else {
				line, err := bufio.NewReader(c.stdin).ReadString('\n')
				if err != nil && line == "" {
					return errors.New("no token was given")
				}
				token = line
			}
			token = strings.TrimSpace(token)
			if token == "" || strings.ContainsAny(token, " \t\n") {
				return usagef("that doesn't look like a token; nothing was saved")
			}
			if err := config.WriteCredential(paths.CredentialsDir(), config.CredClaudeSandboxToken, []byte(token+"\n")); err != nil {
				return err
			}
			fmt.Fprintln(c.stdout, "Saved. Sandboxed sessions sign in with it from now on.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&forget, "forget", false, "remove the stored sandbox sign-in")
	return cmd
}
