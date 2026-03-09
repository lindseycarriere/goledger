# Go-Ledger

[![CI](https://github.com/lindseycarriere/goledger/actions/workflows/ci.yml/badge.svg)](https://github.com/lindseycarriere/goledger/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/lindseycarriere/goledger.svg)](https://pkg.go.dev/github.com/lindseycarriere/goledger)
[![Go Report Card](https://goreportcard.com/badge/github.com/lindseycarriere/goledger)](https://goreportcard.com/report/github.com/lindseycarriere/goledger)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

A high-performance, immutable financial ledger service built to demonstrate **ACID compliance**, **idempotency**, and **concurrency control** in distributed systems.
This project simulates a core banking engine that processes financial transactions safely at high concurrency. No race conditions, no double-spends.

## Architecture

This project follows idiomatic Go conventions for a production microservice:

```
goledger/
├── cmd/
│   ├── server/          # gRPC server entrypoint
│   ├── healthcheck/     # gRPC health check client utility
│   └── client/          # Go client to exercise the Go server concurrently
├── docs/
│   ├── planning/        # The implementation phases for this project (aka spec-driven development)
│   ├── testing/         # Extra testing documentation
├── internal/
│   ├── domain/          # Pure domain types, interfaces, business rules
│   ├── ledger/          # Use-case / service layer
│   ├── store/           # Persistence implementations
│   │   ├── memory/      # In-memory store (Phase 1)
│   │   └── postgres/    # Postgres store (Phase 2+)
│   │       ├── sql/     # schema.sql + queries.sql (sqlc inputs)
│   │       └── db/      # Generated Go (sqlc output; do not edit)
│   └── server/          # gRPC transport layer (Phase 3+)
├── proto/               # Protobuf definitions (Phase 3+)
├── migrations/          # SQL schema migrations (Phase 2+)
└── Makefile             # Developer & CI commands
```

## Getting Started

### Prerequisites

- Go 1.21+ (for slog support)
- Make
- Docker (for integration tests and optional containerized demo; [testcontainers](https://golang.testcontainers.org/) spins up Postgres in a container)
- sqlc (for schema/query changes): `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
- buf (for proto changes): `go install github.com/bufbuild/buf/cmd/buf@latest`
- grpcurl (for manual API verification): `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`

Ensure `$(go env GOPATH)/bin` is in your PATH so these tools are found.
- Leverage the pre-commit hooks with:
  - `brew install pre-commit` if you don't have it. Then `pre-commit install`

### Running

Start the service:
```bash
make run
```

Run unit tests (excludes integration tests):
```bash
make test
```

Run integration tests (requires Docker; uses testcontainers to start Postgres):
```bash
make test-integration
```

Run Go formatting, vetting, module cleanup:
```bash
make fix
```

Generate sqlc and protobuf code (after editing schema, queries, or `.proto` files):
```bash
make generate
```

### gRPC API Implementation

The gRPC API is defined in `proto/ledger/v1/ledger.proto`.

| Tool | Purpose |
|------|---------|
| **buf** | Manages the Protobuf toolchain. Generates Go code from `.proto` files via `make generate`. |
| **grpcurl** | Command-line client for calling gRPC methods. Use for manual verification. |

**When making changes:**

Never edit generated files in `gen/**`. Instead:

1. Edit `proto/ledger/v1/ledger.proto` (add RPCs, messages, or fields)
2. Run `make generate` - this regenerates `gen/go/ledger/v1/*.pb.go`
3. Update `internal/server/grpc.go` to implement new RPCs or handle new fields
4. Run `make test` and verify end-to-end with grpcurl (see [docs/testing/grpc-e2e.md](docs/testing/grpc-e2e.md))

### Postgres Database Implementation

The Postgres store uses two complementary tools for schema and query management:

| Tool | Purpose |
|------|---------|
| **golang-migrate** | Versioned schema in `migrations/`. These `.up.sql` / `.down.sql` files are applied to the database. Integration tests run them automatically via ephemeral `testcontainers`. |
| **sqlc** | Generates type-safe Go from SQL. You write `sql/schema.sql` and `sql/queries.sql`; sqlc produces `db/*.go`. |

**When making changes:**

1. **Schema change**:
   - Add a new migration in `migrations/` (e.g. `000002_add_foo.up.sql` and `.down.sql`)
   - Update `internal/store/postgres/sql/schema.sql` to match (sqlc needs it for type generation)
   - Add or adjust queries in `internal/store/postgres/sql/queries.sql` if needed
   - Run `make generate` → regenerates `internal/store/postgres/db/*.go`
   - Run `make test-integration` to verify

2. **Query change**:
   - Edit `internal/store/postgres/sql/queries.sql`
   - Run `make generate`
   - Update implementation (ie: `internal/store/postgres/store.go`) to use the new generated functions

**Do not edit** `internal/store/postgres/db/*.go` — it is generated. Edit the `.sql` files and regenerate.

### Demo Modes

Run the local go client against various go ledger server implementations if you want to run within docker compose for interoperability.

| Command | Description |
|---------|-------------|
| `make demo` | Runs server with your local go runtime + in-memory datastore. |
| `make demo RUNTIME=compose DB_TYPE=memory` | Containerized server + in-memory datastore. |
| `make demo RUNTIME=compose DB_TYPE=postgres` | Containerized server + Postgres DB. |

Compose demos use host port **50053** (not 50051) so they do not conflict with a local `make run` servers for e2e testing. Ensure port **50053** is free when using `RUNTIME=compose`.

Optionally, override demo parameters: `make demo GOROUTINES=100 TRANSFERS=1000 DUPLICATE_KEYS=50`

### Local Demo

Demonstrates how a Go client connects to the `goledger` gRPC API and runs concurrent transfers with idempotency.

```bash
make demo
```

This starts a local server (in-memory backend), creates accounts "A" and "B", and by default runs 200 transfers across 50 goroutines (including 20 deliberate idempotency-key duplicates), then prints final balances and the sum invariant. 

Example output:

```
[demo] Starting local server (db-type=memory)...
[demo] Creating accounts A and B...
[demo] Firing 200 transfers across 50 goroutines (20 duplicate idempotency keys)...

[demo] Initial state:
  Initial balance A:   200000000 micros
  Initial balance B:   0 micros
  Transfer amount:     100000 micros (per transfer)
  Total transfers:     200
  Goroutines:          50 (concurrent transfers)
  Duplicate Requests:  20

[demo] Results:
  Duration:            13 ms
  Transfers executed:  180
  Duplicates detected: 20
  Failed transfers:    0
  Final balance A:     182000000 micros
  Final balance B:     18000000 micros
  Sum invariant:       PASS (expected=200000000, got=200000000)
[demo] All checks passed.
```

Optionally override parameters like: `make demo GOROUTINES=100 TRANSFERS=1000 DUPLICATE_KEYS=50`

If a prior demo left the server running (e.g. "bind: address already in use"), stop it with:
```bash
make demo-stop
```
Or manually: `kill $(lsof -t -i:50052)`

### Expected Output

`make run` produces structured JSON logging:
```json
{"time":"...","level":"INFO","msg":"gRPC server listening","addr":"[::]:50051"}
```

For manual API verification with grpcurl, see [docs/testing/grpc-e2e.md](docs/testing/grpc-e2e.md).

All tests pass:
```bash
make test
# Running tool: /usr/local/go/bin/go test -v ./...
# === RUN   TestDemo
# --- PASS: TestDemo (0.00s)
# PASS
# ok      goledger/internal/domain/example  0.001s
```

## Learning Journey

This project progresses through phases, each introducing Go idioms and infrastructure dependencies only when they justify the complexity:

- **Phase 0**: Go Foundations (stdlib only) - Syntax, errors, interfaces, testing
- **Phase 1**: In-Memory Ledger - Goroutines, mutex, race detector
- **Phase 2**: Persistence - ACID transactions, SQL, integration testing
- **Phase 3**: gRPC API - Protobuf, RPC server, interceptors
- **Phase 4**: Idempotency - Distributed systems reliability
- **Phase 5**: Demo Client (Local Runtime) - Starts local server and runs local go client demo.
- **Phase 6**: Demo Docker w/ Postgres - Docker Compose the server, datastore selection (memory or postgres) and run the demo client.

## Project Goals

- **ACID Compliance**: Transactions are atomic, consistent, isolated, and durable
- **Idempotency**: Duplicate requests are safely deduplicated
- **Concurrency Control**: Safe concurrent access without race conditions
- **Learning Focus**: Go idioms for Java developers transitioning to Go
