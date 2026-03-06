.PHONY: test run vet test-integration generate demo demo-stop

# Make parameters for demo (Phase 5: local in-memory; Phase 6: compose/postgres)
DEMO_PORT ?= 50052
BACKEND_URL ?= localhost:50051
GOROUTINES ?= 50
TRANSFERS ?= 200
DUPLICATE_KEYS ?= 20
DB_TYPE ?= memory
RUNTIME ?= local

# Kill any process listening on DEMO_PORT. Use when a prior demo left the server running.
demo-stop:
	@pid=$$(lsof -ti:$(DEMO_PORT) 2>/dev/null); \
	if [ -n "$$pid" ]; then kill $$pid 2>/dev/null && echo "[demo] Stopped server (PID $$pid)"; else echo "[demo] No server on port $(DEMO_PORT)"; fi

# Format all code according to the official Go style
fmt:
	go fmt ./...

# Run all tests (with race detector). Excludes integration tests.
test:
	go test -race -v ./...

# Run all tests including Postgres integration tests (requires Docker).
test-integration:
	go test -race -tags integration -v -count=1 ./...

# Generate sqlc and protobuf code.
# Requires: sqlc (go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest)
#           buf (go install github.com/bufbuild/buf/cmd/buf@latest)
generate:
	sqlc generate
	buf generate

# Run the server (in-memory backend)
run:
	go run ./cmd/server

# Run the concurrent demo client against a running server.
# Use: make demo [BACKEND_URL=...] [GOROUTINES=...] [TRANSFERS=...] [DUPLICATE_KEYS=...]
demo-client:
	go run ./cmd/client \
		--backend-url=$(BACKEND_URL) \
		--goroutines=$(GOROUTINES) \
		--transfers=$(TRANSFERS) \
		--duplicate-keys=$(DUPLICATE_KEYS)

# Local demo: start server, run client, stop server. Phase 5 uses DB_TYPE=memory, RUNTIME=local.
# Uses DEMO_PORT (default 50052) to avoid conflict with user-run server on 50051.
# Cleans up any process on DEMO_PORT at startup and end (port-based kill is more reliable than PID).
demo:
	@echo "[demo] Starting local server (db-type=$(DB_TYPE))..."
	@pid=$$(lsof -ti:$(DEMO_PORT) 2>/dev/null); [ -n "$$pid" ] && kill $$pid 2>/dev/null; sleep 1; true
	@go run ./cmd/server --addr=:$(DEMO_PORT) & \
	SERVER_PID=$$!; \
	sleep 3; \
	$(MAKE) demo-client BACKEND_URL=localhost:$(DEMO_PORT) GOROUTINES=$(GOROUTINES) TRANSFERS=$(TRANSFERS) DUPLICATE_KEYS=$(DUPLICATE_KEYS); \
	EXIT=$$?; \
	pid=$$(lsof -ti:$(DEMO_PORT) 2>/dev/null); [ -n "$$pid" ] && kill $$pid 2>/dev/null; \
	kill $$SERVER_PID 2>/dev/null || true; \
	exit $$EXIT

# Run go vet to check for issues
vet:
	go vet ./...