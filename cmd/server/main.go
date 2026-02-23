package main

import (
	"log/slog"
	"os"

	"github.com/lindseycarriere/goledger/internal/store/memory"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	store := memory.NewStore()

	if err := store.CreateAccount("A", 100_000_000); err != nil {
		slog.Error("create account A", "err", err)
		os.Exit(1)
	}
	if err := store.CreateAccount("B", 0); err != nil {
		slog.Error("create account B", "err", err)
		os.Exit(1)
	}

	balA, _ := store.GetBalance("A")
	balB, _ := store.GetBalance("B")
	slog.Info("Initial", "account:A", balA, "account:B", balB)

	amount := int64(50_000_000)
	if err := store.PostTransfer("A", "B", amount); err != nil {
		slog.Error("transfer failed", "err", err)
		os.Exit(1)
	}
	slog.Info("Transfer", "from", "A", "to", "B", "amount_micros", amount)

	balA, _ = store.GetBalance("A")
	balB, _ = store.GetBalance("B")
	slog.Info("Final", "account:A", balA, "account:B", balB)

	total := balA + balB
	expected := int64(100_000_000)
	if total == expected {
		slog.Info("Sum invariant: PASS", "total", total)
	} else {
		slog.Error("Sum invariant: FAIL", "expected", expected, "got", total)
		os.Exit(1)
	}
}
