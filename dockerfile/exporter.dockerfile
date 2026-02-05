FROM scratch
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/harbor-exporter /harbor-exporter
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["/harbor-exporter"]
