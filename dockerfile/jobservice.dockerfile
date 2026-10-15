ARG ALPINE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor && \
    mkdir -p /harbor-ca-writable && chown 10000:10000 /harbor-ca-writable

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
COPY --chown=10000:10000 --from=certs /harbor-ca-writable /etc/ssl/harbor-custom-ca
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/lprobe /lprobe
COPY bin/linux-${TARGETARCH}/jobservice /jobservice
WORKDIR /
EXPOSE 8888
HEALTHCHECK --interval=10s --timeout=5s --retries=5 CMD ["/lprobe", "-port", "8888", "-endpoint", "/api/v1/stats"]
USER harbor
ENTRYPOINT ["/jobservice", "-c", "/etc/jobservice/config.yml"]
