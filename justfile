# mcp-stateless: an MCP server whose core holds no session
set shell := ["bash", "-uc"]

default:
    @just --list

# Fail loudly if the toolchain is missing, instead of failing confusingly later.
toolchain:
    @command -v go >/dev/null || { echo "go not installed — run: brew install go"; exit 1; }
    @go version

build: toolchain
    @go build ./...

# The gate: build, vet, gofmt, test — mirrors CI so red is caught locally.
verify: toolchain
    @go vet ./...
    @go test ./...
    @test -z "$(gofmt -l .)" || { echo "not gofmt-clean:"; gofmt -l .; exit 1; }

run *ARGS: toolchain
    @go run ./cmd/mcpstateless {{ARGS}}

# Stateless core on :8080.
up: toolchain
    @go run ./cmd/mcpstateless --addr :8080

# Session-pinned comparison mode on :8081, for the A/B in bench/.
up-pinned: toolchain
    @go run ./cmd/mcpstateless --addr :8081 --pinned

# The headline number: start three instances, run the harness, tear down.
bench: toolchain
	-@go run ./cmd/mcpstateless --addr :8080 &
	-@go run ./cmd/mcpstateless --addr :8082 &
	-@go run ./cmd/mcpstateless --addr :8081 --pinned &
	@sleep 2
	@go run ./bench
	-@pkill -f "mcpstateless --addr :8080" || true
	-@pkill -f "mcpstateless --addr :8082" || true
	-@pkill -f "mcpstateless --addr :8081 --pinned" || true

# Two instances behind a round-robin, no sticky sessions. Needs bench first.
soak: bench

fmt: toolchain
    @go fmt ./...

check: verify

clean: toolchain
    @go clean -cache
