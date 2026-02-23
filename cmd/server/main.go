package main

import (
	"log/slog"
	"os"
)

func main() {
	// Set up structured logging with JSON output
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("goledger service starting", "version", "0.1.0")
}