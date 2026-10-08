package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// kwinRule is the KWin window rule that keeps the now card above other
// windows, off the taskbar and without a title bar, and lets it take focus,
// so clicking away closes it. Where it opens is nowCardScript's job.
const kwinRule = "gwen-now-card"

// nowCardScript is the KWin script that drops the card down from the tray
// icon. Loaded at runtime, it lasts as long as the KWin session, as the tray
// does.
//
//go:embed nowcard.js
var nowCardScript []byte

var kwinRuleKeys = [][2]string{
	{"Description", "Gwen now card"},
	{"title", "Gwen · now"},
	{"titlematch", "1"}, // exact
	{"wmclass", "gwen-ui"},
	{"wmclassmatch", "2"}, // substring
	{"types", "1"},        // normal windows
	{"above", "true"},
	{"aboverule", "2"},
	{"skiptaskbar", "true"},
	{"skiptaskbarrule", "2"},
	{"skippager", "true"},
	{"skippagerrule", "2"},
	{"skipswitcher", "true"},
	{"skipswitcherrule", "2"},
	{"fsplevel", "0"}, // no focus stealing prevention
	{"fsplevelrule", "2"},
	// GTK asks for no title bar, but KWin draws its own around a Wayland
	// window unless told not to.
	{"noborder", "true"},
	{"noborderrule", "2"},
}

// commander runs a program to the end and returns what it printed.
type commander func(name string, args ...string) (string, error)

func runOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).Output()
	return string(out), err
}

// placeNowCard installs or refreshes kwinRule on KDE Plasma, keeping the
// user's other rules, has KWin reload its rules, and loads nowCardScript from
// dir. Elsewhere it does nothing.
func placeNowCard(desktop, dir string, run commander) error {
	if !isKDE(desktop) {
		return nil
	}
	write := func(group, key, value string) error {
		_, err := run("kwriteconfig6", "--file", "kwinrulesrc", "--group", group, "--key", key, value)
		return err
	}
	for _, kv := range kwinRuleKeys {
		if err := write(kwinRule, kv[0], kv[1]); err != nil {
			return err
		}
	}
	// Placement keys an earlier version wrote; KWin passed them by.
	for _, key := range []string{"placement", "placementrule"} {
		if _, err := run("kwriteconfig6", "--file", "kwinrulesrc", "--group", kwinRule, "--key", key, "--delete"); err != nil {
			return err
		}
	}
	out, err := run("kreadconfig6", "--file", "kwinrulesrc", "--group", "General", "--key", "rules")
	if err != nil {
		return err
	}
	var rules []string
	for r := range strings.SplitSeq(strings.TrimSpace(out), ",") {
		if r != "" {
			rules = append(rules, r)
		}
	}
	if !slices.Contains(rules, kwinRule) {
		rules = append(rules, kwinRule)
		if err := write("General", "rules", strings.Join(rules, ",")); err != nil {
			return err
		}
		if err := write("General", "count", strconv.Itoa(len(rules))); err != nil {
			return err
		}
	}
	if _, err := kwin(run, "/KWin", "org.kde.KWin.reconfigure"); err != nil {
		return err
	}
	return loadNowCardScript(dir, run)
}

var scriptID = regexp.MustCompile(`\((-?\d+),\)`)

// loadNowCardScript writes nowCardScript to dir and has KWin run it, in place
// of the copy an earlier tray loaded.
func loadNowCardScript(dir string, run commander) error {
	path := filepath.Join(dir, "now-card.js")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, nowCardScript, 0o600); err != nil {
		return err
	}
	if _, err := kwin(run, "/Scripting", "org.kde.kwin.Scripting.unloadScript", kwinRule); err != nil {
		return err
	}
	out, err := kwin(run, "/Scripting", "org.kde.kwin.Scripting.loadScript", path, kwinRule)
	if err != nil {
		return err
	}
	m := scriptID.FindStringSubmatch(out)
	if m == nil || m[1] == "-1" {
		return fmt.Errorf("KWin did not load the now card script: %s", strings.TrimSpace(out))
	}
	_, err = kwin(run, "/Scripting/Script"+m[1], "org.kde.kwin.Script.run")
	return err
}

// kwin calls a method of KWin over the session bus.
func kwin(run commander, object, method string, args ...string) (string, error) {
	return run("gdbus", append([]string{"call", "--session", "--dest", "org.kde.KWin", "--object-path", object, "--method", method}, args...)...)
}

func currentDesktop() string { return os.Getenv("XDG_CURRENT_DESKTOP") }

// isKDE reports whether desktop, an XDG_CURRENT_DESKTOP, is KDE Plasma.
func isKDE(desktop string) bool { return slices.Contains(strings.Split(desktop, ":"), "KDE") }

// widgetInPanel reports whether the Gwen panel widget
// (packaging/plasma/dev.gwen.panel) is on a KDE Plasma panel. It shows the
// timer and holds everything the tray does, so the tray steps aside rather
// than show Gwen twice.
func widgetInPanel(desktop, configHome string) bool {
	if !isKDE(desktop) {
		return false
	}
	b, err := os.ReadFile(filepath.Join(configHome, "plasma-org.kde.plasma.desktop-appletsrc"))
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(b)) {
		if strings.TrimSpace(line) == "plugin=dev.gwen.panel" {
			return true
		}
	}
	return false
}
