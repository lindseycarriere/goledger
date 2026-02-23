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

### Running

Start the service:
```bash
make run
```

Run tests:
```bash
make test
```

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