FROM docker:29.4.0-dind

ARG GO_VERSION=1.26.3
ARG TASK_VERSION=v3.50.0
ARG COSIGN_VERSION=v3.0.6
ARG COMPOSE_VERSION=v2.40.3
ARG BUILDX_VERSION=v0.33.0

RUN apk add --no-cache \
    bash \
    ca-certificates \
    curl \
    git \
    gzip \
    iproute2 \
    jq \
    openssl \
    tar \
    xz

RUN set -eux; \
    case "$(apk --print-arch)" in \
      x86_64) arch=amd64; compose_arch=x86_64 ;; \
      aarch64) arch=arm64; compose_arch=aarch64 ;; \
      *) echo "unsupported architecture: $(apk --print-arch)" >&2; exit 1 ;; \
    esac; \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" -o /tmp/go.tgz; \
    tar -C /usr/local -xzf /tmp/go.tgz; \
    rm /tmp/go.tgz; \
    curl -fsSL "https://github.com/go-task/task/releases/download/${TASK_VERSION}/task_linux_${arch}.tar.gz" -o /tmp/task.tgz; \
    tar -C /usr/local/bin -xzf /tmp/task.tgz task; \
    rm /tmp/task.tgz; \
    install -d /usr/local/lib/docker/cli-plugins; \
    curl -fsSL "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-${compose_arch}" -o /usr/local/lib/docker/cli-plugins/docker-compose; \
    chmod +x /usr/local/lib/docker/cli-plugins/docker-compose; \
    curl -fsSL "https://github.com/docker/buildx/releases/download/${BUILDX_VERSION}/buildx-${BUILDX_VERSION}.linux-${arch}" -o /usr/local/lib/docker/cli-plugins/docker-buildx; \
    chmod +x /usr/local/lib/docker/cli-plugins/docker-buildx; \
    curl -fsSL "https://github.com/sigstore/cosign/releases/download/${COSIGN_VERSION}/cosign-linux-${arch}" -o /usr/local/bin/cosign; \
    chmod +x /usr/local/bin/cosign

ENV PATH=/usr/local/go/bin:/root/go/bin:$PATH \
    DEV_ROOT=/workspace \
    GOTOOLCHAIN=local

COPY dockerfile/e2e-ci-entrypoint.sh /usr/local/bin/e2e-ci-entrypoint
RUN chmod +x /usr/local/bin/e2e-ci-entrypoint

WORKDIR /workspace
COPY . .

ENTRYPOINT ["e2e-ci-entrypoint"]
CMD ["task", "e2e:run"]
