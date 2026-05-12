# Build Stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o bot cmd/bot/main.go

# Final Stage
FROM alpine:latest

# Security: Run as non-root user
RUN adduser -D -u 10001 botuser

WORKDIR /app

# Essential runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Copy binary from builder
COPY --from=builder /app/bot .

# Use non-root user
USER botuser

ENTRYPOINT ["./bot"]
