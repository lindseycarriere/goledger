# Go-Ledger

A high-performance, immutable financial ledger service built to demonstrate **ACID compliance**, **idempotency**, and **concurrency control** in distributed systems.
This project simulates a core banking engine that processes financial transactions safely at high concurrency. No race conditions, no double-spends.

## Architecture

This project follows idiomatic Go conventions for a production microservice:

```
goledger/
├── cmd/
│   ├── server/          # gRPC server entrypoint (Phase 3+)
│   │   └── main.go
│   └── client/          # Concurrent CLI demo client (Phase 5)
│       └── main.go
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
├── Makefile             # Developer commands
└── go.mod
```

## Getting Started

### Prerequisites

- Go 1.21+ (for slog support)
- Make
- Docker (for integration tests; [testcontainers](https://golang.testcontainers.org/) spins up Postgres in a container)
- sqlc (for schema/query changes): `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`
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

Run Go formatting / vetting:
```bash
make fmt vet
```

Generate sqlc code (after editing schema or queries):
```bash
make generate
```

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

### Expected Output

`make run` produces structured JSON logging:
```json
{"time":"2024-01-01T12:00:00Z","level":"INFO","msg":"ledger service starting","version":"0.1.0"}
```

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
- **Phase 5**: Demo & Packaging - Concurrency client, one-command demo

## Project Goals

- **ACID Compliance**: Transactions are atomic, consistent, isolated, and durable
- **Idempotency**: Duplicate requests are safely deduplicated
- **Concurrency Control**: Safe concurrent access without race conditions
- **Learning Focus**: Go idioms for Java developers transitioning to Go
