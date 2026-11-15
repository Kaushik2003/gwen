# Gwen build. The targets are specified in docs/09-packaging.md#build.

GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
ifeq ($(strip $(VERSION)),)
VERSION := 0.0.0-dev
endif
# rpm and deb need a version that starts X.Y.Z; untagged builds package as 0.0.0.
PKG_VERSION := $(shell echo '$(VERSION)' | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+' && echo '$(VERSION)' || echo 0.0.0)
LDFLAGS := -X main.version=$(VERSION)
# The GUI links WebKitGTK 4.1, so every go command that loads cmd/gwen-ui needs
# this tag. vet and staticcheck also cover the manual tests.
TAGS := webkit2_41

CONFIG_HOME := $(or $(XDG_CONFIG_HOME),$(HOME)/.config)
DATA_HOME := $(or $(XDG_DATA_HOME),$(HOME)/.local/share)

# Voice input links sherpa-onnx and ONNX Runtime, which ship beside the
# binaries in lib/gwen: /usr/lib/gwen, or ~/.local/lib/gwen after install-dev.
SHERPA_LIB = $(shell $(GO) list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-linux)/lib/x86_64-unknown-linux-gnu
VOICE_LIBS := libsherpa-onnx-c-api.so libonnxruntime.so

.PHONY: build ui ui-fresh build-hub check docs-check package install-dev clean ui-stub term term-update

build: ui-fresh
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/ ./cmd/gwend ./cmd/gwen ./cmd/gwen-tray
	cd cmd/gwen-ui && CGO_LDFLAGS='-Wl,-rpath,$$ORIGIN/../lib/gwen' $(GO) tool wails build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o gwen-ui
	cp cmd/gwen-ui/build/bin/gwen-ui bin/gwen-ui
	CGO_LDFLAGS='-Wl,-rpath,$$ORIGIN/../lib/gwen' $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/gwen-stt ./cmd/gwen-stt
	@mkdir -p bin/lib
	cd '$(SHERPA_LIB)' && install -m 0644 $(VOICE_LIBS) $(CURDIR)/bin/lib/

ui:
	cd ui && npm ci && npm run build && npm run build:mobile

# Every web part a build embeds, made fresh so an install never ships an old copy: the npm packages when
# package-lock.json changed, and the phone app and hub page, which gwend embeds. The Gwen window's ui/dist is
# built by Wails itself (cmd/gwen-ui/wails.json), with the type check.
ui-fresh: ui-stub
	@if [ ! -d ui/node_modules ] || [ ui/package-lock.json -nt ui/node_modules/.package-lock.json ]; then \
		(cd ui && npm ci); \
	fi
	cd ui && npm run build:mobile && npm run build:hub

# Gwen Terminal: gwen-term, the fork of Herdr's app in its own checkout
# ($(TERM_DIR), default ~/Documents/coding/gwen-term), and herdr-core at the
# release in term/core.version. term builds and installs both; term-update
# rebases the fork on Herdr and runs Herdr's checks (plan.md 3.14.3).
term:
	@scripts/term.sh build

term-update:
	@scripts/term.sh update

build-hub: ui-stub
	@mkdir -p bin/arm64
	@if [ -d ui/node_modules ]; then cd ui && npm run build:hub; fi
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/arm64/gwend ./cmd/gwend

check: ui-stub
	@unformatted=$$(find . -name '*.go' -not -path './.*' -not -path './ui/node_modules/*' -print0 | xargs -0 -r gofmt -s -l); \
	if [ -n "$$unformatted" ]; then echo "gofmt -s would change:"; echo "$$unformatted"; exit 1; fi
	$(GO) vet -tags $(TAGS),manual ./...
	$(GO) tool staticcheck -tags $(TAGS),manual ./...
	$(GO) test -race -tags $(TAGS) ./...
	@if [ -d ui/node_modules ]; then cd ui && npx tsc --noEmit; fi

docs-check:
	@scripts/docs-check.sh

package: build
	@mkdir -p dist
	@# Gwen's terminals go in the package: herdr-core and Gwen Terminal, built when missing or not at the pinned release.
	@scripts/term.sh stage $(CURDIR)/bin
	rm -f dist/*.rpm dist/*.deb dist/SHA256SUMS
	VERSION=$(PKG_VERSION) $(GO) tool nfpm package --config packaging/nfpm.yaml --packager rpm --target dist/
	VERSION=$(PKG_VERSION) $(GO) tool nfpm package --config packaging/nfpm.yaml --packager deb --target dist/
	cd dist && sha256sum *.rpm *.deb > SHA256SUMS

install-dev: build
	install -d $(HOME)/.local/bin $(CONFIG_HOME)/systemd/user
	install -m 0755 bin/gwend bin/gwen bin/gwen-tray bin/gwen-ui bin/gwen-stt $(HOME)/.local/bin/
	install -d $(HOME)/.local/lib/gwen
	install -m 0644 $(addprefix bin/lib/,$(VOICE_LIBS)) $(HOME)/.local/lib/gwen/
	sed 's|^ExecStart=.*|ExecStart=%h/.local/bin/gwend|' packaging/gwend.service > $(CONFIG_HOME)/systemd/user/gwend.service
	systemctl --user daemon-reload
	@# One "Gwen" in the launcher: it opens the Gwen window (gwen-ui) on Today (plan.md 3.15).
	@# This copy also hides an older packaged entry of the same name, and Gwen Terminal's own entry.
	install -d $(DATA_HOME)/applications
	sed -e 's|^Exec=gwen |Exec=$(HOME)/.local/bin/gwen |' packaging/gwen.desktop > $(DATA_HOME)/applications/gwen.desktop
	@if [ -f $(DATA_HOME)/applications/gwen-term.desktop ] && ! grep -q '^NoDisplay=true' $(DATA_HOME)/applications/gwen-term.desktop; then \
		echo "NoDisplay=true" >> $(DATA_HOME)/applications/gwen-term.desktop; \
	fi
	-update-desktop-database $(DATA_HOME)/applications 2>/dev/null
	@if command -v kpackagetool6 >/dev/null; then \
		kpackagetool6 -t Plasma/Applet -u packaging/plasma/dev.gwen.panel 2>/dev/null || kpackagetool6 -t Plasma/Applet -i packaging/plasma/dev.gwen.panel; \
	fi
	@# Gwen's terminals: herdr-core and Gwen Terminal, built only when missing or not at the pinned release.
	@scripts/term.sh ensure
	@# A running gwend switches to the new build now; a stopped one stays stopped.
	systemctl --user try-restart gwend
	@echo "Installed. Close and reopen the Gwen window to use its new build."

clean:
	rm -rf bin dist

# Package ui embeds ui/dist, ui/dist-mobile and ui/dist-hub, which `make ui`, `make build`
# (ui-fresh and Wails) and `make build-hub` build. Until then placeholder pages let every
# Go package compile.
ui-stub:
	@if [ -f ui/embed.go ] && [ ! -e ui/dist/index.html ]; then \
		mkdir -p ui/dist && \
		printf '<!doctype html><title>Gwen</title><p>Built without the dashboard. Run make ui.</p>\n' > ui/dist/index.html; \
	fi
	@if [ -f ui/embed_hub.go ] && [ ! -e ui/dist-hub/index.html ]; then \
		mkdir -p ui/dist-hub && \
		printf '<!doctype html><title>Gwen</title><p>Built without the dashboard. Run make build-hub.</p>\n' > ui/dist-hub/index.html; \
	fi
	@if [ -f ui/embed_mobile.go ] && [ ! -e ui/dist-mobile/index.html ]; then \
		mkdir -p ui/dist-mobile && \
		printf '<!doctype html><title>Gwen</title><p>Built without the phone app. Run make ui.</p>\n' > ui/dist-mobile/index.html; \
	fi
