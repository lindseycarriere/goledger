package server

// Unit tests for the gRPC server. Uses bufconn (in-memory transport) so no real
// network or external services are involved — fast, hermetic, and suitable for
// regular `go test` runs.

import (
	"context"
	"net"
	"testing"

	ledgerv1 "github.com/lindseycarriere/goledger/gen/go/ledger/v1"
	"github.com/lindseycarriere/goledger/internal/store/memory"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

const bufconnSize = 1024 * 1024

// TestGRPCServer verifies that the gRPC handlers correctly wire requests to
// domain.Ledger. Subtests run sequentially and share state to exercise the full
// happy path: create accounts → get balances → post transfer → verify invariant.
func TestGRPCServer(t *testing.T) {
	store := memory.NewStore()
	srv := NewServer(store)

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(RecoveryInterceptor()),
	)
	ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)

	lis := bufconn.Listen(bufconnSize)
	go func() {
		_ = grpcSrv.Serve(lis)
	}()
	t.Cleanup(grpcSrv.Stop)

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	client := ledgerv1.NewLedgerServiceClient(conn)
	ctx := context.Background()

	t.Run("CreateAccount", func(t *testing.T) {
		_, err := client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{
			AccountId:            "A",
			InitialBalanceMicros: 100_000_000,
		})
		if err != nil {
			t.Fatalf("CreateAccount A: %v", err)
		}
		_, err = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{
			AccountId:            "B",
			InitialBalanceMicros: 0,
		})
		if err != nil {
			t.Fatalf("CreateAccount B: %v", err)
		}
	})

	t.Run("GetBalance", func(t *testing.T) {
		resp, err := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "A"})
		if err != nil {
			t.Fatalf("GetBalance A: %v", err)
		}
		if resp.BalanceMicros != 100_000_000 {
			t.Errorf("GetBalance A = %d, want 100000000", resp.BalanceMicros)
		}
		resp, err = client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "B"})
		if err != nil {
			t.Fatalf("GetBalance B: %v", err)
		}
		if resp.BalanceMicros != 0 {
			t.Errorf("GetBalance B = %d, want 0", resp.BalanceMicros)
		}
	})

	t.Run("PostTransaction", func(t *testing.T) {
		_, err := client.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
			From:         "A",
			To:           "B",
			AmountMicros: 50_000_000,
		})
		if err != nil {
			t.Fatalf("PostTransaction: %v", err)
		}
		respA, err := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "A"})
		if err != nil {
			t.Fatalf("GetBalance A after transfer: %v", err)
		}
		respB, err := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "B"})
		if err != nil {
			t.Fatalf("GetBalance B after transfer: %v", err)
		}
		if respA.BalanceMicros != 50_000_000 || respB.BalanceMicros != 50_000_000 {
			t.Errorf("after transfer: A=%d B=%d, want 50000000 each", respA.BalanceMicros, respB.BalanceMicros)
		}
		if respA.BalanceMicros+respB.BalanceMicros != 100_000_000 {
			t.Errorf("sum invariant violated: %d + %d != 100000000", respA.BalanceMicros, respB.BalanceMicros)
		}
	})
}
