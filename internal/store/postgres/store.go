package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

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

// PostTransfer moves amount micros from from to to. idempotencyKey is optional:
// when non-empty, duplicate requests return the cached result without re-executing.
func (s *Store) PostTransfer(idempotencyKey string, from, to string, amount int64) error {
	if idempotencyKey == "" {
		return s.postTransferNoIdempotency(from, to, amount)
	}

	ctx := context.Background()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)

	cached, err := q.GetIdempotencyResult(ctx, idempotencyKey)
	if err == nil {
		return codeToDomainErr(cached.ErrorCode, cached.ErrorDetail)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	// Key not seen; execute transfer and store result atomically.
	transferErr := s.doTransferInTx(ctx, q, from, to, amount)

	code, detail := domainErrToCode(transferErr)
	if err := q.InsertIdempotencyKey(ctx, db.InsertIdempotencyKeyParams{
		Key:         idempotencyKey,
		ErrorCode:   code,
		ErrorDetail: detail,
	}); err != nil {
		return err
	}

	// Commit to persist the idempotency record (success or failure) before returning.
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return transferErr
}

// postTransferNoIdempotency performs a transfer without idempotency (single DB transaction).
func (s *Store) postTransferNoIdempotency(from, to string, amount int64) error {
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
	defer func() { _ = tx.Rollback(ctx) }()

	q := s.queries.WithTx(tx)
	if err := s.doTransferInTx(ctx, q, from, to, amount); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// doTransferInTx performs the transfer logic within an existing transaction.
func (s *Store) doTransferInTx(ctx context.Context, q *db.Queries, from, to string, amount int64) error {
	if amount <= 0 {
		return domain.ErrInvalidAmount
	}
	if from == to {
		return domain.ErrSelfTransfer
	}

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
	return nil
}

const (
	idemCodeOK                = "ok"
	idemCodeAccountNotFound   = "account_not_found"
	idemCodeInsufficientFunds = "insufficient_funds"
	idemCodeInvalidAmount     = "invalid_amount"
	idemCodeSelfTransfer      = "self_transfer"
)

func domainErrToCode(err error) (code, detail string) {
	if err == nil {
		return idemCodeOK, ""
	}
	switch {
	case errors.Is(err, domain.ErrAccountNotFound):
		// Extract account id from wrapped error message (e.g. "account not found: A")
		detail = strings.TrimPrefix(err.Error(), domain.ErrAccountNotFound.Error()+": ")
		return idemCodeAccountNotFound, detail
	case errors.Is(err, domain.ErrInsufficientFunds):
		return idemCodeInsufficientFunds, ""
	case errors.Is(err, domain.ErrInvalidAmount):
		return idemCodeInvalidAmount, ""
	case errors.Is(err, domain.ErrSelfTransfer):
		return idemCodeSelfTransfer, ""
	default:
		return "unknown", err.Error()
	}
}

func codeToDomainErr(code, detail string) error {
	switch code {
	case idemCodeOK:
		return nil
	case idemCodeAccountNotFound:
		return fmt.Errorf("%w: %s", domain.ErrAccountNotFound, detail)
	case idemCodeInsufficientFunds:
		return domain.ErrInsufficientFunds
	case idemCodeInvalidAmount:
		return domain.ErrInvalidAmount
	case idemCodeSelfTransfer:
		return domain.ErrSelfTransfer
	default:
		return fmt.Errorf("idempotency replay: %s", code)
	}
}
