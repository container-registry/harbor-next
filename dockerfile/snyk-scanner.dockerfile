ARG GO_VERSION=MISSING-BUILD-ARG
ARG NODE_VERSION=MISSING-BUILD-ARG
ARG SNYK_CLI_VERSION=MISSING-BUILD-ARG

FROM docker.io/library/golang:${GO_VERSION}-alpine AS builder
WORKDIR /src
COPY tools/snyk-scanner/go.mod ./
COPY tools/snyk-scanner/main.go ./
RUN CGO_ENABLED=0 go build -o /snyk-scanner .

FROM docker.io/library/node:${NODE_VERSION}-alpine
ARG SNYK_CLI_VERSION
RUN npm install -g snyk@${SNYK_CLI_VERSION} && \
    addgroup -S scanner && \
    adduser -S -G scanner -h /home/scanner scanner
COPY --from=builder /snyk-scanner /home/scanner/bin/snyk-scanner
USER scanner
WORKDIR /home/scanner
EXPOSE 8080
ENTRYPOINT ["/home/scanner/bin/snyk-scanner"]
