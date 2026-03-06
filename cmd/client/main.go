package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/google/uuid"
	ledgerv1 "github.com/lindseycarriere/goledger/gen/go/ledger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	initialBalanceA int64 = 200_000_000
	initialBalanceB int64 = 0
)

func main() {
	backendURL := flag.String("backend-url", "localhost:50051", "gRPC address for the running ledger server")
	goroutines := flag.Int("goroutines", 50, "Number of concurrent worker goroutines issuing transactions")
	transfers := flag.Int("transfers", 200, "Total number of PostTransaction requests to send")
	duplicateKeys := flag.Int("duplicate-keys", 20, "Number of requests that intentionally reuse an idempotency key")
	flag.Parse()

	if err := validateFlags(*goroutines, *transfers, *duplicateKeys); err != nil {
		slog.Error("invalid flags", "err", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := grpc.NewClient(*backendURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("failed to connect", "addr", *backendURL, "err", err)
		os.Exit(1)
	}
	defer conn.Close()

	client := ledgerv1.NewLedgerServiceClient(conn)

	start := time.Now()
	fmt.Println("[demo] Creating accounts A and B...")
	if err := createAccounts(ctx, client); err != nil {
		slog.Error("failed to create accounts", "err", err)
		os.Exit(1)
	}

	fmt.Printf("[demo] Firing %d transfers across %d goroutines (%d duplicate idempotency keys)...\n",
		*transfers, *goroutines, *duplicateKeys)
	work := buildWork(*transfers, *duplicateKeys)
	executed, duplicates, failed := runConcurrentTransfers(ctx, client, work, *goroutines)

	balanceA, balanceB, err := fetchBalances(ctx, client)
	if err != nil {
		slog.Error("failed to fetch balances", "err", err)
		os.Exit(1)
	}

	expectedSum := initialBalanceA + initialBalanceB
	actualSum := balanceA + balanceB
	invariantOK := actualSum == expectedSum
	durationMs := time.Since(start).Milliseconds()

	transferAmount := int64(100_000)
	if len(work) > 0 {
		transferAmount = work[0].amount
	}
	fmt.Println("\n[demo] Initial state:")
	fmt.Printf("  Initial balance A:   %d micros\n", initialBalanceA)
	fmt.Printf("  Initial balance B:   %d micros\n", initialBalanceB)
	fmt.Printf("  Transfer amount:     %d micros (per transfer)\n", transferAmount)
	fmt.Printf("  Total transfers:     %d\n", *transfers)
	fmt.Printf("  Goroutines:          %d (concurrent transfers)\n", *goroutines)
	fmt.Printf("  Duplicate Requests:  %d\n", *duplicateKeys)

	fmt.Println("\n[demo] Results:")
	fmt.Printf("  Duration:            %d ms\n", durationMs)
	fmt.Printf("  Transfers executed:  %d\n", executed)
	fmt.Printf("  Duplicates detected: %d\n", duplicates)
	fmt.Printf("  Failed transfers:    %d\n", failed)
	fmt.Printf("  Final balance A:     %d micros\n", balanceA)
	fmt.Printf("  Final balance B:     %d micros\n", balanceB)
	fmt.Printf("  Sum invariant:       ")
	if invariantOK {
		fmt.Printf("PASS (expected=%d, got=%d)\n", expectedSum, actualSum)
	} else {
		fmt.Printf("FAIL (expected=%d, got=%d)\n", expectedSum, actualSum)
		os.Exit(1)
	}
	fmt.Println("[demo] All checks passed.")
}

func validateFlags(goroutines, transfers, duplicateKeys int) error {
	if goroutines < 1 {
		return fmt.Errorf("goroutines must be >= 1, got %d", goroutines)
	}
	if transfers < 1 {
		return fmt.Errorf("transfers must be >= 1, got %d", transfers)
	}
	if duplicateKeys < 0 || duplicateKeys > transfers {
		return fmt.Errorf("duplicate-keys must be 0..transfers (%d), got %d", transfers, duplicateKeys)
	}
	return nil
}

func createAccounts(ctx context.Context, client ledgerv1.LedgerServiceClient) error {
	for _, req := range []*ledgerv1.CreateAccountRequest{
		{AccountId: "A", InitialBalanceMicros: initialBalanceA},
		{AccountId: "B", InitialBalanceMicros: initialBalanceB},
	} {
		_, err := client.CreateAccount(ctx, req)
		if err != nil && status.Code(err) != codes.AlreadyExists {
			return err
		}
	}
	return nil
}

type transferWork struct {
	from, to    string
	amount      int64
	key         string
	isDuplicate bool
}

func buildWork(transfers, duplicateKeys int) []transferWork {
	// duplicateKeys requests reuse keys: use duplicateKeys keys each twice (20 keys × 2 = 40 requests, 20 duplicate).
	// uniqueCount = transfers - 2*dupKeysCount get fresh keys; 2*dupKeysCount share keys.
	dupKeysCount := duplicateKeys
	if dupKeysCount < 1 {
		dupKeysCount = 0
	}
	dupKeys := make([]string, dupKeysCount)
	for i := range dupKeys {
		dupKeys[i] = uuid.New().String()
	}
	dupRequestCount := dupKeysCount * 2
	uniqueCount := transfers - dupRequestCount
	if uniqueCount < 0 {
		uniqueCount = 0
	}

	// All A->B to avoid insufficient funds (B starts at 0). Amount chosen so 200× fits in 100M.
	amount := int64(100_000)

	work := make([]transferWork, 0, transfers)
	for i := 0; i < transfers; i++ {
		var key string
		var isDup bool
		if i < uniqueCount {
			key = uuid.New().String()
			isDup = false
		} else {
			idx := (i - uniqueCount) / 2
			key = dupKeys[idx%len(dupKeys)]
			isDup = (i-uniqueCount)%2 == 1
		}
		work = append(work, transferWork{from: "A", to: "B", amount: amount, key: key, isDuplicate: isDup})
	}
	return work
}

type result struct {
	executed  bool
	duplicate bool
	err       error
}

func runConcurrentTransfers(ctx context.Context, client ledgerv1.LedgerServiceClient, work []transferWork, numWorkers int) (executed, duplicates, failed int) {
	results := make(chan result, len(work))
	jobs := make(chan transferWork, len(work))

	for _, w := range work {
		jobs <- w
	}
	close(jobs)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range jobs {
				_, err := client.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
					From:           w.from,
					To:             w.to,
					AmountMicros:   w.amount,
					IdempotencyKey: w.key,
				})
				results <- result{executed: err == nil, duplicate: w.isDuplicate, err: err}
			}
		}()
	}

	// Go: WaitGroup ensures all goroutines finish before we close results
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		if r.err != nil {
			failed++
			continue
		}
		if r.duplicate {
			duplicates++
		} else {
			executed++
		}
	}
	return executed, duplicates, failed
}

func fetchBalances(ctx context.Context, client ledgerv1.LedgerServiceClient) (int64, int64, error) {
	respA, err := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "A"})
	if err != nil {
		return 0, 0, err
	}
	respB, err := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "B"})
	if err != nil {
		return 0, 0, err
	}
	return respA.BalanceMicros, respB.BalanceMicros, nil
}
