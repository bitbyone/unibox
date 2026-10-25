PREFIX ?= $(HOME)/.local

.PHONY: build test test-fast install clean fmt dist

# VERSION is the release tag on HEAD without its v, empty anywhere else,
# where unibox --version names the commit instead.
VERSION ?= $(shell git describe --tags --exact-match --match 'v[0-9]*.[0-9]*' 2>/dev/null | sed 's/^v//')
LDFLAGS := $(if $(VERSION),-X github.com/tobola/unibox/internal/version.Version=$(VERSION))

build:
	go build -ldflags "$(LDFLAGS)" -o unibox ./cmd/unibox

test:
	go test -race ./...

test-fast:
	go test ./...

fmt:
	gofmt -w .

install: build
	install -d "$(PREFIX)/bin"
	@if ! cmp -s unibox "$(PREFIX)/bin/unibox" || [ ! -x "$(PREFIX)/bin/unibox" ]; then \
		install -m 0755 unibox "$(PREFIX)/bin/unibox"; \
		echo "installed unibox"; \
	fi

# dist builds a release for every system unibox runs on, without cgo so
# each binary stands alone: an archive per system, unibox and its licence
# at its top, and their SHA-256 sums.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

dist:
	@test -n "$(VERSION)" || { echo "dist needs VERSION, the release's version without its v"; exit 1; }
	rm -rf dist
	set -e; for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; name=unibox_$(VERSION)_$${os}_$${arch}; exe=unibox; \
		if [ $$os = windows ]; then exe=unibox.exe; fi; \
		mkdir -p dist/$$name; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$$name/$$exe ./cmd/unibox; \
		cp LICENSE README.md dist/$$name/; \
		if [ $$os = windows ]; then (cd dist/$$name && zip -q ../$$name.zip *); \
		else tar -C dist/$$name -czf dist/$$name.tar.gz $$exe LICENSE README.md; fi; \
		rm -rf dist/$$name; \
	done
	cd dist && if command -v sha256sum >/dev/null; then sha256sum *.tar.gz *.zip; else shasum -a 256 *.tar.gz *.zip; fi > checksums.txt

clean:
	rm -rf unibox dist
