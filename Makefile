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

# Voice input links sherpa-onnx and ONNX Runtime, which ship beside the
# binaries in lib/gwen: /usr/lib/gwen, or ~/.local/lib/gwen after install-dev.
SHERPA_LIB = $(shell $(GO) list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-linux)/lib/x86_64-unknown-linux-gnu
VOICE_LIBS := libsherpa-onnx-c-api.so libonnxruntime.so

.PHONY: build ui build-hub check docs-check package install-dev clean ui-stub

build: ui-stub
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o bin/ ./cmd/gwend ./cmd/gwen ./cmd/gwen-tray
	cd cmd/gwen-ui && CGO_LDFLAGS='-Wl,-rpath,$$ORIGIN/../lib/gwen' $(GO) tool wails build -tags $(TAGS) -trimpath -ldflags '$(LDFLAGS)' -o gwen-ui
	cp cmd/gwen-ui/build/bin/gwen-ui bin/gwen-ui
	@mkdir -p bin/lib
	cd '$(SHERPA_LIB)' && install -m 0644 $(VOICE_LIBS) $(CURDIR)/bin/lib/

ui:
	cd ui && npm ci && npm run build

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
	@$(MAKE) --no-print-directory docs-check
	@if [ -d ui/node_modules ]; then cd ui && npx tsc --noEmit; fi

docs-check:
	@scripts/docs-check.sh

package: build
	@mkdir -p dist
	rm -f dist/*.rpm dist/*.deb dist/SHA256SUMS
	VERSION=$(PKG_VERSION) $(GO) tool nfpm package --config packaging/nfpm.yaml --packager rpm --target dist/
	VERSION=$(PKG_VERSION) $(GO) tool nfpm package --config packaging/nfpm.yaml --packager deb --target dist/
	cd dist && sha256sum *.rpm *.deb > SHA256SUMS

install-dev: build
	install -d $(HOME)/.local/bin $(CONFIG_HOME)/systemd/user
	install -m 0755 bin/gwend bin/gwen bin/gwen-tray bin/gwen-ui $(HOME)/.local/bin/
	install -d $(HOME)/.local/lib/gwen
	install -m 0644 $(addprefix bin/lib/,$(VOICE_LIBS)) $(HOME)/.local/lib/gwen/
	sed 's|^ExecStart=.*|ExecStart=%h/.local/bin/gwend|' packaging/gwend.service > $(CONFIG_HOME)/systemd/user/gwend.service
	systemctl --user daemon-reload
	@if command -v kpackagetool6 >/dev/null; then \
		kpackagetool6 -t Plasma/Applet -u packaging/plasma/dev.gwen.panel 2>/dev/null || kpackagetool6 -t Plasma/Applet -i packaging/plasma/dev.gwen.panel; \
	fi

clean:
	rm -rf bin dist

# Package ui embeds ui/dist, which only `make ui` builds, and ui/dist-hub,
# which only `make build-hub` builds. Until then placeholder pages let every
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
