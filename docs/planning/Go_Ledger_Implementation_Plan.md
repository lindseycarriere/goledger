# Go-Ledger: Implementation Plan

## Project Overview

**Go-Ledger** is a high-performance, immutable financial ledger service built to demonstrate **ACID compliance**, **idempotency**, and **concurrency control** in distributed systems. It simulates a core banking engine that processes financial transactions safely at high concurrency — no race conditions, no double-spends.

The project is structured as a progressive learning journey for engineers coming from a Java/Spring background. Each phase builds on the last, introducing Go idioms and infrastructure dependencies only when they clearly earn their place.

---

## Project Structure

This layout follows idiomatic Go conventions for a production microservice. It is established in Phase 0 and grows into it organically — early phases will leave many directories empty or unpopulated.

```
<repo-name>/
├── cmd/
│   ├── server/
│   │   └── main.go          # gRPC server entrypoint (Phase 3+)
│   └── client/
│       └── main.go          # Concurrent CLI demo client (Phase 5)
├── internal/
│   ├── domain/              # Pure domain types, interfaces, business rules. Zero external deps.
│   │   ├── account.go
│   │   ├── transaction.go
│   │   └── ledger.go        # Ledger interface contract
│   ├── ledger/              # Use-case / service layer. Orchestrates domain logic.
│   │   └── service.go
│   ├── store/               # Persistence implementations, hidden behind the domain interface.
│   │   ├── memory/
│   │   │   └── store.go     # In-memory store (Phase 1)
│   │   └── postgres/
│   │       └── store.go     # Postgres store (Phase 2+)
│   └── server/              # gRPC transport layer (Phase 3+)
│       └── grpc.go
├── proto/                   # Protobuf definitions (Phase 3+)
│   └── ledger/v1/
│       └── ledger.proto
├── migrations/              # SQL schema migration files (Phase 2+)
├── Makefile                 # Top-level developer commands
├── go.mod
├── go.sum
└── README.md
```

> **Java Parallel:** `domain/` ≈ your model/entity layer. `ledger/` ≈ your `@Service` classes. `store/` ≈ your `@Repository` implementations. `server/` ≈ your `@RestController` or gRPC endpoint wiring. The key difference: Go expresses all of this through **implicit interfaces** and plain functions — no annotations, no framework magic.

---

## Technology Introduced Per Phase

Dependencies are added only when they justify the complexity they bring.

| Phase | Dependency Added | Reason |
|---|---|---|
| 0 | None | Pure Go stdlib only |
| 1 | None | `sync` package is stdlib |
| 2 | `pgx`, `sqlc`, `golang-migrate`, `testcontainers-go` | Real persistence and hermetic integration testing |
| 3 | `grpc-go`, `protobuf`, `buf` | Schema-first API exposure |
| 4 | No new deps | Idempotency built on existing DB layer |
| 5 | `docker compose` | One-command demo packaging |

---

## Phase 0: Go Foundations

### User Story
*As a developer new to Go coming from Java, I want to scaffold a clean, idiomatic project and write my first Go program so that I understand the core language mechanics before adding any domain complexity.*

### Context
Before writing a single line of ledger logic, this phase ensures you are comfortable with the constructs that appear in every subsequent phase. Go is deceptively simple syntactically but idiomatically very different from Java — especially around error handling, interfaces, and the absence of a framework.

Key concepts covered:
- Module initialisation and project layout
- Go's type system: structs, value vs. pointer receivers, zero values
- **Error handling** — Go has no exceptions. `error` is a return value. `if err != nil` is idiomatic, not a smell.
- **Interfaces** — satisfied implicitly (structural typing). A type doesn't declare `implements Ledger`; it just has the right methods. This is the single biggest mental shift from Java.
- `defer` — analogous to Java's `finally` but scoped to a function, not a block. Evaluated LIFO at function return.
- Writing and running table-driven unit tests with `go test`
- Structured logging with `slog` (stdlib, Go 1.21+)

