package memory

import (
	"errors"
	"sync"
	"testing"

	"github.com/lindseycarriere/goledger/internal/domain"
	"github.com/lindseycarriere/goledger/internal/idempotency"
)

func TestStore_PostTransfer_Success(t *testing.T) {
	store := NewStore()
	if err := store.CreateAccount("A", 100_000_000); err != nil {
		t.Fatalf("CreateAccount A: %v", err)
	}
	if err := store.CreateAccount("B", 0); err != nil {
		t.Fatalf("CreateAccount B: %v", err)
	}

	if err := store.PostTransfer("", "A", "B", 50_000_000); err != nil {
		t.Fatalf("PostTransfer: %v", err)
	}

	balA, err := store.GetBalance("A")
	if err != nil {
		t.Fatalf("GetBalance A: %v", err)
	}
	balB, err := store.GetBalance("B")
	if err != nil {
		t.Fatalf("GetBalance B: %v", err)
	}

	if balA != 50_000_000 {
		t.Errorf("balance A = %d, want 50000000", balA)
	}
	if balB != 50_000_000 {
		t.Errorf("balance B = %d, want 50000000", balB)
	}
	if balA+balB != 100_000_000 {
		t.Errorf("sum invariant violated: %d + %d != 100000000", balA, balB)
	}
}

func TestStore_PostTransfer_InsufficientFunds(t *testing.T) {
	store := NewStore()
	if err := store.CreateAccount("A", 10_000); err != nil {
		t.Fatalf("CreateAccount A: %v", err)
	}
	if err := store.CreateAccount("B", 0); err != nil {
		t.Fatalf("CreateAccount B: %v", err)
	}

	err := store.PostTransfer("", "A", "B", 20_000)
	if err == nil {
		t.Fatal("PostTransfer expected error for insufficient funds")
	}
	if !errors.Is(err, domain.ErrInsufficientFunds) {
		t.Errorf("PostTransfer err = %v, want ErrInsufficientFunds", err)
	}

	balA, _ := store.GetBalance("A")
	balB, _ := store.GetBalance("B")
	if balA != 10_000 || balB != 0 {
		t.Errorf("balances should be unchanged: A=%d B=%d", balA, balB)
	}
}

func TestStore_PostTransfer_SelfTransfer(t *testing.T) {
	store := NewStore()
	if err := store.CreateAccount("A", 100_000); err != nil {
		t.Fatalf("CreateAccount A: %v", err)
	}

	err := store.PostTransfer("", "A", "A", 50_000)
	if err == nil {
		t.Fatal("PostTransfer expected error for self-transfer")
	}
	if !errors.Is(err, domain.ErrSelfTransfer) {
		t.Errorf("PostTransfer err = %v, want ErrSelfTransfer", err)
	}

	balA, _ := store.GetBalance("A")
	if balA != 100_000 {
		t.Errorf("balance A should be unchanged: %d", balA)
	}
}

func TestStore_PostTransfer_InvalidAmount(t *testing.T) {
	store := NewStore()
	if err := store.CreateAccount("A", 100_000); err != nil {
		t.Fatalf("CreateAccount A: %v", err)
	}
	if err := store.CreateAccount("B", 0); err != nil {
		t.Fatalf("CreateAccount B: %v", err)
	}

	for _, amount := range []int64{0, -100} {
		err := store.PostTransfer("", "A", "B", amount)
		if err == nil {
			t.Errorf("PostTransfer(%d) expected error", amount)
		}
		if !errors.Is(err, domain.ErrInvalidAmount) {
			t.Errorf("PostTransfer(%d) err = %v, want ErrInvalidAmount", amount, err)
		}
	}

	balA, _ := store.GetBalance("A")
	balB, _ := store.GetBalance("B")
	if balA != 100_000 || balB != 0 {
		t.Errorf("balances should be unchanged: A=%d B=%d", balA, balB)
	}
}

func TestStore_ConcurrentTransfers_Stress(t *testing.T) {
	store := NewStore()
	if err := store.CreateAccount("A", 100_000_000); err != nil {
		t.Fatalf("CreateAccount A: %v", err)
	}
	if err := store.CreateAccount("B", 0); err != nil {
		t.Fatalf("CreateAccount B: %v", err)
	}

	initialSum := int64(100_000_000)
	numGoroutines := 100
	transfersPerGoroutine := 10
	amountPerTransfer := int64(1_000)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < transfersPerGoroutine; j++ {
				// Alternate direction to avoid draining one account
				if j%2 == 0 {
					_ = store.PostTransfer("", "A", "B", amountPerTransfer)
				} else {
					_ = store.PostTransfer("", "B", "A", amountPerTransfer)
				}
			}
		}()
	}
	wg.Wait()

	balA, _ := store.GetBalance("A")
	balB, _ := store.GetBalance("B")
	finalSum := balA + balB

	if finalSum != initialSum {
		t.Errorf("sum invariant violated: initial=%d, final=%d (A=%d B=%d)",
			initialSum, finalSum, balA, balB)
	}
}

func TestStore_CreateAccount_Duplicate(t *testing.T) {
	store := NewStore()
	if err := store.CreateAccount("A", 100); err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	err := store.CreateAccount("A", 200)
	if err == nil {
		t.Fatal("CreateAccount expected error for duplicate")
	}
	if !errors.Is(err, domain.ErrAccountExists) {
		t.Errorf("CreateAccount err = %v, want ErrAccountExists", err)
	}
}

func TestStore_GetBalance_NotFound(t *testing.T) {
	store := NewStore()
	_, err := store.GetBalance("X")
	if err == nil {
		t.Fatal("GetBalance expected error for missing account")
	}
	if !errors.Is(err, domain.ErrAccountNotFound) {
		t.Errorf("GetBalance err = %v, want ErrAccountNotFound", err)
	}
}

func TestStore_PostTransfer_Idempotency(t *testing.T) {
	idempotency.RunIdempotencyTests(t, func(*testing.T) domain.Ledger { return NewStore() })
}
