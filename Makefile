.PHONY: test run vet test-integration generate

# Format all code according to the official Go style
fmt:
	go fmt ./...

# Run all tests (with race detector). Excludes integration tests.
test:
	go test -race -v ./...

# Run all tests including Postgres integration tests (requires Docker).
test-integration:
	go test -race -tags integration -v -count=1 ./...

# Generate sqlc code. Requires sqlc CLI: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
generate:
	sqlc generate

# Run the server
run:
	go run ./cmd/server

# Run go vet to check for issues
vet:
	go vet ./...