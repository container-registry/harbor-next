# Development Dockerfile for Harbor Trivy Scanner Adapter
# Builds scanner-trivy from source (no pre-compiled binary needed)

ARG GO_VERSION=MISSING-BUILD-ARG
ARG HARBOR_SCANNER_TRIVY_VERSION=MISSING-BUILD-ARG
ARG TRIVY_VERSION=MISSING-BUILD-ARG
ARG TRIVY_BASE_IMAGE_VERSION=MISSING-BUILD-ARG
ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM golang:${GO_VERSION}-alpine AS builder
ARG HARBOR_SCANNER_TRIVY_VERSION
RUN apk add --no-cache git
RUN git clone --branch ${HARBOR_SCANNER_TRIVY_VERSION} --depth 1 \
      https://github.com/container-registry/harbor-scanner-trivy.git /src
WORKDIR /src
RUN CGO_ENABLED=0 go build -o /scanner-trivy cmd/scanner-trivy/main.go

FROM aquasec/trivy:${TRIVY_VERSION} AS trivy-binary
FROM alpine:${ALPINE_VERSION} AS certs
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM aquasec/trivy:${TRIVY_BASE_IMAGE_VERSION}
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe
COPY --from=builder /scanner-trivy /home/scanner/bin/scanner-trivy
COPY --from=trivy-binary /usr/local/bin/trivy /usr/local/bin/trivy

RUN addgroup -S scanner && adduser -S -G scanner -h /home/scanner scanner && \
    chown -R scanner:scanner /home/scanner && \
    chown scanner:scanner /usr/local/bin/trivy

ARG HARBOR_SCANNER_TRIVY_VERSION
ENV SCANNER_VERSION=${HARBOR_SCANNER_TRIVY_VERSION}

# Read by the scanner's GetScannerMetadata(); the dev image ships the
# unmodified upstream trivy binary, so no commit suffix here.
ARG TRIVY_VERSION
ENV TRIVY_VERSION=${TRIVY_VERSION}
WORKDIR /

EXPOSE 8080
EXPOSE 8443
HEALTHCHECK --interval=10s --timeout=5s --retries=5 CMD ["/lprobe", "-port", "8080", "-endpoint", "/probe/ready"]

USER scanner
ENTRYPOINT ["/home/scanner/bin/scanner-trivy"]
