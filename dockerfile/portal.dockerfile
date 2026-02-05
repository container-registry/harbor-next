# Dockerfile for Harbor Portal (Angular Frontend) on Nginx

ARG BUN_VERSION

#
# Build Angular application and Swagger UI
FROM oven/bun:${BUN_VERSION}-alpine AS builder
RUN apk add --no-cache nodejs yq
WORKDIR /harbor/src/portal
COPY src/portal/package.json src/portal/bun.lock* ./
COPY src/portal ./
COPY api/v2.0/swagger.yaml /swagger.yaml
RUN bun install
RUN bun run generate-build-timestamp && \
    bun run node --max_old_space_size=2048 node_modules/@angular/cli/bin/ng build --configuration production
RUN yq -o=json /swagger.yaml > swagger.json
COPY LICENSE ./dist/LICENSE
RUN cd app-swagger-ui && bun install --ignore-scripts && bun run build

#
# RUNTIME
FROM nginx:alpine
RUN apk add --no-cache ca-certificates
COPY --from=builder /harbor/src/portal/dist /usr/share/nginx/html
COPY --from=builder /harbor/src/portal/swagger.json /usr/share/nginx/html/swagger.json
COPY --from=builder /harbor/src/portal/app-swagger-ui/dist /usr/share/nginx/html
COPY config/portal/nginx.conf /etc/nginx/nginx.conf
WORKDIR /usr/share/nginx/html

EXPOSE 8080
EXPOSE 8443

ENTRYPOINT ["nginx", "-g", "daemon off;"]
