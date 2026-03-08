package idempotency

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/lindseycarriere/goledger/internal/domain"
)

// RunIdempotencyTests runs the standard idempotency scenarios against any domain.Ledger implementation.
// newLedger is called once per subtest so each scenario gets a fresh store. The subtest's t is passed
// so that cleanup (e.g. DB restore) runs when that subtest ends, not when the whole suite ends.
func RunIdempotencyTests(t *testing.T, newLedger func(t *testing.T) domain.Ledger) {
	t.Helper()

	t.Run("SameKeyDeduplicated", func(t *testing.T) {
		ledger := newLedger(t)
		if err := ledger.CreateAccount("A", 100_000_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		if err := ledger.CreateAccount("B", 0); err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}

		key := "idem-key-xyz"
		for i := 0; i < 10; i++ {
			err := ledger.PostTransfer(key, "A", "B", 5_000_000)
			if err != nil {
				t.Fatalf("PostTransfer attempt %d: %v", i+1, err)
			}
		}

		balA, _ := ledger.GetBalance("A")
		balB, _ := ledger.GetBalance("B")
		if balA != 95_000_000 || balB != 5_000_000 {
			t.Errorf("balance debited more than once: A=%d B=%d, want A=95000000 B=5000000", balA, balB)
		}
		if balA+balB != 100_000_000 {
			t.Errorf("sum invariant violated: %d + %d != 100000000", balA, balB)
		}
	})

	t.Run("DistinctKeysExecute", func(t *testing.T) {
		ledger := newLedger(t)
		if err := ledger.CreateAccount("A", 100_000_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		if err := ledger.CreateAccount("B", 0); err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}

		for i := 0; i < 3; i++ {
			key := fmt.Sprintf("idem-key-%d", i)
			err := ledger.PostTransfer(key, "A", "B", 10_000_000)
			if err != nil {
				t.Fatalf("PostTransfer %d: %v", i+1, err)
			}
		}

		balA, _ := ledger.GetBalance("A")
		balB, _ := ledger.GetBalance("B")
		if balA != 70_000_000 || balB != 30_000_000 {
			t.Errorf("three distinct keys should execute three transfers: A=%d B=%d", balA, balB)
		}
	})

	t.Run("CachedFailureReturnedOnRetry", func(t *testing.T) {
		ledger := newLedger(t)
		if err := ledger.CreateAccount("A", 100_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		// B does not exist

		key := "idem-fail-key"
		err1 := ledger.PostTransfer(key, "A", "B", 50_000)
		if err1 == nil {
			t.Fatal("PostTransfer expected error for missing account B")
		}
		if !errors.Is(err1, domain.ErrAccountNotFound) {
			t.Errorf("first call err = %v, want ErrAccountNotFound", err1)
		}

		if err := ledger.CreateAccount("B", 0); err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}
		err2 := ledger.PostTransfer(key, "A", "B", 50_000)
		if err2 == nil {
			t.Fatal("PostTransfer with cached-failure key expected error")
		}
		if !errors.Is(err2, domain.ErrAccountNotFound) {
			t.Errorf("retry err = %v, want cached ErrAccountNotFound", err2)
		}

		balA, _ := ledger.GetBalance("A")
		balB, _ := ledger.GetBalance("B")
		if balA != 100_000 || balB != 0 {
			t.Errorf("balances should be unchanged (cached failure): A=%d B=%d", balA, balB)
		}
	})

	t.Run("ConcurrentSameKeyAllSucceed", func(t *testing.T) {
		ledger := newLedger(t)
		if err := ledger.CreateAccount("A", 100_000_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		if err := ledger.CreateAccount("B", 0); err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}

		key := "idem-concurrent-race"
		amount := int64(5_000_000)
		numGoroutines := 20
		var wg sync.WaitGroup
		errs := make(chan error, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := ledger.PostTransfer(key, "A", "B", amount); err != nil {
					errs <- err
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Errorf("concurrent duplicate request returned error (race fix regressed): %v", err)
		}

		balA, _ := ledger.GetBalance("A")
		balB, _ := ledger.GetBalance("B")
		if balA != 95_000_000 || balB != 5_000_000 {
			t.Errorf("exactly one transfer should execute: A=%d B=%d, want A=95000000 B=5000000", balA, balB)
		}
		if balA+balB != 100_000_000 {
			t.Errorf("sum invariant violated: %d + %d != 100000000", balA, balB)
		}
	})
}
