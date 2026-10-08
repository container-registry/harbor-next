ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG HEALTHPROBE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor

FROM 8gears.container-registry.com/healthprobe/healthprobe:${HEALTHPROBE_VERSION} AS healthprobe

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
ARG TARGETARCH
COPY --from=healthprobe /healthprobe-notls /healthprobe
COPY bin/linux-${TARGETARCH}/jobservice /jobservice
WORKDIR /
EXPOSE 8888
HEALTHCHECK --interval=10s --timeout=5s --retries=5 CMD ["/healthprobe", "-port", "8888", "-endpoint", "/api/v1/stats"]
USER harbor
ENTRYPOINT ["/jobservice", "-c", "/etc/jobservice/config.yml"]
