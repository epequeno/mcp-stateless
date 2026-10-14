# mcp-stateless: an MCP server whose core holds no session
set shell := ["bash", "-uc"]

default:
    @just --list

# Fail loudly and usefully if the toolchain is missing, rather than
# letting a later recipe fail with a confusing error.
toolchain:
    @command -v go >/dev/null || { echo "go not installed — run: brew install go"; exit 1; }
    @go version

build: toolchain
    @go build ./...

# The gate for the scaffold: it compiles and its tests pass.
verify: toolchain
    @go vet ./...
    @go test ./...

run *ARGS: toolchain
    @go run ./cmd/mcpstateless {{ARGS}}

# Stateless core on :8080.
up: toolchain
    @go run ./cmd/mcpstateless --addr :8080

# Session-pinned comparison mode on :8081, for the A/B in bench/.
up-pinned: toolchain
    @go run ./cmd/mcpstateless --addr :8081 --pinned

# The headline number. Starts three server instances (two stateless for the
# cross-instance test, one pinned for the counterfactual), runs the bench
# harness against all three, then tears them down.
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
