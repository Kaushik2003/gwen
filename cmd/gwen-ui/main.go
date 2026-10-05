// Command gwen-ui is Gwen's dashboard: a Wails window over the React frontend
// in ui/, talking to the daemon through the same client as the CLI
// (docs/08-clients.md#gui). Closing it changes nothing about tracking.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/ui"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gwen-ui:", err)
		os.Exit(1)
	}
}

func run() error {
	paths, err := config.DefaultPaths()
	if err != nil {
		return err
	}
	assets, err := fs.Sub(ui.Assets, "dist")
	if err != nil {
		return err
	}
	exeDir := ""
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	api := client.New(paths.SocketPath())
	app := &App{api: api, screen: screenArg(os.Args[1:]), host: &host{
		api: api, clk: clock.Real(), run: runCommand, start: startCommand, output: outputCommand, lookPath: llm.FindCommand,
		configHome: paths.ConfigHome, shareDir: "/usr/share/gwen", exeDir: exeDir, credDir: paths.CredentialsDir(),
	}}
	return wails.Run(&options.App{
		Title:     "Gwen",
		Width:     1100,
		Height:    720,
		MinWidth:  900,
		MinHeight: 600,
		// The dashboard is always dark (ui/DESIGN.md); the window behind it
		// matches its canvas, so nothing flashes white while it loads.
		BackgroundColour: &options.RGBA{R: 1, G: 1, B: 2, A: 255},
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			app.ctx = ctx
			go forwardEvents(ctx, api)
		},
		Bind: []any{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "dev.gwen.ui",
			OnSecondInstanceLaunch: func(d options.SecondInstanceData) {
				runtime.WindowUnminimise(app.ctx)
				runtime.WindowShow(app.ctx)
				if screen := screenArg(d.Args); screen != "" {
					runtime.EventsEmit(app.ctx, "gwen:navigate", screen)
				}
			},
		},
		Linux: &linux.Options{ProgramName: "gwen-ui", WindowIsTranslucent: false},
	})
}

// forwardEvents re-emits every daemon event as "gwen:<name>" with its data,
// plus "gwen:connected" and "gwen:disconnected" as the stream comes and goes.
func forwardEvents(ctx context.Context, api client.API) {
	client.Follower{
		API:          api,
		Clock:        clock.Real(),
		OnConnect:    func() { runtime.EventsEmit(ctx, "gwen:connected") },
		OnEvent:      func(ev wire.Event) { runtime.EventsEmit(ctx, "gwen:"+ev.Name, decode(ev.Data)) },
		OnDisconnect: func(error) { runtime.EventsEmit(ctx, "gwen:disconnected") },
	}.Run(ctx)
}

func decode(raw json.RawMessage) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// screenArg is the screen named by --screen NAME or --screen=NAME, such as
// the tray's "assistant" or "inbox", or "".
func screenArg(args []string) string {
	for i, a := range args {
		switch {
		case a == "--screen" && i+1 < len(args):
			return args[i+1]
		case strings.HasPrefix(a, "--screen="):
			return strings.TrimPrefix(a, "--screen=")
		}
	}
	return ""
}
