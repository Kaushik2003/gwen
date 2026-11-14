package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/kzark/gwen/internal/agent"
	"github.com/kzark/gwen/internal/api"
	"github.com/kzark/gwen/internal/notify"
	"github.com/kzark/gwen/internal/store"
	"github.com/kzark/gwen/internal/wire"
)

// kindAgentApproval is the notification that an approval of Gwen's agent waits.
const kindAgentApproval = "agent_approval"

// agentSocketName is the socket `gwen mcp` reaches Gwen's tools on (plan.md
// 3.16). It serves those tools and nothing else.
const agentSocketName = "agent.sock"

func agentSocketPath(o options) string {
	return filepath.Join(filepath.Dir(o.socketPath), agentSocketName)
}

// newAgent builds Gwen's agent over the API server, which gives it her
// tools, the context note, and the chats.
func newAgent(ctx context.Context, repos store.Repos, srv *api.Server, l *loop, hub *api.Hub, o options) (*agent.Manager, error) {
	home, _ := os.UserHomeDir()
	gwen := sibling("gwen")
	if gwen == "" {
		gwen = filepath.Join(home, ".local", "bin", "gwen")
	}
	m := agent.New(agent.Options{Repos: repos, Approvals: srv.Harness.Approvals(), Sources: srv, Publish: hub.Publish,
		Config: l.Config, Home: home, Runtime: filepath.Dir(o.socketPath), DataDir: o.dataDir, GwenBin: gwen,
		AgentSocket: agentSocketPath(o),
		Notify: func(thread, title, body string) {
			// The button names the chat, so Open shows the one that waits.
			l.notifier.Send(notify.Notification{Kind: kindAgentApproval, Title: title, Body: body,
				Actions: []notify.Action{{ID: "open:" + thread, Label: "Open"}}})
			// And the phone, while you are away from the laptop. Only the
			// place goes out: the ntfy fallback is not private, so never
			// the command or the text she wants to send.
			if srv.Code != nil {
				srv.Code.PushPhone(title, "Open Gwen to allow or deny it.", "/", "approval:"+thread)
			}
		}})
	// Gwen follows the Claude sessions her chats started.
	hub.OnPublish(func(name string) {
		if name == wire.EventCodeSession {
			go m.SessionsChanged()
		}
	})
	// Open on the notification shows the chat in Gwen › Chat, where the
	// approval waits.
	l.openAgent = func(ctx context.Context, thread string) {
		if srv.Code == nil {
			return
		}
		target := "gwen"
		if thread != "" {
			target = "agent:" + thread
		}
		_ = srv.Code.OpenWindow(ctx, target)
	}
	return m, m.Start(ctx)
}

// serveAgent listens on agent.sock, mode 0600 like the other sockets.
func serveAgent(srv *api.Server, o options) (*http.Server, error) {
	path := agentSocketPath(o)
	os.Remove(path)
	ln, err := listenUnix(path)
	if err != nil {
		return nil, err
	}
	return serveOn(ln, srv.AgentHandler()), nil
}
