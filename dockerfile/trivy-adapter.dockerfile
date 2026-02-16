ARG GO_VERSION
ARG HARBOR_SCANNER_TRIVY_VERSION
ARG TRIVY_VERSION
ARG TRIVY_BASE_IMAGE_VERSION
ARG ALPINE_VERSION
ARG LPROBE_VERSION

FROM golang:${GO_VERSION} AS builder

ARG HARBOR_SCANNER_TRIVY_VERSION
ARG TARGETARCH

WORKDIR /go/src/github.com/goharbor/
RUN git clone -b ${HARBOR_SCANNER_TRIVY_VERSION} https://github.com/goharbor/harbor-scanner-trivy.git && \
    cd harbor-scanner-trivy && \
    CGO_ENABLED=0 GOARCH=${TARGETARCH} go build -o ./binary/scanner-trivy cmd/scanner-trivy/main.go

FROM aquasec/trivy:${TRIVY_VERSION} AS trivy-binary
FROM alpine:${ALPINE_VERSION} AS certs
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM aquasec/trivy:${TRIVY_BASE_IMAGE_VERSION}
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe
COPY --from=builder /go/src/github.com/goharbor/harbor-scanner-trivy/binary/scanner-trivy /home/scanner/bin/scanner-trivy
COPY --from=trivy-binary /usr/local/bin/trivy /usr/local/bin/trivy

RUN addgroup -S scanner && adduser -S -G scanner -h /home/scanner scanner && \
    chown -R scanner:scanner /home/scanner && \
    chown scanner:scanner /usr/local/bin/trivy

ARG HARBOR_SCANNER_TRIVY_VERSION
ENV SCANNER_VERSION=${HARBOR_SCANNER_TRIVY_VERSION}
WORKDIR /

EXPOSE 8080
EXPOSE 8443

USER scanner
ENTRYPOINT ["/home/scanner/bin/scanner-trivy"]
