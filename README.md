# Gwen

A personal time tracker and study planner for Linux. It clocks your working day, notices when you
walk away, nudges you back on your desktop and phone, and turns long-term goals into daily plans.

- Specification: [docs/README.md](docs/README.md). Start there.
- Work packages for implementing agents: [docs/10-build-plan.md](docs/10-build-plan.md).

## Requirements

- Go 1.26 and GNU make. Every other Go tool (staticcheck, wails, nfpm, the Wayland scanner) is
  pinned in `go.mod` and runs with `go tool`.
- For the GUI: Node.js 24 with npm, and the GTK 3 and WebKitGTK 4.1 development headers
  (`sudo dnf install gtk3-devel webkit2gtk4.1-devel` on Fedora,
  `sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev` on Debian and Ubuntu).

## Build

```sh
make build        # bin/gwend, bin/gwen, bin/gwen-tray, and bin/gwen-ui
make build-hub    # bin/arm64/gwend for the Raspberry Pi sync hub
make package      # rpm and deb in dist/, with dist/SHA256SUMS
```

`VERSION=1.2.3` overrides the version stamped into the binaries, which otherwise comes from
`git describe`.

## Install and run

From a package, install the rpm or deb, then run `gwen setup` or open **Gwen** from the app menu.

From source, for development:

```sh
make install-dev                    # binaries to ~/.local/bin, user unit to ~/.config/systemd/user
gwen setup                          # enables the daemon at login, the tray, and phone push
gwen project add Internship
gwen in --project Internship        # clock in
gwen status
```

`gwend --foreground` runs the daemon in a terminal with readable logs on stderr.

## Checks

```sh
make check        # gofmt, go vet, staticcheck, go test -race, docs-check, and tsc for the GUI
make docs-check   # the specification's change protocol only
```
