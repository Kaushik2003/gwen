// Command gwen-ui is the Gwen app (plan.md 3.15): with no flag the Gwen
// window, which holds every screen and the workbench (projects, terminals
// from Gwen's herdr-core, files, and history); with --now the tray's now
// card, and with --talk the Talk window. All are Wails windows over the
// React frontend in ui/, talking to the daemon through the same client as
// the CLI. `--open TARGET` (or the older `--screen NAME`) shows a screen, a
// project, a session, or a terminal; a second launch hands it to the window
// already open. `gwen-ui host ...` answers Settings about voice without a
// window.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kzark/gwen/internal/client"
	"github.com/kzark/gwen/internal/clock"
	"github.com/kzark/gwen/internal/config"
	"github.com/kzark/gwen/internal/llm"
	"github.com/kzark/gwen/internal/voice"
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
	// `gwen-ui host ...` answers Settings › AI about voice without a window.
	if len(os.Args) > 1 && os.Args[1] == "host" {
		os.Exit(runHost(os.Args[2:], os.Stdout, os.Stderr))
	}
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
	app := &App{api: api, screen: screenArg(os.Args[1:]), nowCard: slices.Contains(os.Args[1:], "--now"), talk: slices.Contains(os.Args[1:], "--talk"), host: &host{
		api: api, clk: clock.Real(), run: runCommand, start: startCommand, output: outputCommand, lookPath: llm.FindCommand,
		configHome: paths.ConfigHome, shareDir: "/usr/share/gwen", exeDir: exeDir, credDir: paths.CredentialsDir(),
	}}
	emit := func(name string, data any) { runtime.EventsEmit(app.ctx, name, data) }
	app.voice = newDictation(filepath.Join(paths.DataDir, "models"), emit)
	app.speech = newSpeech(filepath.Join(paths.DataDir, "models"), paths.CredentialsDir(), speechChoices(paths), app.host.hasCredential, emit)
	if app.nowCard {
		return wails.Run(nowCardOptions(app, api, assets))
	}
	if app.talk {
		return wails.Run(talkOptions(app, api, assets))
	}
	return wails.Run(mainOptions(app, api, assets))
}

// openGwen starts this program as the Gwen window on a target; when the
// window is already open, the new process hands it the target and exits.
func openGwen(target string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if target == "" {
		return startCommand(exe)
	}
	return startCommand(exe, "--open", target)
}

// mainOptions is the Gwen window (plan.md 3.15). It has one instance: a
// second launch hands its --open target to this one, which shows it.
func mainOptions(app *App, api client.API, assets fs.FS) *options.App {
	return &options.App{
		Title:            "Gwen",
		Width:            1440,
		Height:           900,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: &options.RGBA{R: 13, G: 16, B: 18, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets, Middleware: rawFiles},
		OnStartup: func(ctx context.Context) {
			app.ctx = ctx
			go forwardEvents(ctx, api)
			// The terminal bridge (term.go) follows Gwen's herdr-core while the window is open.
			app.startTerminals(ctx)
		},
		OnShutdown: func(context.Context) {
			app.voice.cancel()
			app.speech.stop()
		},
		Bind: []any{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "dev.gwen.ui",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				runtime.WindowUnminimise(app.ctx)
				runtime.WindowShow(app.ctx)
				if target := screenArg(data.Args); target != "" {
					runtime.EventsEmit(app.ctx, "gwen:navigate", target)
				}
			},
		},
		Linux: &linux.Options{ProgramName: "gwen-ui", WindowIsTranslucent: false},
	}
}

// newDictation is voice input with its models under root. The model is freed
// after three minutes without use.
func newDictation(root string, emit func(name string, data any)) *dictation {
	return &dictation{
		root: root, lookPath: exec.LookPath, http: &http.Client{}, emit: emit,
		engine: &voice.Engine{Root: root, Idle: 3 * time.Minute},
		record: func(level func(float64)) (voice.Source, error) {
			rec, err := voice.Record(voice.Recorder, level)
			if err != nil {
				return nil, err
			}
			return rec, nil
		},
		detect: func() (voice.Detector, error) { return voice.NewVAD(root) },
	}
}

// newSpeech is Gwen's voice, with models under root, API keys in credDir, and
// the provider and voices chosen in Settings in the file choices.
func newSpeech(root, credDir, choices string, hasKey func(string) bool, emit func(name string, data any)) *speech {
	hc := &http.Client{}
	return &speech{
		root: root, lookPath: exec.LookPath, hasKey: hasKey, http: hc, emit: emit, choices: choices,
		providers: speechProviders(root, credDir, hc),
		play: func() (sink, error) {
			p, err := voice.Play(voice.Player, voice.SpeechRate)
			if err != nil {
				return nil, err
			}
			return p, nil
		},
	}
}

// speechChoices is the file that keeps the provider and voices chosen in
// Settings, beside Gwen's config.
func speechChoices(paths config.Paths) string {
	return filepath.Join(filepath.Dir(paths.ConfigFile), "speech.json")
}

// nowCardOptions is the now card the tray opens on a click: a small frameless
// window with the day at a glance. It has its own instance lock, so a second
// click closes it instead of opening another.
func nowCardOptions(app *App, api client.API, assets fs.FS) *options.App {
	return &options.App{
		Title:            "Gwen · now",
		Width:            420,
		Height:           346,
		DisableResize:    true,
		Frameless:        true,
		AlwaysOnTop:      true,
		BackgroundColour: &options.RGBA{R: 1, G: 1, B: 2, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			app.ctx = ctx
			go forwardEvents(ctx, api)
		},
		Bind: []any{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "dev.gwen.ui.now",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { runtime.Quit(app.ctx) },
		},
		Linux: &linux.Options{ProgramName: "gwen-ui", WindowIsTranslucent: false},
	}
}

// talkOptions is the talk window the panel widget opens: a frameless window
// that floats over the desktop with Gwen large, listening as soon as it opens.
// Hanging up closes it; opening it again while it is up brings it back.
func talkOptions(app *App, api client.API, assets fs.FS) *options.App {
	return &options.App{
		Title:            "Talk to Gwen",
		Width:            400,
		Height:           620,
		MinWidth:         340,
		MinHeight:        480,
		Frameless:        true,
		AlwaysOnTop:      true,
		BackgroundColour: &options.RGBA{R: 1, G: 1, B: 2, A: 255},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup: func(ctx context.Context) {
			app.ctx = ctx
			go forwardEvents(ctx, api)
		},
		// However the window closes, the mic and her voice stop with it.
		OnShutdown: func(context.Context) {
			app.voice.cancel()
			app.speech.stop()
		},
		Bind: []any{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "dev.gwen.ui.talk",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				runtime.WindowUnminimise(app.ctx)
				runtime.WindowShow(app.ctx)
			},
		},
		Linux: &linux.Options{ProgramName: "gwen-ui", WindowIsTranslucent: false},
	}
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

// screenArg is the target named by --open TARGET, --open=TARGET, or the
// older --screen NAME: a screen such as "inbox" or "settings/phone",
// "project:<id>", "session:<id>", or "terminal:<address>"; "" for none.
func screenArg(args []string) string {
	for i, a := range args {
		for _, flag := range []string{"--open", "--screen"} {
			switch {
			case a == flag && i+1 < len(args):
				return args[i+1]
			case strings.HasPrefix(a, flag+"="):
				return strings.TrimPrefix(a, flag+"=")
			}
		}
	}
	return ""
}
