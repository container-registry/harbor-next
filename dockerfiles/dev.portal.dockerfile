# Development Dockerfile for Harbor Portal
# Deps installed at build time, source mounted at runtime for HMR

FROM oven/bun:1-alpine

RUN apk add --no-cache nodejs

WORKDIR /app

# Copy package files for dependency installation
COPY src/portal/package.json src/portal/bun.lock* ./

# Install deps at build time (skip postinstall - swagger.yaml will be mounted at runtime)
RUN bun install --ignore-scripts

# Source code and swagger.yaml mounted at runtime via docker-compose
# Run postinstall to generate API client, then start dev server
CMD ["sh", "-c", "bun run postinstall && bun run start -- --proxy-config proxy.config.mjs"]