// healthcheck calls the gRPC health check endpoint and exits 0 if SERVING, 1 otherwise.
// Used by the demo Makefile to wait for the server to be ready (no external binaries).
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	addr := flag.String("addr", "localhost:50051", "gRPC server address")
	timeout := flag.Duration("timeout", 2*time.Second, "dial + health check timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	conn, err := grpc.NewClient(*addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		os.Exit(1)
	}
	defer conn.Close()

	client := healthpb.NewHealthClient(conn)
	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: ""})
	if err != nil {
		os.Exit(1)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		os.Exit(1)
	}
}
