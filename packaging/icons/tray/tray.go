// Package tray embeds the tray icons, one per engine state group, drawn by
// gen.go at 22 and 44 px (docs/08-clients.md#tray).
package tray

import (
	"embed"
	"fmt"
)

//go:embed *.png
var icons embed.FS

// The icon names.
const (
	Off     = "off"
	Working = "working"
	Idle    = "idle"
	Break   = "break"
)

// Icon returns the PNG for an icon name at 22 or 44 px.
func Icon(name string, px int) ([]byte, error) {
	return icons.ReadFile(fmt.Sprintf("%s-%d.png", name, px))
}
