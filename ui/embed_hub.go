package ui

import "embed"

// HubAssets holds the hub's read-only build (npm run build:hub), which the
// sync hub serves at /; paths start with dist-hub/.
//
//go:embed all:dist-hub
var HubAssets embed.FS
