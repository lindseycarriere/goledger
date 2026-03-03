# gRPC End-to-End Testing

This guide covers manual verification of the gRPC API using `grpcurl`.

## Prerequisites

### Install grpcurl CLI

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

Note: If your shell can't find `grpcurl` after install, add Go's bin directory to your PATH:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

To make this permanent in zsh, add to `~/.zshrc`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
```

## Running the gRPC Server

```bash
make run
```

Note: The server enables the gRPC Reflection API, so `grpcurl` can discover services and message schemas at runtime without needing local `.proto` files.

Expected output:

```json
{"time":"...","level":"INFO","msg":"gRPC server listening","addr":"[::]:50051"}
```

The server will log structured logs such as:

```json
{"time":"2026-03-03T14:33:58.990635666-07:00","level":"INFO","msg":"RPC completed","method":"/ledger.v1.LedgerService/CreateAccount","duration_ms":0,"code":"OK","err":null}
{"time":"2026-03-03T14:34:26.468752449-07:00","level":"INFO","msg":"RPC completed","method":"/ledger.v1.LedgerService/PostTransaction","duration_ms":0,"code":"OK","err":null}
{"time":"2026-03-03T14:35:26.011137587-07:00","level":"INFO","msg":"RPC completed","method":"/ledger.v1.LedgerService/GetBalance","duration_ms":0,"code":"NotFound","err":"rpc error: code = NotFound desc = account not found"}
```

## RPC API Verification

In a new terminal, run the `grpcurl` commands below.

### List services

```bash
grpcurl -plaintext localhost:50051 list
```

### Create accounts

```bash
# Create account A with 100000000 micros (100.00)
grpcurl -plaintext -d '{"account_id":"A","initial_balance_micros":100000000}' localhost:50051 ledger.v1.LedgerService/CreateAccount

# Create account B with 0
grpcurl -plaintext -d '{"account_id":"B","initial_balance_micros":0}' localhost:50051 ledger.v1.LedgerService/CreateAccount
```

### Get balance

```bash
grpcurl -plaintext -d '{"account_id":"A"}' localhost:50051 ledger.v1.LedgerService/GetBalance
```

Expected: `{"balanceMicros":"100000000"}`

### Post transaction

```bash
grpcurl -plaintext -d '{"from":"A","to":"B","amount_micros":50000000}' localhost:50051 ledger.v1.LedgerService/PostTransaction
```

### Verify transfer

```bash
grpcurl -plaintext -d '{"account_id":"A"}' localhost:50051 ledger.v1.LedgerService/GetBalance
grpcurl -plaintext -d '{"account_id":"B"}' localhost:50051 ledger.v1.LedgerService/GetBalance
```

Expected: A=50000000, B=50000000, sum invariant 100000000.

## Error Cases

### Account not found

```bash
grpcurl -plaintext -d '{"account_id":"X"}' localhost:50051 ledger.v1.LedgerService/GetBalance
```

Expected: `Code: NotFound`, `account not found`

### Duplicate account

```bash
grpcurl -plaintext -d '{"account_id":"A","initial_balance_micros":0}' localhost:50051 ledger.v1.LedgerService/CreateAccount
```

Expected: `Code: AlreadyExists`, `Message: account already exists`

### Insufficient funds

```bash
grpcurl -plaintext -d '{"from":"A","to":"B","amount_micros":999999999}' localhost:50051 ledger.v1.LedgerService/PostTransaction
```

Expected: `Code: FailedPrecondition`, `Message: insufficient funds: balance=50000000, debit=999999999`
