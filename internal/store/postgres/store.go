package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lindseycarriere/goledger/internal/domain"
	"github.com/lindseycarriere/goledger/internal/store/postgres/db"
)

// PostgreSQL error code for unique constraint violation.
const pgCodeUniqueViolation = "23505"

// Store implements domain.Ledger using Postgres with row-level locking for concurrent transfers.
type Store struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewStore returns a new Postgres ledger store. Caller must call Close when done.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:    pool,
		queries: db.New(pool),
	}
}

// Close closes the pool. Safe to call multiple times.
func (s *Store) Close() {
	s.pool.Close()
}

// CreateAccount creates an account with the given ID and initial balance.
func (s *Store) CreateAccount(id string, initialBalance int64) error {
	if initialBalance < 0 {
		return fmt.Errorf("initial balance cannot be negative: %d", initialBalance)
	}
	ctx := context.Background()
	err := s.queries.CreateAccount(ctx, db.CreateAccountParams{
		ID:            id,
		BalanceMicros: initialBalance,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgCodeUniqueViolation {
			return domain.ErrAccountExists
		}
		return err
	}
	return nil
}

// GetBalance returns the balance of the account in micros.
func (s *Store) GetBalance(id string) (int64, error) {
	ctx := context.Background()
	balance, err := s.queries.GetBalance(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, domain.ErrAccountNotFound
		}
		return 0, err
	}
	return balance, nil
}

// PostTransfer moves amount micros from from to to using a single DB transaction with SELECT FOR UPDATE.
func (s *Store) PostTransfer(from, to string, amount int64) error {
	if amount <= 0 {
		return domain.ErrInvalidAmount
	}
	if from == to {
		return domain.ErrSelfTransfer
	}

	ctx := context.Background()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	// Go: defer ensures rollback if we return before Commit
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)

	// Lock both accounts in consistent order to avoid deadlock.
	firstID, secondID := from, to
	if from > to {
		firstID, secondID = to, from
	}

	first, err := q.GetAccountForUpdate(ctx, firstID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", domain.ErrAccountNotFound, firstID)
		}
		return err
	}
	second, err := q.GetAccountForUpdate(ctx, secondID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: %s", domain.ErrAccountNotFound, secondID)
		}
		return err
	}

	fromAcc, toAcc := first, second
	if firstID != from {
		fromAcc, toAcc = second, first
	}
	if fromAcc.BalanceMicros < amount {
		return fmt.Errorf("%w: balance=%d, debit=%d", domain.ErrInsufficientFunds, fromAcc.BalanceMicros, amount)
	}

	newFromBalance := fromAcc.BalanceMicros - amount
	newToBalance := toAcc.BalanceMicros + amount

	if err := q.UpdateBalance(ctx, db.UpdateBalanceParams{ID: from, BalanceMicros: newFromBalance}); err != nil {
		return err
	}
	if err := q.UpdateBalance(ctx, db.UpdateBalanceParams{ID: to, BalanceMicros: newToBalance}); err != nil {
		return err
	}
	if _, err := q.InsertEntry(ctx, db.InsertEntryParams{AccountID: from, AmountMicros: -amount}); err != nil {
		return err
	}
	if _, err := q.InsertEntry(ctx, db.InsertEntryParams{AccountID: to, AmountMicros: amount}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// Ensure Store implements domain.Ledger at compile time.
var _ domain.Ledger = (*Store)(nil)
