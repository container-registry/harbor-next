# Dockerfile for Harbor Nginx Proxy
# Uses Docker Hardened Image (Debian 13, non-root, CIS-compliant).
# DHI images have no shell; permissions are fixed in an alpine stage.

ARG NGINX_VERSION=MISSING-BUILD-ARG
ARG ALPINE_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM dhi.io/nginx:${NGINX_VERSION}-debian13 AS nginx-base

# OpenShift runs containers with arbitrary UID and GID 0.
# Base image dirs are owned by nginx:nginx (65532:65532).
# Grant GID 0 write access so OpenShift's arbitrary UID can write.
# DHI image lacks /bin/sh, so fix permissions in an alpine stage.
FROM alpine:${ALPINE_VERSION} AS fixperms
COPY --from=nginx-base /var/cache/nginx /var/cache/nginx
COPY --from=nginx-base /var/log/nginx /var/log/nginx
COPY --from=nginx-base /etc/nginx/conf.d /etc/nginx/conf.d
RUN mkdir -p /run/nginx && \
    chgrp -R 0 /var/cache/nginx /var/log/nginx /etc/nginx/conf.d /run/nginx && \
    chmod -R g=u /var/cache/nginx /var/log/nginx /etc/nginx/conf.d /run/nginx

FROM nginx-base
COPY --from=lprobe /lprobe /lprobe
COPY --from=fixperms /var/cache/nginx /var/cache/nginx
COPY --from=fixperms /var/log/nginx /var/log/nginx
COPY --from=fixperms /etc/nginx/conf.d /etc/nginx/conf.d
COPY --from=fixperms /run/nginx /run/nginx

EXPOSE 8080
ENTRYPOINT ["nginx", "-g", "daemon off;"]
