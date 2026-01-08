# Development Dockerfile for Harbor Portal
# Hot Module Replacement (HMR) for fast frontend iteration

# Use slim instead of alpine - has prebuilt native binaries for @parcel/watcher
FROM node:18-slim

# Install git (some npm packages need it)
RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Default command - install deps if needed, then serve with HMR
# Use --ignore-scripts to skip @parcel/watcher native build (no ARM64 prebuilts)
# Then run postinstall manually to generate API clients from swagger.yaml
CMD ["sh", "-c", "[ ! -f node_modules/@angular/cli/bin/ng.js ] && npm install --ignore-scripts && npm run postinstall; exec npm start -- --hmr --disable-host-check"]
