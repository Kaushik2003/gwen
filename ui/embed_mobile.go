package ui

import "embed"

// MobileAssets holds the phone app (npm run build:mobile), which the daemon
// serves to paired phones on the remote socket (plan.md 3.14.13); paths
// start with dist-mobile/.
//
//go:embed all:dist-mobile
var MobileAssets embed.FS
