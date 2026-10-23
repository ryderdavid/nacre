VERSION ?= 0.1.0
LDFLAGS := -s -w -X github.com/ryderdavid/nacre/bridge/internal/version.Version=$(VERSION)
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
