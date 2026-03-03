//go:build integration

package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lindseycarriere/goledger/internal/domain"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// setupStore creates a Store backed by a fresh pool, registers restore and pool cleanup on t.
// Call from within a subtest that has t.Cleanup(restore) registered first so DB is reset after each test.
func setupStore(t *testing.T, connStr string, restore func()) *Store {
	t.Helper()
	t.Cleanup(restore)
	pool, err := pgxpool.New(context.Background(), connStr)
	if err != nil {
		t.Fatalf("pgxpool new: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewStore(pool)
}

/*
Use a single testcontainer to run all the simple integration tests sequentially with T.run(...) calls.
  - Leverages snapshot/restore which is much lower overhead then new containers for each test.
  - Use separate containers when you need parallelism, different configs, or stronger isolation, and you accept higher startup cost. Consdier this decision tree:

Do all tests need the same container type & config?

	├── No (e.g., Postgres 14 vs 16, different envs)
	│   └── Use separate TestXxx functions, each with its own container
	└── Yes
		└── Is snapshot/restore (or equivalent) available?
			├── No (Redis, Kafka, etc.)
			│   └── Trade-off: separate containers (parallel, isolated) vs shared (faster startup)
			└── Yes (Postgres, etc.)
				└── Does parallel execution matter more than startup cost?
					├── Yes (e.g., very fast tests, many of them)
					│   └── Consider separate TestXxx + per-test containers
					│       (parallel, but N× container startup)
					└── No (typical case)
						└── Use shared container + t.Run subtests
*/
func TestPostgresStore(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("ledger"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	testcontainers.CleanupContainer(t, ctr)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}

	connStr, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	if err := runMigrations(connStr); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if err := ctr.Snapshot(ctx); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	restore := func() {
		if err := ctr.Restore(ctx); err != nil {
			t.Logf("restore: %v", err)
		}
	}

	t.Run("PostTransfer_Success", func(t *testing.T) {
		store := setupStore(t, connStr, restore)

		if err := store.CreateAccount("A", 100_000_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		if err := store.CreateAccount("B", 0); err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}
		if err := store.PostTransfer("A", "B", 50_000_000); err != nil {
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
		if balA != 50_000_000 || balB != 50_000_000 {
			t.Errorf("balances: A=%d B=%d", balA, balB)
		}
		if balA+balB != 100_000_000 {
			t.Errorf("sum invariant violated: %d + %d != 100000000", balA, balB)
		}
	})

	t.Run("PostTransfer_InsufficientFunds", func(t *testing.T) {
		store := setupStore(t, connStr, restore)

		if err := store.CreateAccount("A", 10_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		if err := store.CreateAccount("B", 0); err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}
		err := store.PostTransfer("A", "B", 20_000)
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
	})

	t.Run("PostTransfer_SelfTransfer", func(t *testing.T) {
		store := setupStore(t, connStr, restore)

		if err := store.CreateAccount("A", 100_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		err := store.PostTransfer("A", "A", 50_000)
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
	})

	t.Run("PostTransfer_InvalidAmount", func(t *testing.T) {
		store := setupStore(t, connStr, restore)

		if err := store.CreateAccount("A", 100_000); err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		if err := store.CreateAccount("B", 0); err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}
		for _, amount := range []int64{0, -100} {
			err := store.PostTransfer("A", "B", amount)
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
	})

	t.Run("CreateAccount_Duplicate", func(t *testing.T) {
		store := setupStore(t, connStr, restore)

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
	})

	t.Run("GetBalance_NotFound", func(t *testing.T) {
		store := setupStore(t, connStr, restore)

		_, err := store.GetBalance("X")
		if err == nil {
			t.Fatal("GetBalance expected error for missing account")
		}
		if !errors.Is(err, domain.ErrAccountNotFound) {
			t.Errorf("GetBalance err = %v, want ErrAccountNotFound", err)
		}
	})

	t.Run("ConcurrentTransfers_Stress", func(t *testing.T) {
		store := setupStore(t, connStr, restore)

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
					if j%2 == 0 {
						_ = store.PostTransfer("A", "B", amountPerTransfer)
					} else {
						_ = store.PostTransfer("B", "A", amountPerTransfer)
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
		t.Logf("stress: %d goroutines, %d transfers, 0 invariant violations",
			numGoroutines, numGoroutines*transfersPerGoroutine)
	})
}
