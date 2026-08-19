ARG GO_VERSION=MISSING-BUILD-ARG
ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG GRYPE_VERSION=MISSING-BUILD-ARG
ARG SYFT_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM docker.io/library/golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
COPY tools/grype-scanner/go.mod ./
COPY tools/grype-scanner/main.go tools/grype-scanner/rpmdb.go tools/grype-scanner/buildstream.go tools/grype-scanner/package_db.go ./
RUN CGO_ENABLED=0 go build -o /grype-scanner .

FROM docker.io/anchore/grype:${GRYPE_VERSION} AS grype-bin
FROM docker.io/anchore/syft:${SYFT_VERSION} AS syft-bin
FROM docker.io/library/alpine:${ALPINE_VERSION} AS certs
RUN apk add --no-cache ca-certificates
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM docker.io/library/alpine:${ALPINE_VERSION}
RUN apk add --no-cache zstd
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe
COPY --from=builder /grype-scanner /home/scanner/bin/grype-scanner
COPY --from=grype-bin /grype /usr/local/bin/grype
COPY --from=syft-bin /syft /usr/local/bin/syft

RUN addgroup -S scanner && \
    adduser -S -G scanner -h /home/scanner scanner && \
    mkdir -p /home/scanner/cache && \
    chown -R scanner:scanner /home/scanner && \
    chown scanner:scanner /usr/local/bin/grype /usr/local/bin/syft

ENV SCANNER_GRYPE_CACHE_DIR=/home/scanner/cache
USER scanner
WORKDIR /home/scanner
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --retries=5 CMD ["/lprobe", "-port", "8080", "-endpoint", "/probe/ready"]
ENTRYPOINT ["/home/scanner/bin/grype-scanner"]
