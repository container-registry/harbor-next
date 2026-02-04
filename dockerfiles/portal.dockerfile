# Dockerfile for Harbor Portal (Angular Frontend)
# Based on .dagger/main.go buildPortal logic

# Stage 1: Extract swagger.yaml and LICENSE
FROM alpine:latest AS extractor
WORKDIR /harbor
COPY api/v2.0/swagger.yaml /swagger.yaml
COPY LICENSE /LICENSE

# Stage 2: Build Angular application
FROM node:16.18.0 AS builder

# Set npm registry
ENV NPM_CONFIG_REGISTRY=https://registry.npmjs.org

# Install Bun
RUN apt-get update && \
    apt-get install -y unzip && \
    npm install -g bun@1.2.13

WORKDIR /harbor/src/portal

# Copy portal source
COPY src/portal/package.json src/portal/package-lock.json ./
COPY src/portal ./

# Copy swagger.yaml for API generation (script checks /swagger.yaml at root)
COPY --from=extractor /swagger.yaml /swagger.yaml

# Install dependencies
RUN bun install

# Generate build timestamp and build Angular app
RUN bun run generate-build-timestamp && \
    bun run node --max_old_space_size=2048 node_modules/@angular/cli/bin/ng build --configuration production

# Convert swagger.yaml to JSON for /swagger.json endpoint
RUN bun install js-yaml@4.1.0 --no-verify && \
    bun -e "const yaml = require('js-yaml'); const fs = require('fs'); const swagger = yaml.load(fs.readFileSync('/swagger.yaml', 'utf8')); fs.writeFileSync('swagger.json', JSON.stringify(swagger));"

# Copy LICENSE to dist
COPY --from=extractor /LICENSE ./dist/LICENSE

# Stage 3: Build Swagger UI
FROM node:16.18.0 AS swagger-builder

WORKDIR /harbor/src/portal/app-swagger-ui

# Copy swagger UI source and built files
COPY src/portal/app-swagger-ui ./
COPY --from=builder /harbor/src/portal /harbor/src/portal

# Build swagger UI
RUN npm install --unsafe-perm && \
    npm run build

# Stage 4: Deploy with Nginx
FROM nginx:alpine

# Install CA certificates
RUN apk add --no-cache ca-certificates

# Copy built Angular app
COPY --from=builder /harbor/src/portal/dist /usr/share/nginx/html

# Copy swagger.json
COPY --from=builder /harbor/src/portal/swagger.json /usr/share/nginx/html/swagger.json

# Copy Swagger UI
COPY --from=swagger-builder /harbor/src/portal/app-swagger-ui/dist /usr/share/nginx/html

# Copy nginx configuration
COPY .dagger/config/portal/nginx.conf /etc/nginx/nginx.conf

WORKDIR /usr/share/nginx/html

# Expose ports
EXPOSE 8080
EXPOSE 8443

# Start nginx
ENTRYPOINT ["nginx", "-g", "daemon off;"]
