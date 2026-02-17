# Dockerfile for Harbor Nginx Proxy
# Uses Docker Hardened Image (Debian 13, non-root, CIS-compliant).
# nginx runs as UID 65532 (nginx). Image includes coreutils and /bin/sh.

ARG NGINX_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM dhi.io/nginx:${NGINX_VERSION}-debian13

COPY --from=lprobe /lprobe /lprobe

# OpenShift runs containers with arbitrary UID and GID 0.
# Base image dirs are owned by nginx:nginx (65532:65532).
# Grant GID 0 write access so OpenShift's arbitrary UID can write.
RUN chgrp -R 0 /var/cache/nginx /var/log/nginx /etc/nginx/conf.d /run/nginx && \
    chmod -R g=u /var/cache/nginx /var/log/nginx /etc/nginx/conf.d /run/nginx

EXPOSE 8080
ENTRYPOINT ["nginx", "-g", "daemon off;"]
