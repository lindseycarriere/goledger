# Build stage
FROM golang:1.25-alpine AS builder
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

# Install grpc-health-probe for the compose healthcheck; built here and copied
# into the runtime image so no extra package manager is needed at runtime.
RUN go install github.com/grpc-ecosystem/grpc-health-probe@v0.4.46

COPY . .
RUN CGO_ENABLED=0 go build -o /server ./cmd/server

# Runtime stage
FROM alpine:3.22
WORKDIR /app

RUN apk add --no-cache ca-certificates
COPY --from=builder /server .
COPY --from=builder /go/bin/grpc-health-probe /usr/local/bin/grpc-health-probe
COPY migrations ./migrations

EXPOSE 50051

ENV LEDGER_DB_TYPE=memory
# LEDGER_DATABASE_URL set by compose when DB_TYPE=postgres

ENTRYPOINT ["./server"]
