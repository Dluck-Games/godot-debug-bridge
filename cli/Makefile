# godot-debug-bridge/cli/Makefile
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test clean

build:
	go build -ldflags='$(LDFLAGS)' -o gdbg .

test:
	go test ./...
	go vet ./...

clean:
	rm -f gdbg
