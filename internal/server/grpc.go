package server

import (
	"context"
	"errors"
	"log/slog"
	"runtime/debug"
	"time"

	ledgerv1 "github.com/lindseycarriere/goledger/gen/go/ledger/v1"
	"github.com/lindseycarriere/goledger/internal/domain"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server implements ledgerv1.LedgerServiceServer by delegating to domain.Ledger.
type Server struct {
	ledgerv1.UnimplementedLedgerServiceServer
	ledger domain.Ledger
}

// NewServer returns a gRPC server implementation backed by the given Ledger.
func NewServer(ledger domain.Ledger) *Server {
	return &Server{ledger: ledger}
}

// CreateAccount creates an account with the given ID and initial balance.
func (s *Server) CreateAccount(ctx context.Context, req *ledgerv1.CreateAccountRequest) (*ledgerv1.CreateAccountResponse, error) {
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	if err := s.ledger.CreateAccount(req.AccountId, req.InitialBalanceMicros); err != nil {
		return nil, domainErrToStatus(err)
	}
	return &ledgerv1.CreateAccountResponse{}, nil
}

// GetBalance returns the account balance in micros.
func (s *Server) GetBalance(ctx context.Context, req *ledgerv1.GetBalanceRequest) (*ledgerv1.GetBalanceResponse, error) {
	if req.AccountId == "" {
		return nil, status.Error(codes.InvalidArgument, "account_id is required")
	}
	balance, err := s.ledger.GetBalance(req.AccountId)
	if err != nil {
		return nil, domainErrToStatus(err)
	}
	return &ledgerv1.GetBalanceResponse{BalanceMicros: balance}, nil
}

// PostTransaction transfers amount_micros from from to to.
// When idempotency_key is set, duplicate requests return the cached result.
func (s *Server) PostTransaction(ctx context.Context, req *ledgerv1.PostTransactionRequest) (*ledgerv1.PostTransactionResponse, error) {
	if req.From == "" || req.To == "" {
		return nil, status.Error(codes.InvalidArgument, "from and to are required")
	}
	if err := s.ledger.PostTransfer(req.IdempotencyKey, req.From, req.To, req.AmountMicros); err != nil {
		return nil, domainErrToStatus(err)
	}
	return &ledgerv1.PostTransactionResponse{}, nil
}

func domainErrToStatus(err error) error {
	switch {
	case errors.Is(err, domain.ErrAccountExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, domain.ErrAccountNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidAmount), errors.Is(err, domain.ErrSelfTransfer):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, domain.ErrInsufficientFunds):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// LoggingInterceptor logs each RPC with method, duration, and status.
func LoggingInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err)
		logger.Info("RPC completed", "method", info.FullMethod, "duration_ms", time.Since(start).Milliseconds(), "code", code.String(), "err", err)
		return resp, err
	}
}

// RecoveryInterceptor catches panics and returns INTERNAL instead of crashing.
func RecoveryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered", "method", info.FullMethod, "panic", r, "stack", string(debug.Stack()))
				err = status.Error(codes.Internal, "internal server error")
			}
		}()
		resp, err = handler(ctx, req)
		return resp, err
	}
}
