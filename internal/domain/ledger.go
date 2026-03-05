package domain

// Ledger defines the contract for double-entry bookkeeping operations.
// Implementations must ensure atomic transfers and consistent lock ordering to prevent deadlocks.
type Ledger interface {
	CreateAccount(id string, initialBalance int64) error
	GetBalance(id string) (int64, error)
	// PostTransfer moves amount micros between accounts (from -> to).
	// idempotencyKey is an optional string to identify the unique request.
	// When non-empty, duplicate requests return the cached result without re-executing.
	PostTransfer(idempotencyKey string, from, to string, amount int64) error
}
