# Bridge Aggregator – common development tasks
#
# Usage:
#   make test        – run all tests (no external deps, no real funds)
#   make test-v      – verbose test output
#   make build       – compile the server binary
#   make run         – start the server (sources .env if it exists)

GO      := go
GOFLAGS := GOMODCACHE=/Users/catalyst/go/pkg/mod GOCACHE=/Users/catalyst/.cache/go-build

TEST_PKGS := ./internal/ethutil/... ./internal/service/... ./internal/api/...

.PHONY: test test-v build run

test:
	$(GOFLAGS) $(GO) test $(TEST_PKGS) -count=1

test-v:
	$(GOFLAGS) $(GO) test $(TEST_PKGS) -v -count=1

build:
	$(GOFLAGS) $(GO) build -o bin/bridge-aggregator ./cmd/server

run:
	@if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	$(GOFLAGS) $(GO) run ./cmd/server
