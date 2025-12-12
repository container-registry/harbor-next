# Dockerfile for Harbor Nginx Proxy
# Based on .dagger/main.go buildNginx logic (lines 566-574)

FROM nginx:alpine

# Copy CA certificates
COPY --from=alpine:latest /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

WORKDIR /

# Expose ports
EXPOSE 8080

# Set entrypoint (line 573)
ENTRYPOINT ["nginx", "-g", "daemon off;"]
