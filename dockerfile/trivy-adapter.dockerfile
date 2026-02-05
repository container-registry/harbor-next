# Dockerfile for Harbor Trivy Adapter

ARG GO_VERSION=1.25.6
ARG HARBOR_SCANNER_TRIVY_VERSION=v0.33.2
ARG TRIVY_VERSION=0.64.1
ARG TRIVY_BASE_IMAGE_VERSION=0.58.1

FROM golang:${GO_VERSION} AS builder

ARG HARBOR_SCANNER_TRIVY_VERSION
ARG TRIVY_VERSION

# Build trivy-adapter (lines 598-614)
WORKDIR /go/src/github.com/goharbor/
RUN git clone -b ${HARBOR_SCANNER_TRIVY_VERSION} https://github.com/goharbor/harbor-scanner-trivy.git && \
    cd harbor-scanner-trivy && \
    CGO_ENABLED=0 go build -o ./binary/scanner-trivy cmd/scanner-trivy/main.go

# Download trivy binary
RUN cd harbor-scanner-trivy && \
    wget -O trivyDownload https://github.com/aquasecurity/trivy/releases/download/v${TRIVY_VERSION}/trivy_${TRIVY_VERSION}_Linux-64bit.tar.gz && \
    tar -zxvf trivyDownload && \
    cp trivy ./binary/trivy

# Final stage - use aquasec/trivy base image (line 620)
ARG TRIVY_BASE_IMAGE_VERSION
FROM aquasec/trivy:${TRIVY_BASE_IMAGE_VERSION}

# Copy CA certificates
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binaries from builder (lines 622-623)
COPY --from=builder /go/src/github.com/goharbor/harbor-scanner-trivy/binary/scanner-trivy /home/scanner/bin/scanner-trivy
COPY --from=builder /go/src/github.com/goharbor/harbor-scanner-trivy/binary/trivy /usr/local/bin/trivy

# Set environment variable (line 625)
ARG HARBOR_SCANNER_TRIVY_VERSION
ENV SCANNER_VERSION=${HARBOR_SCANNER_TRIVY_VERSION}

WORKDIR /

# Expose ports (lines 626-627)
EXPOSE 8080
EXPOSE 8443

# Set entrypoint (line 628)
ENTRYPOINT ["/home/scanner/bin/scanner-trivy"]
