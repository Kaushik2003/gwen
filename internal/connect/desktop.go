package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// MaxURLs is how many links one browser.open call opens.
const MaxURLs = 10

// openTimeout bounds one open: the portal's answer, or xdg-open.
const openTimeout = 20 * time.Second

// Opener opens a URL or a folder on this desktop. The default is the
// desktop portal for URLs, with xdg-open as the fallback, and xdg-open for
// folders; tests pass a fake.
type Opener interface {
	OpenURL(ctx context.Context, u string) error
	OpenFolder(ctx context.Context, path string) error
}

// Tracker starts tracking time on a project, or a task and its project.
type Tracker func(ctx context.Context, projectID, taskID string) (string, error)

// Desktop is the desktop connector: browser.open, folder.open, and
// tracking.start, all local.
func Desktop(open Opener, track Tracker, home string) Connector {
	if open == nil {
		open = SystemOpener{}
	}
	return Simple{IDValue: "desktop", NameValue: "This desktop", Caps: []Capability{
		{
			Name: "browser.open", Title: "Open links in your browser", Risk: Local,
			Input: `{"urls": ["https://…"]} (1 to 10 http or https links)`,
			Check: Checked(func(in *openURLs) error {
				if len(in.URLs) == 0 || len(in.URLs) > MaxURLs {
					return fmt.Errorf("give 1 to %d links", MaxURLs)
				}
				for i, u := range in.URLs {
					p, err := url.Parse(strings.TrimSpace(u))
					if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
						return fmt.Errorf("%q is not an http or https link", u)
					}
					in.URLs[i] = p.String()
				}
				return nil
			}),
			Summary: func(input json.RawMessage) string {
				var in openURLs
				_ = json.Unmarshal(input, &in)
				if len(in.URLs) == 1 {
					return "Open " + hostOf(in.URLs[0]) + " in your browser"
				}
				return fmt.Sprintf("Open %d links in your browser", len(in.URLs))
			},
			// One call per URL, and a failed one is reported, not hidden.
			Invoke: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
				var in openURLs
				if err := json.Unmarshal(input, &in); err != nil {
					return nil, err
				}
				type opened struct {
					URL    string `json:"url"`
					Opened bool   `json:"opened"`
					Error  string `json:"error,omitempty"`
				}
				out := struct {
					Links []opened `json:"links"`
				}{}
				failed := 0
				for _, u := range in.URLs {
					o := opened{URL: u, Opened: true}
					if err := open.OpenURL(ctx, u); err != nil {
						o.Opened, o.Error = false, err.Error()
						failed++
					}
					out.Links = append(out.Links, o)
				}
				res, _ := json.Marshal(out)
				switch {
				case failed == len(in.URLs):
					return res, errors.New("couldn't open the browser")
				case failed > 0:
					return res, fmt.Errorf("couldn't open %d of the %d links", failed, len(in.URLs))
				}
				return res, nil
			},
		},
		{
			Name: "folder.open", Title: "Open a folder in your file manager", Risk: Local,
			Input: `{"path": "~/dsa"} (an existing folder in your home)`,
			Check: Checked(func(in *openFolder) error {
				p, err := folderIn(home, in.Path)
				in.Path = p
				return err
			}),
			Summary: func(input json.RawMessage) string {
				var in openFolder
				_ = json.Unmarshal(input, &in)
				return "Open " + tilde(home, in.Path) + " in your file manager"
			},
			Scope: func(input json.RawMessage) string {
				var in openFolder
				_ = json.Unmarshal(input, &in)
				return in.Path
			},
			Invoke: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
				var in openFolder
				if err := json.Unmarshal(input, &in); err != nil {
					return nil, err
				}
				if err := open.OpenFolder(ctx, in.Path); err != nil {
					return nil, fmt.Errorf("couldn't open the folder: %w", err)
				}
				return json.Marshal(map[string]any{"opened": in.Path})
			},
		},
		{
			Name: "tracking.start", Title: "Start tracking time", Risk: Local,
			Input: `{"project": "<project id>"} or {"task": "<task id>"}`,
			Check: Checked(func(in *startTracking) error {
				if (in.Project == "") == (in.Task == "") {
					return errors.New("give a project or a task")
				}
				return nil
			}),
			Summary: func(input json.RawMessage) string { return "Start tracking time" },
			Invoke: func(ctx context.Context, input json.RawMessage) (json.RawMessage, error) {
				var in startTracking
				if err := json.Unmarshal(input, &in); err != nil {
					return nil, err
				}
				if track == nil {
					return nil, errors.New("tracking isn't running on this device")
				}
				on, err := track(ctx, in.Project, in.Task)
				if err != nil {
					return nil, err
				}
				return json.Marshal(map[string]any{"tracking": on})
			},
		},
	}}
}

