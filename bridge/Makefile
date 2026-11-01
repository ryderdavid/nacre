# Release tags supply the version; later commits and local edits remain visible.
# Archives without git metadata can supply VERSION and COMMIT explicitly.
VERSION ?= $(shell v=$$(git describe --tags --match 'v[0-9]*' --dirty 2>/dev/null) && echo "$${v\#v}" || { v=$$(git describe --always --dirty 2>/dev/null) && echo "dev-$$v" || echo dev; })
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null)
LDFLAGS := -s -w -X github.com/ryderdavid/nacre/bridge/internal/version.Version=$(VERSION) -X github.com/ryderdavid/nacre/bridge/internal/version.Commit=$(COMMIT)
BIN := bin/nacred

.PHONY: build test vet install clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/nacred

test: vet
	go test ./...

vet:
	go vet ./...

# Copies the binary to ~/.local/bin/nacred, writes the LaunchAgent plist and loads it.
install: build
	./$(BIN) install

clean:
	rm -rf bin
