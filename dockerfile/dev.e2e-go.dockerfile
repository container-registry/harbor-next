# Minimal Go runtime/build image for containerized E2E services.
ARG GO_VERSION=MISSING-BUILD-ARG
FROM golang:${GO_VERSION}-alpine

ARG DEV_UID=1000
ARG DEV_GID=1000

RUN apk add --no-cache git && \
    group="$(getent group "${DEV_GID}" | cut -d: -f1)" && \
    if [ -z "$group" ]; then \
      addgroup -S -g "${DEV_GID}" harbor; \
      group=harbor; \
    fi && \
    adduser -S -D -G "$group" -u "${DEV_UID}" harbor && \
    mkdir -p /home/harbor/.cache/go-build /go/pkg/mod /var/lib/harbor-e2e && \
    chown -R harbor:"$group" /home/harbor /go/pkg/mod /var/lib/harbor-e2e

ENV HOME=/home/harbor

WORKDIR /app
USER harbor
