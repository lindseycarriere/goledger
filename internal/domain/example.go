package domain

import (
	"errors"
	"fmt"
)

// Account represents a financial account
type Account struct {
	ID      string
	Balance int64 // Balance in micros (1 unit = 0.000001 of currency)
}

// Processor defines the interface for processing financial operations
// Go: interfaces are satisfied implicitly - no "implements" keyword needed
type Processor interface {
	Process(amount int64) error
}

// Go: pointer receiver allows modification of the struct
func (a *Account) Debit(amount int64) error {
	if amount <= 0 {
		return errors.New("debit amount must be positive")
	}
	if a.Balance < amount {
		return fmt.Errorf("insufficient funds: balance=%d, debit=%d", a.Balance, amount)
	}
	a.Balance -= amount
	return nil
}

// Credit adds funds to the account
func (a *Account) Credit(amount int64) error {
	if amount <= 0 {
		return errors.New("credit amount must be positive")
	}
	a.Balance += amount
	return nil
}

// Process implements the Processor interface
func (a *Account) Process(amount int64) error {
	return a.Credit(amount)
}