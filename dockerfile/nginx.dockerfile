# Dockerfile for Harbor Nginx Proxy

ARG NGINX_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM nginx:${NGINX_VERSION}-alpine
RUN apk add --no-cache ca-certificates

COPY --from=lprobe /lprobe /lprobe

WORKDIR /

RUN chgrp -R 0 /var/cache/nginx /var/log/nginx /etc/nginx/conf.d && \
    chmod -R g=u /var/cache/nginx /var/log/nginx /etc/nginx/conf.d

EXPOSE 8080
USER nginx
ENTRYPOINT ["nginx", "-g", "daemon off;"]