> **Java Parallel — Errors:** Java throws `Exception`. Go returns `error`. There is no try/catch. Callers are forced to handle errors at each call site. This feels verbose at first; it becomes a feature — errors are never silently swallowed by an unchecked exception.

> **Java Parallel — Interfaces:** `interface Ledger { PostTransaction(...) }` in Go is satisfied by any type with a matching `PostTransaction` method signature. The implementing type has no knowledge of the interface. This inverts the Java dependency — the consumer defines the contract, not the implementor.

### Definition of Done
- [ ] Repository initialised with `go mod init`, standard directory layout in place
- [ ] `README.md` with project purpose, architecture overview, and how to run
- [ ] `Makefile` with `make test` and `make run` targets
- [ ] A `main.go` in `cmd/server/` that starts, logs a startup message with `slog`, and exits cleanly
- [ ] At least one Go source file demonstrating: a struct, a method with a pointer receiver, an interface, and an `error` return
- [ ] A passing table-driven unit test file
- [ ] `go vet ./...` and `go test ./...` both pass with zero warnings

### Verification
```
make test   # All tests pass
make run    # Prints structured JSON log line and exits 0
go vet ./... # Zero issues
```

Expected output of `make run`:
```json
{"time":"...","level":"INFO","msg":"ledger service starting","version":"0.1.0"}
```

---

## Phase 1: In-Memory Ledger (Concurrency & Domain Logic)

### User Story
*As a developer learning Go concurrency, I want to implement a double-entry ledger that safely handles concurrent transfers using goroutines and mutexes so that I understand Go's concurrency model and can prove correctness under load.*

### Context
This phase builds the core business logic entirely in memory — no database, no network. The goal is to master Go's concurrency primitives and the domain rules before any infrastructure complexity enters.

Double-entry bookkeeping means every transfer creates two entries: a debit on one account and a credit on another. The invariant is simple and testable: **the sum of all account balances must never change after any transfer.**

All monetary values are stored as `int64` representing micros (1 unit = 0.000001 of the currency). This eliminates floating-point rounding errors entirely — a common source of financial bugs.

The `Ledger` interface defined here in `internal/domain/` is the contract that **both** the in-memory store (this phase) and the Postgres store (Phase 2) will satisfy — without either knowing about the other.

Key concepts covered:
- Defining and implementing Go interfaces
- `sync.Mutex` for mutual exclusion — the Go equivalent of Java's `synchronized`
- **Deadlock prevention:** transferring between two accounts requires locking both. Naive locking causes deadlocks. The canonical Go solution is to enforce a consistent lock ordering (e.g., always lock the lower account ID first).
- Goroutines — lightweight threads managed by the Go runtime. `go func()` spawns one.
- `sync.WaitGroup` — waiting for a group of goroutines to complete (analogous to `CountDownLatch` or `CompletableFuture.allOf` in Java)
- The `-race` flag: Go's built-in race condition detector

> **Java Parallel — Goroutines vs Threads:** Java threads are OS threads (expensive, ~1MB stack). Goroutines are multiplexed onto OS threads by the Go scheduler (~2KB initial stack, can run millions concurrently). `go fn()` is the idiom; there is no `Thread.start()`.

> **Java Parallel — sync.Mutex vs synchronized:** Go's `sync.Mutex` is explicit (you call `.Lock()` and `.Unlock()`). Always pair with `defer mu.Unlock()` immediately after `mu.Lock()` to guarantee release, just as you'd use try/finally in Java.

### Definition of Done
- [ ] `internal/domain/` contains `Account`, `Transaction` structs and a `Ledger` interface
- [ ] `internal/store/memory/` implements the `Ledger` interface using a `map` and `sync.Mutex`
- [ ] `PostTransfer` correctly locks both accounts, applies debit and credit atomically, and rolls back (in memory) on validation failure
- [ ] `cmd/server/main.go` demonstrates a transfer and prints the resulting balances
- [ ] Unit tests cover: successful transfer, insufficient funds, self-transfer rejection, concurrent stress test
- [ ] **Stress test:** 100 goroutines fire concurrent transfers simultaneously; final balance sum matches initial sum
- [ ] All tests pass with `go test -race ./...` (race detector finds zero issues)

