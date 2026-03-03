package main

import (
	"flag"
	"log/slog"
	"net"
	"os"

	"github.com/lindseycarriere/goledger/internal/server"
	"github.com/lindseycarriere/goledger/internal/store/memory"
	"github.com/lindseycarriere/goledger/gen/go/ledger/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	addr := flag.String("addr", ":50051", "gRPC listen address")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	store := memory.NewStore()
	srv := server.NewServer(store)

	grpcSrv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			server.RecoveryInterceptor(),
			server.LoggingInterceptor(logger),
		),
	)
	ledgerv1.RegisterLedgerServiceServer(grpcSrv, srv)
	reflection.Register(grpcSrv) // Enables grpcurl to discover services at runtime

	lis, err := net.Listen("tcp", *addr)
	if err != nil {
		slog.Error("listen failed", "addr", *addr, "err", err)
		os.Exit(1)
	}

	slog.Info("gRPC server listening", "addr", lis.Addr().String())
	if err := grpcSrv.Serve(lis); err != nil {
		slog.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
