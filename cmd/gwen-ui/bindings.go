//go:build bindings

package main

// bindingsBuild is true in the build `wails build` runs to write the
// frontend's bindings. That run has no flags, and must reach wails.Run,
// which writes them and exits, instead of opening the Gwen window.
const bindingsBuild = true
