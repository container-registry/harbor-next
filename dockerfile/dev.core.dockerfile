# Development Dockerfile for Harbor Core and JobService
# Includes Go toolchain, Air (hot reload), and Delve (debugger)

ARG GO_VERSION=MISSING-BUILD-ARG
FROM golang:${GO_VERSION}-alpine

ARG AIR_VERSION
ARG DELVE_VERSION
ARG DEV_UID=1000
ARG DEV_GID=1000

# Install git (required by go modules)
RUN apk add --no-cache git

# Install development tools
RUN go install github.com/air-verse/air@${AIR_VERSION} && \
    go install github.com/go-delve/delve/cmd/dlv@${DELVE_VERSION}

RUN addgroup -S -g ${DEV_GID} harbor && \
    adduser -S -D -G harbor -u ${DEV_UID} harbor && \
    mkdir -p /home/harbor/.cache/go-build && \
    chown -R harbor:harbor /home/harbor

ENV HOME=/home/harbor

WORKDIR /app

# Default command - can be overridden in docker-compose
HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=5 CMD wget -q -O /dev/null "http://127.0.0.1:${HEALTHCHECK_PORT:-8080}${HEALTHCHECK_ENDPOINT:-/api/v2.0/ping}" || exit 1
USER harbor
CMD ["air"]
