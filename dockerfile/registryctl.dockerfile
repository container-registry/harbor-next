ARG LPROBE_VERSION


FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe


FROM scratch
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=lprobe /lprobe /lprobe
ARG TARGETARCH
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["/registryctl", "-c", "/etc/registryctl/config.yml"]
COPY bin/linux-${TARGETARCH}/registryctl /registryctl
