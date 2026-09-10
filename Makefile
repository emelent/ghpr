.PHONY: build run clean install deps fmt lint test build-all

BINARY := ghpr

# Build the application
build:
	go build -o $(BINARY) .

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

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o dist/$(BINARY)-linux-amd64 .
	GOOS=linux GOARCH=386 go build -o dist/$(BINARY)-linux-x86 .
	GOOS=linux GOARCH=arm GOARM=6 go build -o dist/$(BINARY)-linux-armv6 .
	GOOS=linux GOARCH=arm GOARM=7 go build -o dist/$(BINARY)-linux-armv7 .
	GOOS=linux GOARCH=arm64 go build -o dist/$(BINARY)-linux-arm64 .
	GOOS=darwin GOARCH=amd64 go build -o dist/$(BINARY)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -o dist/$(BINARY)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -o dist/$(BINARY)-windows-amd64.exe .
