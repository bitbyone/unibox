PREFIX ?= $(HOME)/.local

.PHONY: build test test-fast install clean fmt

build:
	go build -o unibox ./cmd/unibox

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
