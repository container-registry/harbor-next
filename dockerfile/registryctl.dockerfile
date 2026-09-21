ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor

# lprobe ships as a released multi-arch image; buildx resolves it for
# the target platform, so no cross-compilation happens here.
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
ARG TARGETARCH
COPY --from=lprobe /lprobe /lprobe
COPY bin/linux-${TARGETARCH}/registryctl /registryctl
WORKDIR /
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --retries=5 CMD ["/lprobe", "-port", "8080", "-endpoint", "/api/health"]
USER harbor
ENTRYPOINT ["/registryctl", "-c", "/etc/registryctl/config.yml"]
