# Development Dockerfile for Harbor Portal
# Hot Module Replacement (HMR) for fast frontend iteration

FROM node:18-alpine

# Install git (some npm packages need it)
RUN apk add --no-cache git

WORKDIR /app

# Default command - install deps if needed, then serve with HMR
CMD ["sh", "-c", "[ ! -d node_modules ] && npm install; exec npm start -- --hmr --disable-host-check"]
