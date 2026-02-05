# Development Dockerfile for Harbor Core and JobService
# Includes Go toolchain, Air (hot reload), and Delve (debugger)

FROM golang:1.25.6-alpine

# Install git (required for go mod download) and other tools
RUN apk add --no-cache git

# Install development tools (Air pinned to version compatible with Go 1.25)
RUN go install github.com/air-verse/air@v1.61.5 && \
    go install github.com/go-delve/delve/cmd/dlv@latest

WORKDIR /app

# Default command - can be overridden in docker-compose
CMD ["air"]
