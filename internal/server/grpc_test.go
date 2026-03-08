package server

// Unit tests for the gRPC server. Uses bufconn (in-memory transport) so no real
// network or external services are involved — fast, hermetic, and suitable for
// regular `go test` runs.

import (
	"context"
	"fmt"
	"net"
	"testing"

	ledgerv1 "github.com/lindseycarriere/goledger/gen/go/ledger/v1"
	"github.com/lindseycarriere/goledger/internal/store/memory"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
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

	t.Run("PostTransaction_Idempotency", func(t *testing.T) {
		store := memory.NewStore()
		srv := NewServer(store)
		grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(RecoveryInterceptor()))
		ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)
		lis := bufconn.Listen(bufconnSize)
		go func() { _ = grpcSrv.Serve(lis) }()
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

		_, _ = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{AccountId: "A", InitialBalanceMicros: 100_000_000})
		_, _ = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{AccountId: "B", InitialBalanceMicros: 0})

		key := "idem-key-123"
		for i := 0; i < 10; i++ {
			_, err := client.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
				IdempotencyKey: key,
				From:           "A",
				To:             "B",
				AmountMicros:   10_000_000,
			})
			if err != nil {
				t.Fatalf("PostTransaction attempt %d: %v", i+1, err)
			}
		}

		respA, _ := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "A"})
		respB, _ := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "B"})
		if respA.BalanceMicros != 90_000_000 || respB.BalanceMicros != 10_000_000 {
			t.Errorf("balance debited more than once: A=%d B=%d, want A=90000000 B=10000000", respA.BalanceMicros, respB.BalanceMicros)
		}
	})

	t.Run("PostTransaction_Idempotency_DistinctKeyExecutes", func(t *testing.T) {
		store := memory.NewStore()
		srv := NewServer(store)
		grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(RecoveryInterceptor()))
		ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)
		lis := bufconn.Listen(bufconnSize)
		go func() { _ = grpcSrv.Serve(lis) }()
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

		_, _ = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{AccountId: "A", InitialBalanceMicros: 100_000_000})
		_, _ = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{AccountId: "B", InitialBalanceMicros: 0})

		for i := 0; i < 3; i++ {
			_, err := client.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
				IdempotencyKey: fmt.Sprintf("key-%d", i),
				From:           "A",
				To:             "B",
				AmountMicros:   10_000_000,
			})
			if err != nil {
				t.Fatalf("PostTransaction %d: %v", i+1, err)
			}
		}

		respA, _ := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "A"})
		respB, _ := client.GetBalance(ctx, &ledgerv1.GetBalanceRequest{AccountId: "B"})
		if respA.BalanceMicros != 70_000_000 || respB.BalanceMicros != 30_000_000 {
			t.Errorf("three distinct keys should execute three transfers: A=%d B=%d", respA.BalanceMicros, respB.BalanceMicros)
		}
	})

	t.Run("PostTransaction_IdempotencyHeader_Success", func(t *testing.T) {
		store := memory.NewStore()
		srv := NewServer(store)
		grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(RecoveryInterceptor()))
		ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)
		lis := bufconn.Listen(bufconnSize)
		go func() { _ = grpcSrv.Serve(lis) }()
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

		_, _ = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{AccountId: "A", InitialBalanceMicros: 100_000_000})
		_, _ = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{AccountId: "B", InitialBalanceMicros: 0})

		var header metadata.MD
		_, err = client.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
			IdempotencyKey: "header-test-key",
			From:           "A",
			To:             "B",
			AmountMicros:   1_000_000,
		}, grpc.Header(&header))
		if err != nil {
			t.Fatalf("PostTransaction: %v", err)
		}
		vals := header.Get("x-idempotency-key")
		if len(vals) != 1 || vals[0] != "header-test-key" {
			t.Errorf("x-idempotency-key header = %v, want [header-test-key]", vals)
		}
	})

	t.Run("PostTransaction_IdempotencyHeader_Error", func(t *testing.T) {
		store := memory.NewStore()
		srv := NewServer(store)
		grpcSrv := grpc.NewServer(grpc.ChainUnaryInterceptor(RecoveryInterceptor()))
		ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)
		lis := bufconn.Listen(bufconnSize)
		go func() { _ = grpcSrv.Serve(lis) }()
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

		_, _ = client.CreateAccount(ctx, &ledgerv1.CreateAccountRequest{AccountId: "A", InitialBalanceMicros: 1_000})
		// B does not exist — request will fail with NotFound

		var header metadata.MD
		_, err = client.PostTransaction(ctx, &ledgerv1.PostTransactionRequest{
			IdempotencyKey: "header-error-key",
			From:           "A",
			To:             "B",
			AmountMicros:   500,
		}, grpc.Header(&header))
		if err == nil {
			t.Fatal("PostTransaction expected error for missing account B")
		}
		vals := header.Get("x-idempotency-key")
		if len(vals) != 1 || vals[0] != "header-error-key" {
			t.Errorf("x-idempotency-key header on error = %v, want [header-error-key]", vals)
		}
	})
}

// TestHealthServer verifies that the standard gRPC health service reports
// SERVING for both the overall server ("") and the ledger service specifically,
// matching the state transitions performed in cmd/server/main.go.
func TestHealthServer(t *testing.T) {
	store := memory.NewStore()
	srv := NewServer(store)

	grpcSrv := grpc.NewServer()
	ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthSrv.SetServingStatus(ledgerv1.LedgerService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)
	// Simulate post-listen readiness transition (mirrors main.go startup sequence).
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus(ledgerv1.LedgerService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)

	lis := bufconn.Listen(bufconnSize)
	go func() { _ = grpcSrv.Serve(lis) }()
	t.Cleanup(grpcSrv.Stop)

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	healthClient := healthpb.NewHealthClient(conn)
	ctx := context.Background()

	t.Run("OverallStatus", func(t *testing.T) {
		resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
		if err != nil {
			t.Fatalf("Health/Check overall: %v", err)
		}
		if resp.Status != healthpb.HealthCheckResponse_SERVING {
			t.Errorf("overall status = %v, want SERVING", resp.Status)
		}
	})

	t.Run("LedgerServiceStatus", func(t *testing.T) {
		resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{
			Service: ledgerv1.LedgerService_ServiceDesc.ServiceName,
		})
		if err != nil {
			t.Fatalf("Health/Check ledger service: %v", err)
		}
		if resp.Status != healthpb.HealthCheckResponse_SERVING {
			t.Errorf("ledger service status = %v, want SERVING", resp.Status)
		}
	})
}
