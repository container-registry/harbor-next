ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/harbor-exporter /harbor-exporter
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["/harbor-exporter"]
