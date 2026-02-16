# Development Dockerfile for Harbor Core and JobService
# Includes Go toolchain, Air (hot reload), and Delve (debugger)

ARG GO_VERSION
FROM golang:${GO_VERSION}-alpine

ARG AIR_VERSION
ARG DELVE_VERSION

# Install git (required for go mod download) and other tools
RUN apk add --no-cache git

# Install development tools
RUN go install github.com/air-verse/air@${AIR_VERSION} && \
    go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION}

WORKDIR /app

# Default command - can be overridden in docker-compose
CMD ["air"]
