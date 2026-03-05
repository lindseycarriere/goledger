package memory

import (
	"fmt"
	"sync"

	"github.com/lindseycarriere/goledger/internal/domain"
)

type accountState struct {
	balance int64
	mu      sync.Mutex
}

// idempotencyResult stores the outcome of a completed transfer for replay on duplicate requests.
type idempotencyResult struct {
	err error // nil = success
}

// Store implements domain.Ledger using an in-memory map with per-account mutexes.
// Lock ordering (lower account ID first) prevents deadlock when transferring between accounts.
type Store struct {
	mu             sync.Mutex
	accounts       map[string]*accountState
	idempotencyMu  sync.Mutex
	idempotencyMap map[string]*idempotencyResult
}

// NewStore returns a new in-memory ledger store.
func NewStore() *Store {
	return &Store{
		accounts:       make(map[string]*accountState),
		idempotencyMap: make(map[string]*idempotencyResult),
	}
}

// CreateAccount creates an account with the given ID and initial balance.
func (s *Store) CreateAccount(id string, initialBalance int64) error {
	if initialBalance < 0 {
		return fmt.Errorf("initial balance cannot be negative: %d", initialBalance)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.accounts[id]; exists {
		return domain.ErrAccountExists
	}
	s.accounts[id] = &accountState{balance: initialBalance}
	return nil
}

// GetBalance returns the balance of the account in micros.
func (s *Store) GetBalance(id string) (int64, error) {
	s.mu.Lock()
	acc, exists := s.accounts[id]
	s.mu.Unlock()
	if !exists {
		return 0, domain.ErrAccountNotFound
	}
	acc.mu.Lock()
	defer acc.mu.Unlock()
	return acc.balance, nil
}

// PostTransfer moves amount micros from from to to. idempotencyKey is optional:
// when non-empty, duplicate requests return the cached result without re-executing.
func (s *Store) PostTransfer(idempotencyKey string, from, to string, amount int64) error {
	if idempotencyKey == "" {
		return s.doTransfer(from, to, amount)
	}

	s.idempotencyMu.Lock()
	defer s.idempotencyMu.Unlock()

	if cached, ok := s.idempotencyMap[idempotencyKey]; ok {
		return cached.err
	}

	err := s.doTransfer(from, to, amount)
	s.idempotencyMap[idempotencyKey] = &idempotencyResult{err: err}
	return err
}

// doTransfer performs the core transfer logic. Lock ordering prevents deadlock when transferring A<->B.
func (s *Store) doTransfer(from, to string, amount int64) error {
	if amount <= 0 {
		return domain.ErrInvalidAmount
	}
	if from == to {
		return domain.ErrSelfTransfer
	}

	s.mu.Lock()
	fromAcc, fromExists := s.accounts[from]
	toAcc, toExists := s.accounts[to]
	s.mu.Unlock()

	if !fromExists {
		return fmt.Errorf("%w: %s", domain.ErrAccountNotFound, from)
	}
	if !toExists {
		return fmt.Errorf("%w: %s", domain.ErrAccountNotFound, to)
	}

	// Go: consistent lock ordering prevents deadlock when two goroutines transfer A<->B
	first, second := fromAcc, toAcc
	if from > to {
		first, second = toAcc, fromAcc
	}
	first.mu.Lock()
	defer first.mu.Unlock()
	second.mu.Lock()
	defer second.mu.Unlock()

	if fromAcc.balance < amount {
		return fmt.Errorf("%w: balance=%d, debit=%d", domain.ErrInsufficientFunds, fromAcc.balance, amount)
	}

	fromAcc.balance -= amount
	toAcc.balance += amount
	return nil
}

// Ensure Store implements domain.Ledger at compile time.
var _ domain.Ledger = (*Store)(nil)
