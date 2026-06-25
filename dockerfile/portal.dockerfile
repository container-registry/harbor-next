# Dockerfile for Harbor Portal (Angular Frontend) on Nginx

ARG BUN_VERSION=MISSING-BUILD-ARG
ARG GO_VERSION=MISSING-BUILD-ARG
ARG LPROBE_VERSION=MISSING-BUILD-ARG
ARG NGINX_VERSION=MISSING-BUILD-ARG

#
# Build Angular application and Swagger UI
FROM --platform=$BUILDPLATFORM oven/bun:${BUN_VERSION}-alpine AS builder
# nodejs required: bun hangs on Angular/webpack build inside Docker (oven-sh/bun#15226)
RUN apk add --no-cache nodejs
WORKDIR /harbor/src/portal
COPY src/portal/package.json src/portal/bun.lock* ./
RUN bun install --ignore-scripts
COPY src/portal ./
COPY api/v2.0/swagger.yaml /swagger.yaml
RUN bun run postinstall && \
    bun run generate-build-timestamp && \
    node --max_old_space_size=2048 node_modules/@angular/cli/bin/ng build --configuration production
COPY LICENSE ./dist/LICENSE
WORKDIR /harbor/src/portal/app-swagger-ui
RUN bun install --ignore-scripts && bun run build

#
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS lprobe
ARG LPROBE_VERSION
ARG TARGETARCH
ARG TARGETOS
RUN apk add --no-cache git && \
    git clone --branch v${LPROBE_VERSION} --depth 1 https://github.com/fivexl/lprobe.git /src/lprobe && \
    cd /src/lprobe && \
    go mod edit -require github.com/go-jose/go-jose/v4@v4.1.4 && \
    go mod edit -require golang.org/x/net@v0.55.0 && \
    go mod tidy -e && \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -o /lprobe .

FROM 8gears.container-registry.com/dhi.io/nginx:${NGINX_VERSION}-debian13
COPY --from=lprobe /lprobe /lprobe
COPY --from=builder /harbor/src/portal/dist /usr/share/nginx/html
COPY --from=builder /harbor/src/portal/app-swagger-ui/dist /usr/share/nginx/html
COPY config/portal/nginx.conf /etc/nginx/nginx.conf
WORKDIR /usr/share/nginx/html

EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD ["/lprobe", "-port", "8080"]
USER nginx
ENTRYPOINT ["nginx", "-g", "daemon off;"]
