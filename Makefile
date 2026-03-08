.PHONY: test run vet test-integration generate demo demo-stop demo-local demo-compose

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

# Demo: RUNTIME=local (default) starts server process and runs client; RUNTIME=compose uses Docker Compose.
# Uses DEMO_PORT (default 50052) for local to avoid conflict with user-run server on 50051.
demo:
	@if [ "$(RUNTIME)" = "compose" ]; then \
		$(MAKE) demo-compose; \
	else \
		$(MAKE) demo-local; \
	fi

# Local demo: start server, run client, stop server.
# Verifies server is listening before running client (fails fast on invalid DB_TYPE etc.).
demo-local:
	@echo "[demo] Starting local server (db-type=$(DB_TYPE))..."
	@pid=$$(lsof -ti:$(DEMO_PORT) 2>/dev/null); [ -n "$$pid" ] && kill $$pid 2>/dev/null; sleep 1; true
	@LEDGER_DB_TYPE=$(DB_TYPE) go run ./cmd/server --addr=:$(DEMO_PORT) & \
	SERVER_PID=$$!; \
	sleep 3; \
	nc -z localhost $(DEMO_PORT) 2>/dev/null || { kill $$SERVER_PID 2>/dev/null; echo "[demo] Server failed to start (check LEDGER_DB_TYPE, e.g. use memory or postgres)"; exit 1; }; \
	$(MAKE) demo-client BACKEND_URL=localhost:$(DEMO_PORT) GOROUTINES=$(GOROUTINES) TRANSFERS=$(TRANSFERS) DUPLICATE_KEYS=$(DUPLICATE_KEYS); \
	EXIT=$$?; \
	pid=$$(lsof -ti:$(DEMO_PORT) 2>/dev/null); [ -n "$$pid" ] && kill $$pid 2>/dev/null; \
	kill $$SERVER_PID 2>/dev/null || true; \
	[ $$EXIT -eq 0 ] || echo "[demo] Demo failed (exit $$EXIT)."; \
	exit $$EXIT

# Compose demo uses host port 50053 (see docker-compose.yml) to avoid conflicting with 50051.
COMPOSE_DEMO_PORT ?= 50053

# Compose demo: bring up stack (--wait until server healthcheck passes), run client, tear down.
# Reject invalid DB_TYPE here so the container does not start and exit; server main.go also rejects for manual runs.
demo-compose:
	@if [ "$(DB_TYPE)" != "memory" ] && [ "$(DB_TYPE)" != "postgres" ]; then \
		echo "[demo] DB_TYPE must be memory or postgres, got: $(DB_TYPE)"; exit 1; fi
	@echo "[demo] Starting Docker Compose stack (db-type=$(DB_TYPE))..."
	@if [ "$(DB_TYPE)" = "postgres" ]; then \
		DB_TYPE=$(DB_TYPE) docker compose --profile postgres up -d --build --wait --wait-timeout 90; \
	else \
		DB_TYPE=$(DB_TYPE) docker compose up -d --build --wait --wait-timeout 90; \
	fi
	@$(MAKE) demo-client BACKEND_URL=localhost:$(COMPOSE_DEMO_PORT) GOROUTINES=$(GOROUTINES) TRANSFERS=$(TRANSFERS) DUPLICATE_KEYS=$(DUPLICATE_KEYS); \
	EXIT=$$?; \
	if [ "$(DB_TYPE)" = "postgres" ]; then docker compose --profile postgres down; else docker compose down; fi; \
	[ $$EXIT -eq 0 ] || echo "[demo] Demo failed (exit $$EXIT)."; \
	exit $$EXIT

# Run go vet to check for issues
vet:
	go vet ./...