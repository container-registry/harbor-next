# Dockerfile for Harbor Nginx Proxy

ARG NGINX_VERSION
ARG LPROBE_VERSION

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM nginx:${NGINX_VERSION}-alpine
RUN apk add --no-cache ca-certificates

COPY --from=lprobe /lprobe /lprobe

WORKDIR /

EXPOSE 8080
ENTRYPOINT ["nginx", "-g", "daemon off;"]
