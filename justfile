set dotenv-load := false
set positional-arguments

# Default recipe: list available recipes
default:
    @just --list

# Run CLI in human mode (default)
run *args:
    go run ./cmd/tag-track "$@"

# Run CLI in agent-facing token-conservative mode
run-ai *args:
    AGENT=1 go run ./cmd/tag-track "$@"

# Inspect track metadata tags
inspect *args:
    go run ./cmd/tag-track track inspect "$@"

# Update track metadata tags
update *args:
    go run ./cmd/tag-track track update "$@"

# Run all unit tests
test:
    go test -v ./...

# Lint and static check
lint:
    go vet ./...

# Static code analysis (alias for lint)
vet:
    go vet ./...

# Run static checks and test suite in sequence
check:
    go vet ./...
    go test ./...

# Format Go code
fmt:
    go fmt ./...

# Build the tag-track binary in bin/
build:
    mkdir -p bin
    go build -o bin/tag-track ./cmd/tag-track

# Install tag-track binary to GOPATH bin directory
install:
    go install ./cmd/tag-track

# Clean up build binaries and temporary files
clean:
    rm -rf bin tag-track coverage.out .tmp

# Download project dependencies for local development
setup:
    go mod download
