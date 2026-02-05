ARG GO_VERSION
ARG HARBOR_SCANNER_TRIVY_VERSION
ARG TRIVY_VERSION
ARG TRIVY_BASE_IMAGE_VERSION
ARG LPROBE_VERSION

FROM golang:${GO_VERSION} AS builder

ARG HARBOR_SCANNER_TRIVY_VERSION
ARG TRIVY_VERSION

WORKDIR /go/src/github.com/goharbor/
RUN git clone -b ${HARBOR_SCANNER_TRIVY_VERSION} https://github.com/goharbor/harbor-scanner-trivy.git && \
    cd harbor-scanner-trivy && \
    CGO_ENABLED=0 go build -o ./binary/scanner-trivy cmd/scanner-trivy/main.go

# Download trivy binary
RUN cd harbor-scanner-trivy && \
    wget -O trivyDownload https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz && \
    tar -zxvf trivyDownload && \
    cp trivy ./binary/trivy

ARG TRIVY_BASE_IMAGE_VERSION
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM aquasec/trivy:${TRIVY_BASE_IMAGE_VERSION}
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe
COPY --from=builder /go/src/github.com/goharbor/harbor-scanner-trivy/binary/scanner-trivy /home/scanner/bin/scanner-trivy
COPY --from=builder /go/src/github.com/goharbor/harbor-scanner-trivy/binary/trivy /usr/local/bin/trivy

ARG HARBOR_SCANNER_TRIVY_VERSION
ENV SCANNER_VERSION=${HARBOR_SCANNER_TRIVY_VERSION}
WORKDIR /

EXPOSE 8080
EXPOSE 8443

ENTRYPOINT ["/home/scanner/bin/scanner-trivy"]
