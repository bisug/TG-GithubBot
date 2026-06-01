# Build stage
FROM golang:1.26.3-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o tg-githubbot cmd/bot/main.go

# Final stage
FROM alpine:3.22

WORKDIR /app

RUN addgroup -S app && adduser -S -G app app

# Copy the CA certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the built binary
COPY --from=builder /app/tg-githubbot .

# Expose the webhook port (default 8080)
EXPOSE 8080

USER app

# Command to run the executable
CMD ["./tg-githubbot"]
