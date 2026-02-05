# Dockerfile for Harbor Registryctl (Production)
# Based on .dagger/main.go buildImage logic (lines 432-472)
# Uses scratch base with only CA certs and binary

FROM scratch

# Copy CA certificates from Alpine
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

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
