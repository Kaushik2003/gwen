package remote

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kzark/gwen/internal/wire"
	"github.com/kzark/gwen/term"
)

// palette is a Herdr theme: a terminal palette, from which the app derives
// the colours of everything around the terminal.
type palette struct {
	Background string   `json:"background"`
	Foreground string   `json:"foreground"`
	Selection  string   `json:"selection"`
	Cursor     string   `json:"cursor"`
	ANSI       []string `json:"ansi"`
}

// defaultTheme is the theme Gwen Terminal shows when none is chosen.
const defaultTheme = "graphite"

// Theme is Gwen Terminal's active theme, so the phone looks like the laptop
// (plan.md 3.14.13): the theme named in its settings.json, read from its
// themes folder, or from Herdr's built-in ones.
func (s *Service) Theme() wire.CodeTheme {
	home := termHome(s.home)
	name := defaultTheme
	if data, err := os.ReadFile(filepath.Join(home, "settings.json")); err == nil {
		var st struct {
			Theme json.RawMessage `json:"theme"`
		}
		if json.Unmarshal(data, &st) == nil && len(st.Theme) > 0 {
			var one string
			var pair struct {
				Dark string `json:"dark"`
			}
			if json.Unmarshal(st.Theme, &one) == nil && one != "" {
				name = one
			} else if json.Unmarshal(st.Theme, &pair) == nil && pair.Dark != "" {
				name = pair.Dark
			}
		}
	}
	var builtin map[string]palette
	_ = json.Unmarshal(term.Themes(), &builtin)
	p, ok := builtin[name]
	if data, err := os.ReadFile(filepath.Join(home, "themes", filepath.Base(name)+".json")); err == nil {
		var file palette
		if json.Unmarshal(data, &file) == nil && validPalette(file) {
			p, ok = file, true
		}
	}
	if !ok || !validPalette(p) {
		name, p = defaultTheme, builtin[defaultTheme]
	}
	if p.Cursor == "" {
		p.Cursor = p.Foreground
	}
	return wire.CodeTheme{
		Name: name, Dark: luminance(p.Background) < 0.5,
		Background: p.Background, Foreground: p.Foreground, Cursor: p.Cursor, Selection: p.Selection,
		Accent:  p.ANSI[4],
		Surface: mix(p.Background, p.Foreground, 0.05),
		Border:  mix(p.Background, p.Foreground, 0.13),
		Muted:   mix(p.Background, p.Foreground, 0.55),
		ANSI:    p.ANSI,
	}
}

func validPalette(p palette) bool {
	if len(p.ANSI) != 16 {
		return false
	}
	for _, c := range append([]string{p.Background, p.Foreground, p.Selection}, p.ANSI...) {
		if _, ok := rgb(c); !ok {
			return false
		}
	}
	return true
}

func rgb(hex string) ([3]float64, bool) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return [3]float64{}, false
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return [3]float64{}, false
	}
	return [3]float64{float64(n >> 16 & 0xff), float64(n >> 8 & 0xff), float64(n & 0xff)}, true
}

// mix moves colour a toward b by t.
func mix(a, b string, t float64) string {
	ca, _ := rgb(a)
	cb, _ := rgb(b)
	var out [3]int
	for i := range 3 {
		out[i] = int(ca[i] + (cb[i]-ca[i])*t + 0.5)
	}
	return fmt.Sprintf("#%02x%02x%02x", out[0], out[1], out[2])
}

func luminance(hex string) float64 {
	c, _ := rgb(hex)
	return (0.2126*c[0] + 0.7152*c[1] + 0.0722*c[2]) / 255
}
