# Dockerfile for Harbor Registryctl (Production)
# Uses scratch base with only CA certs and binary

ARG LPROBE_VERSION
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch

# Copy CA certificates from Alpine
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe

# Copy binary from build context
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/registryctl /registryctl

# Set working directory
WORKDIR /

# Expose port
EXPOSE 8080

# Set entrypoint with config file
# From line 442: entrypoint includes "-c /etc/registryctl/config.yml"
ENTRYPOINT ["/registryctl", "-c", "/etc/registryctl/config.yml"]
