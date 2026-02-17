# Dockerfile for Harbor Registry (Docker Distribution)
# https://github.com/distribution/distribution

ARG GO_VERSION=MISSING-BUILD-ARG
ARG DISTRIBUTION_VERSION=MISSING-BUILD-ARG
ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

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
FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
COPY --from=lprobe /lprobe /lprobe
COPY --from=builder /go/bin/registry /usr/bin/registry_DO_NOT_USE_GC

ENV OTEL_TRACES_EXPORTER=none

EXPOSE 5000
EXPOSE 5443

USER harbor
ENTRYPOINT ["/usr/bin/registry_DO_NOT_USE_GC", "serve", "/etc/registry/config.yml"]
