package domain

import "errors"

// Sentinel errors for Ledger operations. Both in-memory and postgres stores return these.
var (
	ErrAccountExists     = errors.New("account already exists")
	ErrAccountNotFound   = errors.New("account not found")
	ErrInvalidAmount     = errors.New("amount must be positive")
	ErrSelfTransfer      = errors.New("cannot transfer to same account")
	ErrInsufficientFunds = errors.New("insufficient funds")
)
