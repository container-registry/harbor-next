# Dockerfile for Harbor Nginx Proxy

ARG LPROBE_VERSION
FROM ghcr.io/fivexl/lprobe:${LPROBE_VERSION} AS lprobe

FROM nginx:alpine

RUN apk add --no-cache ca-certificates
COPY --from=lprobe /lprobe /lprobe

WORKDIR /

# Expose ports
EXPOSE 8080

# Set entrypoint (line 573)
ENTRYPOINT ["nginx", "-g", "daemon off;"]
