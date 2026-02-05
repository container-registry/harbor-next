# Dockerfile for Harbor Nginx Proxy

FROM nginx:alpine

RUN apk add --no-cache ca-certificates

WORKDIR /

# Expose ports
EXPOSE 8080

# Set entrypoint (line 573)
ENTRYPOINT ["nginx", "-g", "daemon off;"]
