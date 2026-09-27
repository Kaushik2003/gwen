// Package extidle holds the Go bindings for the ext-idle-notify-v1 Wayland
// protocol, version 2, generated from the vendored protocol/ext-idle-notify-v1.xml.
// Regenerate with go generate; never edit ext_idle_notify.go by hand.
package extidle

//go:generate go tool go-wayland-scanner -pkg extidle -prefix ext -suffix v1 -i ../../../protocol/ext-idle-notify-v1.xml -o ext_idle_notify.go