type openURLs struct {
	URLs []string `json:"urls"`
}

type openFolder struct {
	Path string `json:"path"`
}

type startTracking struct {
	Project string `json:"project,omitempty"`
	Task    string `json:"task,omitempty"`
}

func hostOf(u string) string {
	if p, err := url.Parse(u); err == nil && p.Host != "" {
		return p.Host
	}
	return "a link"
}

// folderIn is path as an existing folder inside home, with ~ expanded.
func folderIn(home, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "~" || strings.HasPrefix(path, "~/") {
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("%q is not a full path", path)
	}
	path = filepath.Clean(path)
	if home != "" && path != home && !strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "", errors.New("only folders in your home can be opened")
	}
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("there is no folder %s", tilde(home, path))
	}
	return path, nil
}

func tilde(home, path string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

// SystemOpener opens URLs through the desktop portal's OpenURI and waits for
// its answer, falling back to xdg-open when there is no portal; folders open
// with xdg-open.
type SystemOpener struct{}

// OpenURL opens one link in the default browser.
func (SystemOpener) OpenURL(ctx context.Context, u string) error {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	err := portalOpen(ctx, u)
	if errors.Is(err, errNoPortal) {
		return xdgOpen(ctx, u)
	}
	return err
}

// OpenFolder opens a folder in the file manager.
func (SystemOpener) OpenFolder(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	return xdgOpen(ctx, path)
}

var errNoPortal = errors.New("no desktop portal")

// portalOpen asks org.freedesktop.portal.OpenURI to open u and waits for
// its Response: 0 opened, 1 you cancelled, 2 it failed.
func portalOpen(ctx context.Context, u string) error {
	conn, err := dbus.ConnectSessionBus(dbus.WithContext(ctx))
	if err != nil {
		return errNoPortal
	}
	defer conn.Close()
	token := fmt.Sprintf("gwen%d", time.Now().UnixNano())
	sender := strings.ReplaceAll(strings.TrimPrefix(conn.Names()[0], ":"), ".", "_")
	handle := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)
	// Listen before asking, so the answer can't come first.
	if err := conn.AddMatchSignal(dbus.WithMatchObjectPath(handle), dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response")); err != nil {
		return errNoPortal
	}
	signals := make(chan *dbus.Signal, 4)
	conn.Signal(signals)
	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	var got dbus.ObjectPath
	call := obj.CallWithContext(ctx, "org.freedesktop.portal.OpenURI.OpenURI", 0, "", u,
		map[string]dbus.Variant{"handle_token": dbus.MakeVariant(token)})
	if call.Err != nil {
		var derr dbus.Error
		if errors.As(call.Err, &derr) && (derr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || derr.Name == "org.freedesktop.DBus.Error.UnknownMethod") {
			return errNoPortal
		}
		return fmt.Errorf("the desktop refused: %w", call.Err)
	}
	if err := call.Store(&got); err != nil {
		return errNoPortal
	}
	for {
		select {
		case <-ctx.Done():
			return errors.New("the desktop didn't answer")
		case sig := <-signals:
			if sig == nil || (sig.Path != handle && sig.Path != got) || len(sig.Body) == 0 {
				continue
			}
			switch code, _ := sig.Body[0].(uint32); code {
			case 0:
				return nil
			case 1:
				return errors.New("you cancelled it")
			default:
				return errors.New("the desktop couldn't open it")
			}
		}
	}
}

func xdgOpen(ctx context.Context, target string) error {
	out, err := exec.CommandContext(ctx, "xdg-open", target).CombinedOutput()
	if err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%s", firstLine(msg))
		}
		return err
	}
	return nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
