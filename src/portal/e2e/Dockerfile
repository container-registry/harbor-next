FROM ubuntu:20.04 AS runtime

ENV TZ=Asia/Shanghai \
    DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8
ENV HELM_EXPERIMENTAL_OCI=1
ENV COSIGN_PASSWORD=Harbor12345
ENV COSIGN_EXPERIMENTAL=1
ENV COSIGN_OCI_EXPERIMENTAL=1
ENV NOTATION_EXPERIMENTAL=1

RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    curl \
    gnupg2 \
    apt-transport-https \
    ca-certificates \
    lsb-release \
    libseccomp2 \
    git \
    iproute2 \
    iptables \
    build-essential \
    sed \
    libssl-dev \
    tar \
    unzip \
    gzip \
    jq \
    libnss3-tools \
    sudo

RUN install -d -m 0755 /etc/apt/keyrings && \
    curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --dearmor -o /etc/apt/keyrings/google-linux.gpg && \
    chmod a+r /etc/apt/keyrings/google-linux.gpg && \
    printf 'deb [arch=amd64 signed-by=/etc/apt/keyrings/google-linux.gpg] https://dl.google.com/linux/chrome/deb/ stable main\n' > /etc/apt/sources.list.d/google.list && \
    apt-get update && \
    apt-get install -y --no-install-recommends google-chrome-stable

RUN curl -fsSL https://bun.sh/install | bash && \
    ln -s /root/.bun/bin/bun /usr/local/bin/bun

RUN DOCKER_VERSION=27.1.1 && \
    wget https://download.docker.com/linux/static/stable/x86_64/docker-$DOCKER_VERSION.tgz && \
    tar --strip-components=1 -xvzf docker-$DOCKER_VERSION.tgz -C /usr/bin && \
    rm docker-$DOCKER_VERSION.tgz

WORKDIR /app

RUN mkdir -p $HOME/.pki/nssdb && \
    echo Harbor12345 > password.ca && \
    certutil -d sql:$HOME/.pki/nssdb -N -f password.ca

VOLUME /var/lib/docker

CMD ["bun", "x", "playwright", "test", "--reporter=html"]

FROM runtime AS bundled

COPY api/v2.0/swagger.yaml /api/v2.0/swagger.yaml
COPY src/portal/package.json src/portal/bun.lock ./
COPY src/portal/scripts ./scripts
COPY src/portal/playwright.config.ts ./playwright.config.ts
COPY src/portal/e2e ./e2e
RUN mkdir -p src && \
    bun install --frozen-lockfile && \
    bun x playwright install --with-deps chromium && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*
