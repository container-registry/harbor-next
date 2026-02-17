# Development Dockerfile for Harbor Portal
# Deps installed at build time, source mounted at runtime for HMR

ARG BUN_VERSION
FROM oven/bun:${BUN_VERSION}-alpine
WORKDIR /app
# Copy package files for dependency installation
COPY src/portal/package.json src/portal/bun.lock* ./
# Install deps at build time (skip postinstall - generated API client mounted from host)
RUN bun install --ignore-scripts

# Source code and generated API client mounted at runtime via docker-compose
CMD ["bun", "./node_modules/@angular/cli/bin/ng", "serve", "--host", "0.0.0.0", "--hmr", "--disable-host-check"]
