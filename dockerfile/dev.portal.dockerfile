# Development Dockerfile for Harbor Portal
# Deps installed at build time, source mounted at runtime for HMR

ARG BUN_VERSION=MISSING-BUILD-ARG
FROM oven/bun:${BUN_VERSION}-alpine

WORKDIR /app
# Copy package files for dependency installation
COPY src/portal/package.json src/portal/bun.lock* ./
RUN bun install --ignore-scripts

WORKDIR /swagger-ui
COPY src/portal/app-swagger-ui/package.json src/portal/app-swagger-ui/package-lock.json ./
RUN bun install --ignore-scripts

WORKDIR /app
COPY src/portal/scripts/dev-portal-start.js /app/scripts/dev-portal-start.js
HEALTHCHECK --interval=10s --timeout=5s --retries=5 CMD wget -q -O /dev/null http://127.0.0.1:4200 || exit 1
USER bun
CMD ["bun", "/app/scripts/dev-portal-start.js"]