### Verification
```
go test -race -count=1 ./...   # All pass, no data races reported
make run                       # Prints initial balances, runs transfers, prints final balances
```

Expected `make run` output:
```
Initial: account:A=100000000 account:B=0
Transfer: A -> B 50000000 micros
Final:   account:A=50000000 account:B=50000000
Sum invariant: PASS (total=100000000)
```

---

## Phase 2: Persistence Layer (Real Database & Integration Testing)

### User Story
*As a developer, I want to replace the in-memory store with a real ACID-compliant Postgres database so that I can prove the transfer invariant holds under concurrency at the database level, with hermetic integration tests that use a real Postgres instance.*

### Context
Phase 1 proved correctness in memory. Phase 2 proves the same guarantee with a real database — which introduces a new class of challenges: network latency, transaction isolation levels, and row-level locking.

The `Ledger` interface from Phase 1 is unchanged. Only the backing implementation swaps from `store/memory` to `store/postgres`. This is the payoff of the interface-first design.

**New tools introduced:**
- **`golang-migrate`** (CLI only): Manages schema changes as versioned SQL files in `migrations/`. Treats the schema as code with a history — the same way Git treats source code.
- **`sqlc`**: Takes raw SQL queries you write and compiles them into type-safe Go functions. There is no ORM. You write the SQL; Go enforces types at compile time. Zero runtime reflection.
- **`pgx`**: The high-performance Postgres driver that `sqlc` generates code against.
- **`testcontainers-go`**: Spins up a real, disposable Postgres Docker container per test suite. No mocking, no test doubles for the database. Tests hit a real engine and tear it down after.

The critical SQL pattern for concurrent transfers is `SELECT ... FOR UPDATE`, which acquires a row-level lock within a transaction, preventing another transaction from modifying the same account rows until the first commits or rolls back. This is how the database enforces the same invariant that the `sync.Mutex` enforced in memory.

> **Java Parallel — sqlc vs JPA:** JPA/Hibernate generates SQL from annotations at runtime and can produce surprising queries. `sqlc` runs at compile time — if your SQL is wrong, you find out before the program runs, not in production.

> **Java Parallel — pgx transactions vs Spring @Transactional:** In Go there is no AOP proxy or annotation magic. You call `db.BeginTx()`, defer a rollback, do your work, then explicitly commit. The control flow is explicit and readable — you can see exactly when transactions begin and end.

### Definition of Done
- [ ] `migrations/` contains versioned SQL files for `accounts` and `entries` tables
- [ ] `sqlc.yaml` configured; generated Go code lives in `internal/store/postgres/`
- [ ] `internal/store/postgres/` implements the same `Ledger` interface as the in-memory store
- [ ] `PostTransfer` uses `BEGIN / SELECT FOR UPDATE / INSERT entries / UPDATE balances / COMMIT`
- [ ] Testcontainers setup runs migrations automatically before each integration test suite
- [ ] **Stress test re-run:** 100 goroutines fire concurrent transfers against real Postgres; final balance sum matches initial sum
- [ ] All integration tests pass; unit tests from Phase 1 still pass against the in-memory store

### Verification
```
go test -race -tags integration -count=1 ./...   # All pass including DB integration tests
```

The test output should show Testcontainers starting and stopping Postgres:
```
--- PASS: TestConcurrentTransfers (4.21s)
    stress: 100 goroutines, 1000 transfers, 0 invariant violations
```

---

## Phase 3: gRPC API

### User Story
*As a developer, I want to expose the ledger service via a gRPC API defined by a Protobuf schema so that clients can call `CreateAccount`, `GetBalance`, and `PostTransaction` over a strongly-typed RPC interface.*

### Context
This phase wires the service built in Phases 1 and 2 to a network transport. gRPC is chosen because it is schema-first (the `.proto` file is the contract), binary-efficient (Protobuf encoding), and natively generates client and server code in any language.

