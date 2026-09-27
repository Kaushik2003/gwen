// Package ui is the dashboard's frontend: React and TypeScript built by Vite
// into dist/, which this package embeds for the GUI host to serve
// (docs/08-clients.md#gui).
package ui

import "embed"

// Assets holds the Vite build; paths start with dist/.
//
//go:embed all:dist
var Assets embed.FS
