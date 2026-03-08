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
| 5 | No new deps | Demo client and scenario runner against existing in-memory server |
| 6 | `docker compose` | Containerized demo packaging and backend switching (`memory`/`postgres`) |
| 7 | No new deps | Idempotency refinements: shared handler, response metadata |

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

## Phase 5: Demo Client (In-Memory First)

### User Story
*As a developer showcasing this project, I want a clear, local, no-Docker demo flow that runs a concurrent gRPC scenario against the current in-memory backend and prints proof of correctness in under two minutes.*

### Context
This phase is about storytelling with minimal moving parts. Phase 5 makes it observable and repeatable without introducing container orchestration yet.

The main artifact is:

**`cmd/client/main.go` — The Concurrent Demo Client**
A Go program that itself is a concurrency exercise: it spawns N goroutines (configurable via flags), each firing a `PostTransaction` gRPC request. It collects results using `sync.WaitGroup` and channels, then prints a summary showing final balances, duplicate detection, and whether the sum invariant holds.

`Makefile` provides a simple local entrypoint:
```
make demo
```
Phase 5 demo sequence:
1. Starts (or assumes) local server with in-memory backend
2. Creates two accounts via gRPC
3. Runs the concurrent client with configurable goroutines/transfers (including deliberate duplicate idempotency keys)
4. Prints final balances and invariant proof

Client flag contract (stable across both phases):

| Flag | Type | Required | Default | Description | Example |
|---|---|---|---|---|---|
| `--backend-url` | `string` | No | `localhost:50051` | gRPC address for the running ledger server | `--backend-url=localhost:50051` |
| `--goroutines` | `int` | No | `50` | Number of concurrent worker goroutines issuing transactions | `--goroutines=100` |
| `--transfers` | `int` | No | `200` | Total number of `PostTransaction` requests to send | `--transfers=1000` |
| `--duplicate-keys` | `int` | No | `20` | Number of requests that intentionally reuse an idempotency key | `--duplicate-keys=0` |

Validation rules:
- `--goroutines` must be `>= 1`
- `--transfers` must be `>= 1`
- `--duplicate-keys` must be `>= 0` and `<= --transfers`

Canonical demo parameters (used by `make demo` in both Phase 5 and Phase 6):

| Make parameter | Type | Default | Forwards to | Notes |
|---|---|---|---|---|
| `BACKEND_URL` | `string` | `localhost:50051` | Client `--backend-url` | gRPC server address |
| `GOROUTINES` | `int` | `50` | Client `--goroutines` | Concurrent workers |
| `TRANSFERS` | `int` | `200` | Client `--transfers` | Total requests |
| `DUPLICATE_KEYS` | `int` | `20` | Client `--duplicate-keys` | Intentional idempotency duplicates |
| `DB_TYPE` | `string` | `memory` | Server datastore setting | `memory` in Phase 5; `postgres` in Phase 6 |
| `RUNTIME` | `string` | `local` | Make orchestration only | `local` in Phase 5; `compose` in Phase 6 |

### Definition of Done
- [ ] `cmd/client/main.go` accepts `--backend-url`, `--goroutines`, `--transfers`, and `--duplicate-keys` flags
- [ ] Client uses goroutines + `sync.WaitGroup` to fire concurrent gRPC requests
- [ ] Client prints a clear summary: requests sent, successes, duplicates detected, final balances, invariant result
- [ ] `make run` starts server with the in-memory backend and structured startup logs
- [ ] `make demo` accepts `BACKEND_URL`, `GOROUTINES`, `TRANSFERS`, `DUPLICATE_KEYS`, `DB_TYPE`, and `RUNTIME` (Phase 5 uses `DB_TYPE=memory`, `RUNTIME=local`)
- [ ] `make demo` runs the local end-to-end scenario and exits with code 0
- [ ] `README.md` updated with a "Local Demo (In-Memory)" section (no Docker prerequisite for this phase)

### Verification
```
make demo
```

Expected output:
```
[demo] Starting local server (db-type=memory)...
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

## Phase 6: Docker-based Demo with Postgres DB

### User Story
*As a developer preparing a portfolio-ready demo, I want one containerized workflow that can run the same client scenario against either in-memory or Postgres backends so that I can clearly prove interface-driven design, real ACID behavior, and operational simplicity.*

### Context
Phase 5 proved the demo flow locally. Phase 6 packages that exact flow for reproducible execution in any environment and a live postgres database.

Key outcomes:

**1. Dockerized runtime**
- `Dockerfile` uses a multi-stage build (builder + minimal runtime image)
- `docker-compose.yml` starts `server` and (when needed) `postgres` with healthchecks
- Compose uses host port 50053 (not 50051) so it does not conflict with a local `make run`

**2. Explicit datastore selection**
- Server datastore selected via environment variable: `LEDGER_DB_TYPE=memory|postgres`
- Compose wiring passes `DB_TYPE` clearly and predictably

**3. Single demo entrypoint with parameters**
Use `make demo` as the canonical command and pass parameters from the same contract introduced in Phase 5:
```bash
# Local server process (Phase 5 behavior)
make demo RUNTIME=local DB_TYPE=memory