**New tools introduced:**
- **`buf`**: Manages the Protobuf toolchain (replaces raw `protoc` with a simpler, reproducible config). Generates Go code from `.proto` files.
- **`grpc-go`**: The Go gRPC runtime library.

The `.proto` file in `proto/ledger/v1/ledger.proto` defines the three RPC methods. Generated code goes into a `gen/` directory (excluded from manual editing). The `internal/server/grpc.go` implements the generated server interface and delegates to `domain.Ledger`.

gRPC interceptors (middleware) are added for:
- **Request logging**: Logs every RPC call with method name, duration, and status code using `slog`
- **Panic recovery**: Catches any unexpected panics and returns a gRPC `INTERNAL` error instead of crashing the server

`grpcurl` is used as the verification client — it's a command-line tool for calling gRPC servers (analogous to `curl` for HTTP). It requires no code to use and makes the API immediately explorable.

> **Java Parallel — gRPC vs REST:** Where Spring Boot uses Jackson + annotation-driven routing, gRPC uses Protobuf + generated interfaces. The generated interface forces you to implement every method; there's no runtime reflection routing. The trade-off: more setup, but the contract is enforced at compile time on both sides.

### Definition of Done
- [ ] `proto/ledger/v1/ledger.proto` defines `CreateAccount`, `GetBalance`, `PostTransaction` RPCs
- [ ] `buf.yaml` and `buf.gen.yaml` configured; `make generate` regenerates Go code cleanly
- [ ] `internal/server/grpc.go` implements the generated server interface, delegates to `domain.Ledger`
- [ ] Server starts on a configurable port and logs each RPC call with duration
- [ ] Panic recovery interceptor in place
- [ ] Integration test calls each RPC method and asserts correct response

### Verification

```bash
make run   # Server starts and logs: {"msg":"gRPC server listening","addr":"[::]:50051"}
```

In a second terminal, run the grpcurl commands below. See [docs/testing/grpc-e2e.md](../testing/grpc-e2e.md) for full end-to-end verification (including PATH setup if grpcurl is not found).

```bash
# Create accounts (grpcurl requires PATH; see docs/testing/grpc-e2e.md if command not found)
grpcurl -plaintext -d '{"account_id":"A","initial_balance_micros":100000000}' localhost:50051 ledger.v1.LedgerService/CreateAccount
grpcurl -plaintext -d '{"account_id":"B","initial_balance_micros":0}' localhost:50051 ledger.v1.LedgerService/CreateAccount

# Get balance
grpcurl -plaintext -d '{"account_id":"A"}' localhost:50051 ledger.v1.LedgerService/GetBalance

# Post transaction
grpcurl -plaintext -d '{"from":"A","to":"B","amount_micros":50000000}' localhost:50051 ledger.v1.LedgerService/PostTransaction
```

---

## Phase 4: Idempotency

### User Story
*As a developer, I want `PostTransaction` to be idempotent so that duplicate requests — caused by network retries or client bugs — are safely deduplicated and return the original response without re-processing the transfer.*

### Context
Idempotency is a critical distributed systems property: calling an operation multiple times with the same inputs produces the same result as calling it once. For financial transactions, this is non-negotiable — a retry must never cause a double-spend.

The implementation pattern: every `PostTransaction` request includes a client-generated `idempotency_key` (a UUID). Before executing the transfer, the service checks whether this key has been seen before. If yes, it returns the stored response immediately. If no, it executes and stores both the result and the key atomically in the same transaction.

This requires a schema change — a new `idempotency_keys` table — added as a new migration file. No new external dependencies are introduced; this is pure application and SQL logic built on Phase 2's foundation.

The verification story for this phase is particularly clear: fire the same request 10 times and observe that the balance changes only once.

### Definition of Done
- [ ] New migration adds `idempotency_keys` table with `(key, response, created_at)` columns and a unique constraint on `key`
- [ ] `PostTransaction` checks for existing key before executing; returns cached response if found
- [ ] Key storage and transfer execution occur in a single database transaction (atomic)
- [ ] Integration test fires the same `PostTransaction` request 10 times with the same key; asserts balance debited exactly once
- [ ] A distinct request with a new key executes normally

