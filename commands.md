# Gwen: build and run

Full guide: [setup.md](setup.md). Run everything from the repo root.

## Build

```sh
make build    # bin/gwend, bin/gwen, bin/gwen-tray, bin/gwen-ui (+ bin/lib voice libs)
make check    # gofmt, vet, staticcheck, go test -race, docs-check, tsc
make clean    # remove bin/ and dist/
```

## Dev install (what this machine uses now)

`make install-dev` builds, copies the binaries to `~/.local/bin`, writes
`~/.config/systemd/user/gwend.service`, and reinstalls the KDE panel widget.

```sh
make install-dev
systemctl --user restart gwend               # daemon picks up the new binary
pkill -x gwen-ui; setsid gwen-ui &           # restart the dashboard (a 2nd launch only focuses the old window)
systemctl --user restart plasma-plasmashell  # only if the panel widget (QML) changed
```

One-liner after code changes:

```sh
make install-dev && systemctl --user restart gwend && (pkill -x gwen-ui; setsid gwen-ui >/dev/null 2>&1 &)
```
! make install-dev
! systemctl --user restart plasma-plasmashell

## First run (once)

```sh
gwen setup                  # wizard: service, tracking, tray, phone (--yes accepts defaults)
gwen project add Internship
gwen in --project Internship
gwen status
gwen out
```

## Run the daemon in a terminal (readable logs)

```sh
systemctl --user stop gwend
bin/gwend --foreground
systemctl --user start gwend   # when done
```

## Status and logs

```sh
systemctl --user status gwend
journalctl --user -u gwend -e
tail -f ~/.local/state/gwen/gwend.log
```

## Package (rpm/deb)

```sh
make check
make package VERSION=0.1.3                # higher than installed (rpm -q gwen)
sudo dnf install ./dist/gwen-*.x86_64.rpm
systemctl --user restart gwend
pkill -x gwen-ui
```

The dev unit in `~/.config/systemd/user` overrides the package. See
[Moving from a dev install to the package](setup.md#moving-from-a-dev-install-to-the-package)
before switching.

## Raspberry Pi hub

```sh
make ui
make build-hub   # bin/arm64/gwend
scp bin/arm64/gwend pi@PI_ADDRESS:~
ssh pi@PI_ADDRESS 'sudo install -m 0755 ~/gwend /usr/local/bin/gwend && sudo systemctl restart gwen-hub'
```
