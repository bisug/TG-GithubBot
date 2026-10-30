# Build stage
FROM golang:1.27 AS builder

WORKDIR /app

# Download Go dependencies
COPY go.mod go.sum ./
RUN go mod download

# Cache TDLib binary download layer (only re-downloads when dependencies change)
RUN go run github.com/AshokShau/gotdbot/scripts/tools

# Copy source code and build stripped binary
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o bot .

# Final runtime stage (~80MB instead of ~1.2GB)
FROM debian:bookworm-slim

WORKDIR /app

# Install runtime dependencies required by TDLib and HTTPS requests
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    libstdc++6 \
    && rm -rf /var/lib/apt/lists/*

# Run as non-root user for security
RUN groupadd -g 10001 appgroup && \
    useradd -u 10001 -g appgroup -s /bin/sh -d /app appuser && \
    mkdir -p /app/database && \
    chown -R appuser:appgroup /app

# Copy binary and TDLib shared library from builder
COPY --from=builder --chown=appuser:appgroup /app/bot /app/bot
COPY --from=builder --chown=appuser:appgroup /app/libtdjson.so* /app/

USER appuser

EXPOSE 8080

CMD ["./bot"]