### Verification
```
go test -race -tags integration -run TestIdempotency ./...
```

Manual verification with `grpcurl` — fire the same request twice:
```bash
grpcurl -plaintext -d '{"idempotency_key":"abc-123","from":"A","to":"B","amount_micros":10000000}' \
  localhost:50051 ledger.v1.LedgerService/PostTransaction   # executes
grpcurl -plaintext -d '{"idempotency_key":"abc-123","from":"A","to":"B","amount_micros":10000000}' \
  localhost:50051 ledger.v1.LedgerService/PostTransaction   # returns cached result, balance unchanged
```

---

## Phase 5: Demo & Packaging

### User Story
*As a developer showcasing this project, I want a single `make demo` command that starts the full stack, runs a concurrent load scenario, and prints a human-readable proof of correctness so that anyone can verify the system in under two minutes.*

### Context
This phase is about storytelling. The engineering is done; Phase 5 makes it observable and shareable. Three artifacts are produced:

**1. `cmd/client/main.go` — The Concurrent Demo Client**
A Go program that itself is a concurrency exercise: it spawns N goroutines (configurable via flag), each firing a `PostTransaction` gRPC request. It collects results using `sync.WaitGroup` and `channels`, then prints a summary showing final balances and whether the sum invariant holds. This client is the "demo" — it shows both the gRPC API working and Go concurrency in action.

**2. `docker-compose.yml`**
Defines two services: `postgres` and `server`. The server depends on Postgres being healthy (via a healthcheck). A single `docker compose up` starts the full stack.

**3. `Makefile` demo target**
```
make demo
```
Sequence:
1. Starts Docker Compose stack
2. Waits for server healthcheck
3. Creates two accounts via gRPC
4. Runs the concurrent client with 50 goroutines, 200 total transfers, including deliberate duplicates
5. Prints final balances and invariant proof
6. Tears down the stack

> **Java Parallel — No Spring Boot, no Tomcat:** The entire server is a single statically compiled binary. `docker build` produces an image typically under 20MB. There is no JVM startup time, no warm-up phase, no GC tuning required for this scale.

### Definition of Done
- [ ] `cmd/client/main.go` accepts `--goroutines`, `--transfers`, and `--addr` flags
- [ ] Client uses goroutines + `sync.WaitGroup` to fire concurrent gRPC requests
- [ ] Client prints a clear summary: requests sent, successes, duplicates detected, final balances, invariant result
- [ ] `docker-compose.yml` starts Postgres and server cleanly with a healthcheck
- [ ] `Dockerfile` produces a multi-stage build (builder + minimal runtime image)
- [ ] `make demo` runs the full scenario end-to-end and exits with code 0
- [ ] `README.md` updated with a Prerequisites section and the one-command demo instructions

### Verification
```
make demo
```

Expected output:
```
[demo] Starting stack...
[demo] Creating accounts A and B...
[demo] Firing 200 transfers across 50 goroutines (20 duplicate idempotency keys)...
[demo] Results:
  Transfers executed:  180
  Duplicates detected: 20
  Final balance A:     80000000 micros
  Final balance B:     120000000 micros
  Sum invariant:       PASS (expected=200000000, got=200000000)
[demo] All checks passed.
```

---

## Summary

| Phase | Theme | New Dependency | Key Learning | Runnable Artifact |
|---|---|---|---|---|
| 0 | Go Foundations | None | Syntax, errors, interfaces, testing | `make run` (startup log) |
| 1 | In-Memory Ledger | None | Goroutines, mutex, race detector | `make run` (balance proof) |
| 2 | Persistence | pgx, sqlc, golang-migrate, testcontainers | ACID transactions, SQL, integration testing | `go test -tags integration` |
| 3 | gRPC API | grpc-go, buf | Protobuf, RPC server, interceptors | `grpcurl` calls |
| 4 | Idempotency | None | Distributed systems reliability | `grpcurl` duplicate demo |
| 5 | Demo & Packaging | Docker Compose | Concurrency client, one-command demo | `make demo` |
