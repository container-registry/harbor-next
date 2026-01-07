# Development Dockerfile for Harbor Portal
# Deps installed at build time, source mounted at runtime for HMR

FROM oven/bun:1-alpine

RUN apk add --no-cache nodejs

WORKDIR /app

# Copy everything needed for install
COPY src/portal/package.json src/portal/bun.lock* ./
COPY src/portal/scripts ./scripts
COPY api/v2.0/swagger.yaml ./swagger.yaml

# Install deps at build time (cached by Docker layer)
RUN bun install

# Source code mounted at runtime via docker-compose
CMD ["bun", "run", "start", "--", "--proxy-config", "proxy.config.mjs"]
