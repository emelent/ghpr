.PHONY: build run clean install deps fmt lint test build-all

BINARY  := ghpr
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOBUILD := go build -trimpath -ldflags "$(LDFLAGS)"

# Build the application
build:
	$(GOBUILD) -o $(BINARY) .

# Run the application
run: build
	./$(BINARY)

# Clean build artifacts
clean:
	rm -f $(BINARY)
	rm -rf dist

# Install the application to /usr/local/bin
install: build
	sudo mv $(BINARY) /usr/local/bin/

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	golangci-lint run ./...

# Run tests
test:
	go test ./...

# Build for multiple platforms (same set the release workflow publishes)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64
build-all:
	@mkdir -p dist
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; [ "$$os" = windows ] && ext=.exe; \
		echo "building $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GOBUILD) -o dist/$(BINARY)-$$os-$$arch$$ext . || exit 1; \
	done
