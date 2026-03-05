ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor && \
    mkdir -p /tmp && chmod 1777 /tmp

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
COPY --from=certs /tmp /tmp
COPY --from=lprobe /lprobe /lprobe
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/core /core
COPY make/migrations /migrations
COPY icons /icons
COPY src/core/views /views
WORKDIR /
EXPOSE 8080
USER harbor
ENTRYPOINT ["/core"]
