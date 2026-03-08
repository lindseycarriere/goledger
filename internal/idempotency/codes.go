package idempotency

import (
	"errors"
	"fmt"
	"strings"

	"github.com/lindseycarriere/goledger/internal/domain"
)

// Serialization codes for idempotency storage (e.g. postgres idempotency_keys table).
const (
	CodeOK                = "ok"
	CodeAccountNotFound   = "account_not_found"
	CodeInsufficientFunds = "insufficient_funds"
	CodeInvalidAmount     = "invalid_amount"
	CodeSelfTransfer      = "self_transfer"
)

// DomainErrToCode maps a domain error to (code, detail) for persistence.
func DomainErrToCode(err error) (code, detail string) {
	if err == nil {
		return CodeOK, ""
	}
	switch {
	case errors.Is(err, domain.ErrAccountNotFound):
		detail = strings.TrimPrefix(err.Error(), domain.ErrAccountNotFound.Error()+": ")
		return CodeAccountNotFound, detail
	case errors.Is(err, domain.ErrInsufficientFunds):
		return CodeInsufficientFunds, ""
	case errors.Is(err, domain.ErrInvalidAmount):
		return CodeInvalidAmount, ""
	case errors.Is(err, domain.ErrSelfTransfer):
		return CodeSelfTransfer, ""
	default:
		return "unknown", err.Error()
	}
}

// CodeToDomainErr maps a stored (code, detail) back to a domain error.
func CodeToDomainErr(code, detail string) error {
	switch code {
	case CodeOK:
		return nil
	case CodeAccountNotFound:
		return fmt.Errorf("%w: %s", domain.ErrAccountNotFound, detail)
	case CodeInsufficientFunds:
		return domain.ErrInsufficientFunds
	case CodeInvalidAmount:
		return domain.ErrInvalidAmount
	case CodeSelfTransfer:
		return domain.ErrSelfTransfer
	default:
		return fmt.Errorf("idempotency replay: %s", code)
	}
}
