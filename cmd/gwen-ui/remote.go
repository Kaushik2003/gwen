package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/wire"
)

// Gwen on the phone, for Settings › Phone (plan.md 3.14).

func (a *App) CodeSessions() (*wire.CodeSessionList, error) { return call(a.api.CodeSessions(a.ctx)) }
func (a *App) StartCodeSession(req wire.StartCodeSession) (*wire.CodeSession, error) {
	return call(a.api.StartCodeSession(a.ctx, req))
}
func (a *App) RemoteStatus() (*wire.RemoteStatus, error) { return call(a.api.RemoteStatus(a.ctx)) }
func (a *App) RemotePairCode() (*wire.RemotePairCode, error) {
	return call(a.api.RemotePairCode(a.ctx))
}
func (a *App) RemoteDevices() (*wire.RemoteDevices, error) { return call(a.api.RemoteDevices(a.ctx)) }
func (a *App) RevokeRemoteDevice(id string) error {
	return wrap(a.api.RevokeRemoteDevice(a.ctx, id))
}
func (a *App) SetRemoteHooks(on bool) (*wire.RemoteStatus, error) {
	return call(a.api.SetRemoteHooks(a.ctx, on))
}

// PublishRemote turns Gwen on the phone on or off: it sets [remote] enabled
// and has Tailscale Serve publish, or stop publishing, the remote socket on
// the tailnet. Tailscale needs root to serve a Unix socket, so this asks for
// your password once through the desktop's own prompt; gwend itself never
// runs anything as root (plan.md 3.14.4).
func (a *App) PublishRemote(on bool) (*wire.RemoteStatus, error) {
	if on {
		if _, err := a.api.PatchConfig(a.ctx, wire.ConfigPatch{"remote": {"enabled": true}}); err != nil {
			return nil, wrap(err)
		}
	}
	st, err := a.api.RemoteStatus(a.ctx)
	if err != nil {
		return nil, wrap(err)
	}
	port := "--https=" + strconv.Itoa(st.Port)
	switch {
	case on && st.Tailscale != "ok":
		return st, hostFail("Tailscale isn't running on this laptop. Start it, then turn this on again.")
	case on && !st.Published:
		if err := serve(a.ctx, "--bg", port, "unix:"+st.Socket); err != nil {
			return st, err
		}
	case !on && st.Published:
		if err := serve(a.ctx, port, "off"); err != nil {
			return st, err
		}
	}
	if !on {
		if _, err := a.api.PatchConfig(a.ctx, wire.ConfigPatch{"remote": {"enabled": false}}); err != nil {
			return nil, wrap(err)
		}
	}
	return call(a.api.RemoteStatus(a.ctx))
}

func hostFail(msg string) error {
	return &hostError{Code: wire.CodeUnavailable, Message: msg, Details: map[string]any{}}
}

// serve runs `tailscale serve` as root through pkexec.
func serve(ctx context.Context, args ...string) error {
	tailscale, err := exec.LookPath("tailscale")
	if err != nil {
		return hostFail("Tailscale isn't installed on this laptop.")
	}
	if _, err := exec.LookPath("pkexec"); err != nil {
		return hostFail("Gwen can't ask for your password here. In a terminal, run: gwen remote on")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pkexec", append([]string{tailscale, "serve"}, args...)...).CombinedOutput()
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && (exit.ExitCode() == 126 || exit.ExitCode() == 127) {
		return hostFail("Nothing changed: the password prompt was closed.")
	}
	msg := strings.TrimSpace(string(out))
	if i := strings.LastIndex(msg, "\n"); i >= 0 {
		msg = msg[i+1:]
	}
	return hostFail(fmt.Sprintf("Tailscale didn't accept that: %s", msg))
}