# Containerized server, memory datastore
make demo RUNTIME=compose DB_TYPE=memory

# Containerized server + Postgres datastore (portfolio default)
make demo RUNTIME=compose DB_TYPE=postgres
```

The same client scenario runs in all modes; only infrastructure changes.

`make demo` parameter contract:

| Parameter | Type | Required | Default | Allowed values | Example |
|---|---|---|---|---|---|
| `BACKEND_URL` | `string` | No | `localhost:50051` | Any `host:port` gRPC target | `make demo BACKEND_URL=127.0.0.1:50051` |
| `GOROUTINES` | `int` | No | `50` | `>= 1` | `make demo GOROUTINES=100` |
| `TRANSFERS` | `int` | No | `200` | `>= 1` | `make demo TRANSFERS=1000` |
| `DUPLICATE_KEYS` | `int` | No | `20` | `0..TRANSFERS` | `make demo DUPLICATE_KEYS=0` |
| `DB_TYPE` | `string` | No | `memory` | `memory`, `postgres` | `make demo RUNTIME=compose DB_TYPE=postgres` |
| `RUNTIME` | `string` | No | `local` | `local`, `compose` | `make demo RUNTIME=compose DB_TYPE=memory` |

Forwarding and consistency rules:
- `BACKEND_URL`, `GOROUTINES`, `TRANSFERS`, `DUPLICATE_KEYS` map 1:1 to client flags (`--backend-url`, `--goroutines`, `--transfers`, `--duplicate-keys`)
- `DB_TYPE` controls server datastore in both local and compose runtime
- `RUNTIME` only selects orchestration path (`local` process vs `compose` stack); it is intentionally not a client/server flag

### Definition of Done
- [ ] `Dockerfile` builds a small production-style image for `cmd/server`
- [ ] `docker-compose.yml` supports `DB_TYPE=memory|postgres` (default `memory`) and only requires Postgres when `DB_TYPE=postgres` (via profiles)
- [ ] Compose healthchecks ensure server starts only after required dependencies are ready
- [ ] `make demo` accepts the same parameter set defined in Phase 5 and runs one consistent scenario flow
- [ ] `DB_TYPE=postgres` path demonstrates real DB behavior and logs transaction success under concurrency
- [ ] `README.md` updated with a concise matrix of demo commands and expected outcomes

**Implementation notes:** Server runs migrations on startup when `LEDGER_DB_TYPE=postgres`. Postgres idempotency handles concurrent duplicate requests (unique-violation race) by fetching the cached result instead of returning an error.

### Verification
```bash
make demo RUNTIME=compose DB_TYPE=postgres
```

Expected output:
```
[demo] Starting Docker Compose stack (db-type=postgres)...
[demo] Waiting for server on port 50053...
[demo] Waiting for gRPC to be ready...
[demo] Creating accounts A and B...
[demo] Firing 200 transfers across 50 goroutines (20 duplicate idempotency keys)...
[demo] Results:
  Transfers executed:  180
  Duplicates detected: 20
  Final balance A:     182000000 micros
  Final balance B:     18000000 micros
  Sum invariant:       PASS (expected=200000000, got=200000000)
[demo] All checks passed.
```

---

## Phase 7: Idempotency Refinements

### User Story
*As a developer, I want consistent idempotency behavior across store implementations and response metadata that lets clients correlate responses with requests without mixing correlation data into domain errors.*

### Context
Phase 4 introduced idempotency. This phase refines it in two ways:

1. **Shared idempotency package** — Extract error code mapping and a shared test harness into `internal/idempotency/`. Both memory and postgres stores run the same idempotency scenarios, reducing drift and ensuring identical behavior. Memory continues to store raw errors; postgres uses the shared codes for DB serialization.

2. **Response metadata** — When `idempotency_key` is present in a request, the server sets `x-idempotency-key` in the gRPC response header (success or error). Clients can correlate responses with requests without changing domain error messages. Correlation stays in transport metadata; domain data stays clean.

### Definition of Done
- [ ] `internal/idempotency/codes.go` — error code mapping (domain ↔ serializable codes) used by postgres store
- [ ] `internal/idempotency/testing.go` — `RunIdempotencyTests(t, ledger)` harness covering same-key dedup, distinct keys, cached failure, concurrent same key
- [ ] Memory and postgres tests call shared harness; duplicated idempotency tests removed
- [ ] `PostTransaction` sets `x-idempotency-key` response header when key present, for both success and error
- [ ] gRPC test asserts header is present when `idempotency_key` is set

### Verification
```
go test -race ./...
go test -race -tags integration ./...
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
| 5 | Demo Client (Local) | None | Concurrency client, invariant-focused storytelling | `make demo` (local memory) |
| 6 | Demo Packaging | Docker Compose | Container runtime, datastore selection, reproducible showcase | `make demo RUNTIME=compose DB_TYPE=postgres` |
| 7 | Idempotency Refinements | None | Shared test harness, response metadata, separation of concerns | `go test` (idempotency header) |
