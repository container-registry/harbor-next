ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/core /core
COPY make/migrations /migrations
COPY icons /icons
COPY src/core/views /views
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["/core"]
