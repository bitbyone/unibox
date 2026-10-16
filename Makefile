PREFIX ?= $(HOME)/.local

.PHONY: build test test-fast install clean fmt

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

clean:
	rm -f unibox
