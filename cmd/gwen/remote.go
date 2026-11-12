package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/wire"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

// `gwen remote` is Gwen on the phone from the command line (plan.md 3.14):
// what Settings › Phone does, for a terminal.
func (c *cli) remoteCmd() *cobra.Command {
	remote := group("remote", "Use Gwen from your phone: pair it, publish to your tailnet, list sessions")

	status := &cobra.Command{
		Use: "status", Short: "Show whether your phone can reach this laptop", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.RemoteStatus(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(st, func(w io.Writer) error { return printRemote(w, st) })
		},
	}
	on := &cobra.Command{
		Use: "on", Short: "Publish Gwen to your tailnet (asks for your password once)", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := c.api.PatchConfig(cmd.Context(), wire.ConfigPatch{"remote": {"enabled": true}}); err != nil {
				return err
			}
			st, err := c.api.RemoteStatus(cmd.Context())
			if err != nil {
				return err
			}
			if st.Tailscale != "ok" {
				return fmt.Errorf("tailscale isn't running on this laptop (%s); start it, then run this again", st.Tailscale)
			}
			if !st.Certificates {
				fmt.Fprintln(c.stderr, "HTTPS certificates are off in your tailnet. Turn them on at https://login.tailscale.com/admin/dns, then run this again.")
			}
			if err := c.serve(strconv.Itoa(st.Port), "--bg", "--https="+strconv.Itoa(st.Port), "unix:"+st.Socket); err != nil {
				return err
			}
			if st, err = c.api.RemoteStatus(cmd.Context()); err != nil {
				return err
			}
			return c.emit(st, func(w io.Writer) error { return printRemote(w, st) })
		},
	}
	off := &cobra.Command{
		Use: "off", Short: "Stop publishing Gwen to your tailnet", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := c.api.RemoteStatus(cmd.Context())
			if err != nil {
				return err
			}
			if st.Published {
				if err := c.serve(strconv.Itoa(st.Port), "--https="+strconv.Itoa(st.Port), "off"); err != nil {
					return err
				}
			}
			if _, err := c.api.PatchConfig(cmd.Context(), wire.ConfigPatch{"remote": {"enabled": false}}); err != nil {
				return err
			}
			fmt.Fprintln(c.stdout, "Your phone can no longer reach this laptop. Paired phones stay paired.")
			return nil
		},
	}
	pair := &cobra.Command{
		Use: "pair", Short: "Show a link that pairs a phone (valid ten minutes, one use)", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			code, err := c.api.RemotePairCode(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(code, func(w io.Writer) error {
				t := wire.Time(code.ExpiresAt).In(c.loc)
				fmt.Fprintf(w, "Open this on your phone before %s:\n\n  %s\n\nSettings › Phone shows it as a QR code.\n",
					client.FormatTime(t), code.URL)
				return nil
			})
		},
	}
	devices := &cobra.Command{
		Use: "devices", Short: "List paired phones", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.RemoteDevices(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if len(list.Devices) == 0 {
					fmt.Fprintln(w, "No phone is paired. Pair one with: gwen remote pair")
					return nil
				}
				for _, d := range list.Devices {
					seen := "never used"
					if d.LastSeenAt != nil {
						t := wire.Time(*d.LastSeenAt).In(c.loc)
						seen = "last used " + client.FormatDay(t) + " " + client.FormatTime(t)
					}
					fmt.Fprintf(w, "%s  %s  (%s)\n", d.ID[:8], d.Name, seen)
				}
				return nil
			})
		},
	}
	revoke := &cobra.Command{
		Use: "revoke <device>", Short: "Unpair a phone, by the id `gwen remote devices` shows", Args: args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			list, err := c.api.RemoteDevices(cmd.Context())
			if err != nil {
				return err
			}
			var match []wire.RemoteDevice
			for _, d := range list.Devices {
				if strings.HasPrefix(d.ID, a[0]) {
					match = append(match, d)
				}
			}
			switch len(match) {
			case 0:
				return usagef("no paired phone starts with %q", a[0])
			case 1:
			default:
				return usagef("%q matches more than one phone; give more of the id", a[0])
			}
			if err := c.api.RevokeRemoteDevice(cmd.Context(), match[0].ID); err != nil {
				return err
			}
			fmt.Fprintf(c.stdout, "Unpaired %s.\n", match[0].Name)
			return nil
		},
	}
	ls := &cobra.Command{
		Use: "ls", Short: "List Claude Code sessions in Gwen Terminal", Args: args(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			list, err := c.api.CodeSessions(cmd.Context())
			if err != nil {
				return err
			}
			return c.emit(list, func(w io.Writer) error {
				if list.Core != "ok" && list.Message != "" {
					fmt.Fprintln(w, list.Message)
				}
				if len(list.Sessions) == 0 {
					fmt.Fprintln(w, "No Claude Code sessions. Start Claude in Gwen Terminal, or from your phone.")
					return nil
				}
				for _, s := range list.Sessions {
					line := fmt.Sprintf("%-10s %-20s %s", stateWords(s), s.Name, s.Cwd)
					if s.Step != "" {
						line += "  · " + s.Step
					}
					fmt.Fprintln(w, line)
				}
				return nil
			})
		},
	}
	hooks := &cobra.Command{
		Use: "hooks <on|off>", Short: "Let Gwen see your Claude Code sessions (edits ~/.claude/settings.json)", Args: args(1),
		RunE: func(cmd *cobra.Command, a []string) error {
			if a[0] != "on" && a[0] != "off" {
				return usagef("say on or off")
			}
			st, err := c.api.SetRemoteHooks(cmd.Context(), a[0] == "on")
			if err != nil {
				return err
			}
			return c.emit(st, func(w io.Writer) error { return printRemote(w, st) })
		},
	}
	remote.AddCommand(status, on, off, pair, devices, revoke, ls, hooks)
	return remote
}

