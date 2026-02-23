.PHONY: test run vet

# Format all code according to the official Go style
fmt:
	go fmt ./...

# Run all tests (with race detector)
test:
	go test -race -v ./...

# Run the server
run:
	go run ./cmd/server

# Run go vet to check for issues
vet:
	go vet ./...