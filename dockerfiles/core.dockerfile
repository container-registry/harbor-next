FROM scratch
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
ARG TARGETARCH
COPY bin/linux-${TARGETARCH}/core /core
COPY make/migrations /migrations
COPY icons /icons
COPY src/core/views /views
WORKDIR /
EXPOSE 8080
ENTRYPOINT ["/core"]
