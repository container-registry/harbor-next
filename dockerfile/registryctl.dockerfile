ARG ALPINE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor && \
    mkdir -p /etc/registry /etc/registryctl

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
COPY --from=certs /etc/registry /etc/registry
COPY --from=certs /etc/registryctl /etc/registryctl
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/lprobe /lprobe
COPY bin/linux-${TARGETARCH}/registryctl /registryctl
WORKDIR /
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --retries=5 CMD ["/lprobe", "-port", "8080", "-endpoint", "/api/health"]
USER harbor
ENTRYPOINT ["/registryctl", "-c", "/etc/registryctl/config.yml"]
