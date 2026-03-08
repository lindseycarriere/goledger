# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

# Runtime stage
FROM alpine:3.22
WORKDIR /app

# netcat-openbsd: used by the docker-compose healthcheck to probe TCP port 50051
# (gRPC has no HTTP GET to curl; nc -z is a simple "is the port open?" check).
RUN apk add --no-cache ca-certificates netcat-openbsd
COPY --from=builder /server .
COPY migrations ./migrations

EXPOSE 50051

ENV LEDGER_DB_TYPE=memory
# LEDGER_DATABASE_URL set by compose when DB_TYPE=postgres

ENTRYPOINT ["./server"]
