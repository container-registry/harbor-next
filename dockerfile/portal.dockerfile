# Dockerfile for Harbor Portal (Angular Frontend) on Nginx

ARG BUN_VERSION
ARG NGINX_VERSION
ARG LPROBE_VERSION

#
# Build Angular application and Swagger UI
FROM oven/bun:${BUN_VERSION}-alpine AS builder
# nodejs required: bun hangs on Angular/webpack build inside Docker (oven-sh/bun#15226)
RUN apk add --no-cache nodejs yq
WORKDIR /harbor/src/portal
COPY src/portal/package.json src/portal/bun.lock* ./
RUN bun install --ignore-scripts
COPY src/portal ./
COPY api/v2.0/swagger.yaml /swagger.yaml
RUN bun run postinstall && \
    bun run generate-build-timestamp && \
    node --max_old_space_size=2048 node_modules/@angular/cli/bin/ng build --configuration production
RUN yq -o=json /swagger.yaml > swagger.json
COPY LICENSE ./dist/LICENSE
RUN cd app-swagger-ui && bun install --ignore-scripts && bun run build

#
# RUNTIME
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM nginx:${NGINX_VERSION}-alpine
RUN apk add --no-cache ca-certificates
COPY --from=lprobe /lprobe /lprobe
COPY --from=builder /harbor/src/portal/dist /usr/share/nginx/html
COPY --from=builder /harbor/src/portal/swagger.json /usr/share/nginx/html/swagger.json
COPY --from=builder /harbor/src/portal/app-swagger-ui/dist /usr/share/nginx/html
COPY config/portal/nginx.conf /etc/nginx/nginx.conf
WORKDIR /usr/share/nginx/html

EXPOSE 8080
EXPOSE 8443

ENTRYPOINT ["nginx", "-g", "daemon off;"]
