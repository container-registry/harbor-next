# Dockerfile for Harbor Registry (Docker Distribution)
# https://github.com/distribution/distribution

# Versions from Taskfile.yml
ARG GO_VERSION
ARG DISTRIBUTION_VERSION

FROM golang:${GO_VERSION}-alpine AS builder

ARG DISTRIBUTION_VERSION

WORKDIR /go/src/github.com/distribution

RUN apk add --no-cache git && \
    git clone --branch ${DISTRIBUTION_VERSION} --depth 1 https://github.com/distribution/distribution.git && \
    cd distribution && \
    # CVE-2025-22872: XSS in golang.org/x/net/html - fixed in v0.38.0
    go mod edit -require golang.org/x/net@v0.49.0 && \
    go mod tidy -e && \
    go mod vendor && \
    REVISION=$(git rev-parse HEAD) && \
    PKG=github.com/distribution/distribution/v3 && \
    LDFLAGS="-X ${PKG}/version.version=${DISTRIBUTION_VERSION#v} -X ${PKG}/version.revision=${REVISION} -X ${PKG}/version.mainpkg=${PKG} -s -w" && \
    CGO_ENABLED=0 go build -trimpath -ldflags "${LDFLAGS}" -o /go/bin/registry ./cmd/registry && \
    /go/bin/registry --version

# Final stage
FROM scratch

COPY --from=alpine:3.21 /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /go/bin/registry /usr/bin/registry_DO_NOT_USE_GC

ENV OTEL_TRACES_EXPORTER=none

EXPOSE 5000
EXPOSE 5443

ENTRYPOINT ["/usr/bin/registry_DO_NOT_USE_GC", "serve", "/etc/registry/config.yml"]
