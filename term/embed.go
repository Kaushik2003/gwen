// Package term holds Gwen's own files for Gwen Terminal, the fork of Herdr's
// app that lives in its own repository (plan.md 3.14.3): the herdr-core
// release it is built against, the entry `make term` merges into the
// personal programs.json, and the colours of Herdr's built-in themes, which
// the phone app draws with.
package term

import (
	_ "embed"
	"strings"
)

//go:embed themes.json
var themes []byte

//go:embed core.version
var coreVersion string

// Themes is themes.json: Herdr's built-in palettes by name.
func Themes() []byte { return themes }

// CoreVersion is the oldest herdr-core Gwen works with, e.g. "0.1.20".
func CoreVersion() string { return strings.TrimPrefix(strings.TrimSpace(coreVersion), "v") }