func stateWords(s wire.CodeSession) string {
	switch s.State {
	case wire.CodeNeedsYou:
		return "Needs you"
	case wire.CodeWorking:
		return "Working"
	case wire.CodeDone:
		return "Done"
	case wire.CodeIdle:
		return "Idle"
	case wire.CodeError:
		return "Error"
	case wire.CodeEnded:
		return "Ended"
	}
	return "Starting"
}

// serve runs `tailscale serve` as root: Tailscale needs root to publish a
// Unix socket. In a terminal that is sudo; otherwise the desktop's own
// password prompt.
func (c *cli) serve(_ string, args ...string) error {
	tailscale, err := exec.LookPath("tailscale")
	if err != nil {
		return errors.New("tailscale isn't installed on this laptop")
	}
	argv := append([]string{tailscale, "serve"}, args...)
	// sudo needs a real terminal to ask for the password in; anywhere else
	// (a launcher, a script, an agent's shell) the desktop's prompt asks.
	elevate := "pkexec"
	if f, ok := c.stdin.(*os.File); ok {
		if _, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS); err == nil {
			elevate = "sudo"
		}
	}
	fmt.Fprintf(c.stderr, "Running: %s %s\n", elevate, strings.Join(argv, " "))
	cmd := exec.Command(elevate, argv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, c.stderr, c.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tailscale didn't accept that: %w", err)
	}
	return nil
}

func printRemote(w io.Writer, st *wire.RemoteStatus) error {
	yes := func(ok bool, good, bad string) string {
		if ok {
			return good
		}
		return bad
	}
	ts := map[string]string{"ok": "running", "missing": "not installed", "stopped": "not running", "logged_out": "signed out"}[st.Tailscale]
	fmt.Fprintf(w, "Phone remote: %s\n", yes(st.Enabled, "on", "off"))
	fmt.Fprintf(w, "Tailscale: %s\n", ts)
	if st.Host != "" {
		fmt.Fprintf(w, "HTTPS certificates: %s\n", yes(st.Certificates, "on", "off (turn them on at https://login.tailscale.com/admin/dns)"))
		fmt.Fprintf(w, "Published: %s\n", yes(st.Published, st.URL, "no (run: gwen remote on)"))
	}
	if st.Owner != "" {
		fmt.Fprintf(w, "Allowed account: %s\n", st.Owner)
	}
	fmt.Fprintf(w, "Paired phones: %d\n", st.Devices)
	core := map[string]string{"ok": "running " + st.CoreVersion, "down": "not running", "outdated": "out of date (run: make term)"}[st.Core]
	fmt.Fprintf(w, "Gwen Terminal: %s; core %s\n", yes(st.Terminal, "installed", "not installed (run: make term)"), core)
	hooks := yes(st.Hooks, "on", "off (run: gwen remote hooks on)")
	if st.HooksNote != "" {
		hooks += ". " + st.HooksNote
	}
	fmt.Fprintf(w, "Claude Code hooks: %s\n", hooks)
	sandbox := yes(st.Sandbox, "available", "needs bubblewrap")
	if st.Sandbox {
		sandbox += "; " + yes(st.SandboxSignedIn, "signed in", "not signed in (run: gwen sandbox-login)")
	}
	fmt.Fprintf(w, "Sandbox: %s\n", sandbox)
	fmt.Fprintf(w, "Voice: %s\n", yes(st.Voice, "ready", "not set up (Gwen › Settings › Voice)"))
	return nil
}
