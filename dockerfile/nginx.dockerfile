# Dockerfile for Harbor Nginx Proxy
# Uses Docker Hardened Image (distroless, non-root, no shell).

ARG NGINX_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG
ARG ALPINE_VERSION=MISSING-BUILD-ARG

FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

# Prep stage: set group ownership for OpenShift (arbitrary UID, GID 0).
# DHI runtime has no shell, so permission fixups happen here.
FROM alpine:${ALPINE_VERSION} AS perms
RUN mkdir -p /var/cache/nginx /var/log/nginx /etc/nginx/conf.d && \
    chgrp -R 0 /var/cache/nginx /var/log/nginx /etc/nginx/conf.d && \
    chmod -R g=u /var/cache/nginx /var/log/nginx /etc/nginx/conf.d

FROM dhi.io/nginx:${NGINX_VERSION}-debian13

COPY --from=lprobe /lprobe /lprobe
COPY --from=perms /var/cache/nginx /var/cache/nginx
COPY --from=perms /var/log/nginx /var/log/nginx
COPY --from=perms /etc/nginx/conf.d /etc/nginx/conf.d

EXPOSE 8080
ENTRYPOINT ["nginx", "-g", "daemon off;"]
