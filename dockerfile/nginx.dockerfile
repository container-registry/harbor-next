# Dockerfile for Harbor Nginx Proxy
# Uses Docker Hardened Image (Debian 13, non-root, CIS-compliant).
# nginx runs as UID 65532 (nginx). Image has no /bin/sh (distroless).

ARG NGINX_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM dhi.io/nginx:${NGINX_VERSION}-debian13 AS base

# OpenShift runs containers with arbitrary UID and GID 0.
# Base image dirs are owned by nginx:nginx (65532:65532) — GID 0 has no
# write access. Since the DHI image has no shell, permission fixups run
# in an Alpine builder that first copies content from the base image.
FROM alpine:3 AS perms
COPY --from=base /var/cache/nginx /var/cache/nginx
COPY --from=base /var/log/nginx /var/log/nginx
COPY --from=base /etc/nginx/conf.d /etc/nginx/conf.d
COPY --from=base /run/nginx /run/nginx
RUN chgrp -R 0 /var/cache/nginx /var/log/nginx /etc/nginx/conf.d /run/nginx && \
    chmod -R g=u /var/cache/nginx /var/log/nginx /etc/nginx/conf.d /run/nginx

FROM base

COPY --from=lprobe /lprobe /lprobe
COPY --from=perms /var/cache/nginx /var/cache/nginx
COPY --from=perms /var/log/nginx /var/log/nginx
COPY --from=perms /etc/nginx/conf.d /etc/nginx/conf.d
COPY --from=perms /run/nginx /run/nginx

EXPOSE 8080
ENTRYPOINT ["nginx", "-g", "daemon off;"]
