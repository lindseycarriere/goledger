package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net"
	"os"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lindseycarriere/goledger/gen/go/ledger/v1"
	"github.com/lindseycarriere/goledger/internal/domain"
	"github.com/lindseycarriere/goledger/internal/server"
	"github.com/lindseycarriere/goledger/internal/store/memory"
	"github.com/lindseycarriere/goledger/internal/store/postgres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

func main() {
	addr := flag.String("addr", ":50051", "gRPC listen address")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	dbType := strings.ToLower(os.Getenv("LEDGER_DB_TYPE"))
	if dbType == "" {
		dbType = "memory"
	}

	var store domain.Ledger

	switch dbType {
	case "memory":
		store = memory.NewStore()
	case "postgres":
		dsn := os.Getenv("LEDGER_DATABASE_URL")
		if dsn == "" {
			dsn = os.Getenv("DATABASE_URL")
		}
		if dsn == "" {
			slog.Error("postgres store requires LEDGER_DATABASE_URL or DATABASE_URL")
			os.Exit(1)
		}
		if err := postgres.RunMigrations(dsn); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			slog.Error("migrations failed", "err", err)
			os.Exit(1)
		}
		pool, err := pgxpool.New(context.Background(), dsn)
		if err != nil {
			slog.Error("postgres pool failed", "err", err)
			os.Exit(1)
		}
		pgStore := postgres.NewStore(pool)
		defer pgStore.Close()
		store = pgStore
	default:
		slog.Error("unknown LEDGER_DB_TYPE", "db_type", dbType)
		os.Exit(1)
	}

	srv := server.NewServer(store)

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			server.RecoveryInterceptor(),
			server.LoggingInterceptor(logger),
		),
	)
	ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)
	reflection.Register(grpcSrv) // Enables grpcurl to discover services at runtime

	// Go: health.NewServer() starts "" as SERVING by default; set NOT_SERVING
	// explicitly so state transitions reflect actual readiness intent.
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcSrv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_NOT_SERVING)
	healthSrv.SetServingStatus(ledgerv1.LedgerService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		slog.Error("listen failed", "addr", *addr, "err", err)
		os.Exit(1)
	}

	// Listener is bound; migrations and store setup are complete above — mark serving.
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthSrv.SetServingStatus(ledgerv1.LedgerService_ServiceDesc.ServiceName, healthpb.HealthCheckResponse_SERVING)

	slog.Info("gRPC server listening", "addr", lis.Addr().String())
	if err := grpcSrv.Serve(lis); err != nil {
		slog.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
