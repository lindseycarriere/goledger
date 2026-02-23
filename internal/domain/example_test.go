package domain

import (
	"testing"
)

func TestAccount_Debit(t *testing.T) {
	tests := []struct {
		name        string
		initialBal  int64
		debitAmount int64
		wantBalance int64
		wantError   bool
	}{
		{
			name:        "successful debit",
			initialBal:  1000000, // 1.00 in micros
			debitAmount: 500000,  // 0.50 in micros
			wantBalance: 500000,  // 0.50 remaining
			wantError:   false,
		},
		{
			name:        "insufficient funds",
			initialBal:  100000,  // 0.10 in micros
			debitAmount: 200000,  // 0.20 in micros
			wantBalance: 100000,  // unchanged
			wantError:   true,
		},
		{
			name:        "zero amount debit",
			initialBal:  1000000,
			debitAmount: 0,
			wantBalance: 1000000, // unchanged
			wantError:   true,
		},
		{
			name:        "negative amount debit",
			initialBal:  1000000,
			debitAmount: -100000,
			wantBalance: 1000000, // unchanged
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := &Account{ID: "test", Balance: tt.initialBal}
			err := acc.Debit(tt.debitAmount)

			if (err != nil) != tt.wantError {
				t.Errorf("Account.Debit() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if acc.Balance != tt.wantBalance {
				t.Errorf("Account.Debit() balance = %v, want %v", acc.Balance, tt.wantBalance)
			}
		})
	}
}

func TestAccount_Credit(t *testing.T) {
	tests := []struct {
		name        string
		initialBal  int64
		creditAmount int64
		wantBalance  int64
		wantError    bool
	}{
		{
			name:         "successful credit",
			initialBal:   1000000,
			creditAmount: 500000,
			wantBalance:  1500000,
			wantError:    false,
		},
		{
			name:         "zero amount credit",
			initialBal:   1000000,
			creditAmount: 0,
			wantBalance:  1000000, // unchanged
			wantError:    true,
		},
		{
			name:         "negative amount credit",
			initialBal:   1000000,
			creditAmount: -100000,
			wantBalance:  1000000, // unchanged
			wantError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := &Account{ID: "test", Balance: tt.initialBal}
			err := acc.Credit(tt.creditAmount)

			if (err != nil) != tt.wantError {
				t.Errorf("Account.Credit() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if acc.Balance != tt.wantBalance {
				t.Errorf("Account.Credit() balance = %v, want %v", acc.Balance, tt.wantBalance)
			}
		})
	}
}

func TestAccount_Process(t *testing.T) {
	acc := &Account{ID: "test", Balance: 1000000}

	// Test that Account implements Processor interface
	var p Processor = acc
	err := p.Process(500000)

	if err != nil {
		t.Errorf("Processor.Process() error = %v", err)
	}

	if acc.Balance != 1500000 {
		t.Errorf("Processor.Process() balance = %v, want %v", acc.Balance, 1500000)
	}
}