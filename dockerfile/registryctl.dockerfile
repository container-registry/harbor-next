ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM alpine:${ALPINE_VERSION} AS certs
RUN addgroup -S -g 10000 harbor && adduser -S -G harbor -u 10000 harbor

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM scratch
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /etc/passwd /etc/group /etc/
COPY --from=lprobe /lprobe /lprobe
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/registryctl /registryctl
WORKDIR /
EXPOSE 8080
USER harbor
ENTRYPOINT ["/registryctl", "-c", "/etc/registryctl/config.yml"]
