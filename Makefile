.PHONY: test run vet

# Run all tests
test:
	go test -v ./...

# Run the server
run:
	go run ./cmd/server

# Run go vet to check for issues
vet:
	go vet ./...