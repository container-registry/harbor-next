ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor && \
    mkdir -p /var/lib/registry && chown harbor:harbor /var/lib/registry

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
COPY --from=lprobe /lprobe /lprobe
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/registry /usr/bin/registry_DO_NOT_USE_GC
COPY --from=certs --chown=harbor:harbor /var/lib/registry /var/lib/registry

ENV OTEL_TRACES_EXPORTER=none

VOLUME /var/lib/registry

EXPOSE 5000
EXPOSE 5443

USER harbor
ENTRYPOINT ["/usr/bin/registry_DO_NOT_USE_GC", "serve", "/etc/registry/config.yml"]
