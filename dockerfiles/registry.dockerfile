# Dockerfile for Harbor Registry (Docker Distribution)
# Based on .dagger/main.go buildRegistry logic (lines 576-592)

FROM golang:1.24.6-alpine AS versioner

# Clone distribution repository
WORKDIR /go/src/github.com/docker
RUN apk add --no-cache git && \
    git clone -b v3.0.0 https://github.com/distribution/distribution.git && \
    cd distribution && \
    git apply CVE-2025-22872 fix
RUN cd distribution && \
    go mod edit -require golang.org/x/net@v0.38.0 && \
    go mod tidy -e && \
    go mod vendor

# Generate version info
RUN cd distribution && \
    VERSION=$(git describe --match 'v[0-9]*' --dirty='.m' --always --tags) && \
    REVISION=$(git rev-parse HEAD) && \
    PKG=github.com/distribution/distribution/v3 && \
    echo "-X ${PKG}/version.version=${VERSION#v} -X ${PKG}/version.revision=${REVISION} -X ${PKG}/version.mainpkg=${PKG}" > /tmp/.ldflags

# Build stage
FROM golang:1.24.6-alpine AS builder
COPY --from=versioner /go/src/github.com/docker/distribution /go/src/github.com/docker/distribution
COPY --from=versioner /tmp/.ldflags /tmp/.ldflags

WORKDIR /go/src/github.com/docker/distribution

ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags "$(cat /tmp/.ldflags) -s -w" -o /go/bin/registry ./cmd/registry && \
    /go/bin/registry --version

# Final stage - scratch base
FROM scratch

# Copy CA certificates
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy registry binary
COPY --from=builder /go/bin/registry /usr/bin/registry_DO_NOT_USE_GC

WORKDIR /

# Set OTEL environment variable (line 588)
ENV OTEL_TRACES_EXPORTER=none

# Expose ports
EXPOSE 5000
EXPOSE 5443

# Set entrypoint (line 591)
ENTRYPOINT ["/usr/bin/registry_DO_NOT_USE_GC", "serve", "/etc/registry/config.yml"]
