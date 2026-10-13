// Package assets holds files Gwen ships inside her binaries.
package assets

import _ "embed"

// NudgeSound is the sound a desktop nudge plays.
//
//go:embed sounds/spider_man_ps4_tono.mp3
var NudgeSound []byte
