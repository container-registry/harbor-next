# 8gcr

<p align="center"><img alt="8gcr" width="256px" src="https://raw.githubusercontent.com/container-registry/8gcr/refs/heads/main/docs/img/8gcr-logo.svg"></p>

Harbor is a CNCF graduated open-source container registry to store and manage container images and other OCI artifacts securely with policies, role-based access control, vulnerability scans and signing.

Harbor is hosted by the [Cloud Native Computing Foundation](https://cncf.io)
(CNCF).
If you are an organization that wants to help shape the evolution of Harbor,
[reach out](https://container-registry.com/contact/) to us.

## What is 8gcr

This project, 8gcr, is a community-driven evolution of Harbor, designed to accelerate innovation and lower the barrier to entry for contributors.
It is the open source foundation of the [8gcr](https://container-registry.com/8gcr) (8gears Container Registry) commercial distribution, used across enterprises, government agencies, and cloud providers.

We're developing 8gcr as a [community proposal](https://github.com/goharbor/community/pull/272) with the goal of advancing Harbor and the container registry ecosystem.

- 8gcr and CNCF Harbor cross-pollinate: 8gcr cherry-picks features and fixes from Harbor, and upstream Harbor adopts features and concepts proven in 8gcr.
- 8gcr follows the same release versioning and cadence as Harbor.
- 8gcr is a drop-in replacement for Harbor.


## Notable Changes in 8gcr
- Contributor/Maintainer ladder automation
- Continuous delivery
- Easy Contributor onboarding with out of the box dev environments
- Multi-architecture artifacts
- Scratch images with minimal size and attack surface.
- Use of Docker Distribution V3
- Replicate images to SFTP endpoints
- Harbor Satellite Support
- Versatile Helm Chart
- Open Compose (install.sh less) supporting Docker & Podman Compose
- Support for OpenShift, Rancher, k3s, Nutanix (NKP)
- Prepending vetted features not yet upstream — see the [release notes](https://github.com/container-registry/8gcr/releases)
- See [ROADMAP.md](/ROADMAP.md) for more...

## Harbor Features

* **Cloud native registry**: Stores container images and OCI artifacts, including Helm charts as OCI artifacts, for container runtimes and orchestration platforms.
* **Role based access control**: Users access repositories through 'projects', with per-project permissions on the artifacts they contain.
* **Policy based replication**: Replicate artifacts between registry instances by policy and filters (repository, tag, label) with automatic retries — for load balancing, high availability, and multi-datacenter/hybrid/multi-cloud deployments.
* **Vulnerability scanning**: Scans images for vulnerabilities and enforces policy checks to block vulnerable images from being deployed.
* **LDAP/AD support**: Integrates with enterprise LDAP/AD for authentication and group import, mapping groups to project permissions.
* **OIDC support**: Authenticates users via OpenID Connect against an external identity provider, with single sign-on into the portal.
* **Image deletion & garbage collection**: Delete artifacts and reclaim storage by running garbage collection on dangling manifests and unreferenced blobs.
* **Image signing & verification**: Released images are signed with keyless [cosign](https://github.com/sigstore/cosign) signatures — see [docs/signature-verification.md](docs/signature-verification.md).
* **Graphical user portal**: Browse, search repositories, and manage projects from the web UI.
* **Auditing**: All repository operations are tracked through logs.
* **RESTful API**: REST APIs for administrative operations and integration, with an embedded Swagger UI for exploring and testing.
* **Easy deployment**: Deploy via Docker/Podman Compose or the 8gcr Helm chart.


## Architecture

For the architecture design of 8gcr, see [Architecture Overview](docs/architecture-overview.md).

## API

8gcr exposes a RESTful API for administrative operations and integration. Explore it in the [Swagger editor](https://editor.swagger.io/?url=https://raw.githubusercontent.com/container-registry/8gcr/main/api/v2.0/swagger.yaml) or from the source spec [`api/v2.0/swagger.yaml`](api/v2.0/swagger.yaml).

## Install & Run

**System requirements:** Docker Engine 24+ with Compose v2.24+.

**Docker Compose** — see [deploy/compose/README.md](deploy/compose/README.md):

```bash
cd deploy/compose
cp .env.example .env          # set EXT_ENDPOINT, TLS_CERT/TLS_KEY, and secrets
docker compose up -d
```

**Kubernetes** — install the 8gcr Helm chart (`deploy/chart/`, also published as an OCI artifact; under active development). Platform guides live in [deploy/chart/docs/guide/](deploy/chart/docs/guide/) (k3s, OpenShift, Rancher, Nutanix).

## Development

8gcr uses [Taskfile](https://taskfile.dev) for local development and in pipelines for a fast, hybrid development environment with hot reload capabilities.

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| [Task](https://taskfile.dev/installation/) | v3.x | Build system (replaces Make) |
| [Docker](https://docs.docker.com/get-docker/) / [Podman](https://podman.io/) | 20.10.10+ | Dev environment, linting, image builds |
| [Go](https://go.dev/dl/) | see `versions.env` | Backend compilation and tests |
| [Bun](https://bun.sh) | see `versions.env` | Frontend dependency management |
| [Node.js](https://nodejs.org/) | 16+ | Frontend build, tests, and API codegen |
| Git | any | Required by build metadata and mock checks |

Additional Go tools (`air`, `dlv`, `govulncheck`) are auto-installed on first use via `go install` or by running `task setup`.

### Quick Start

```bash
# Install Task
brew install go-task/tap/go-task  # macOS
# or: sh -c "$(curl --location https://taskfile.dev/install.sh)" -- -d  # Linux

# Start DevEnv
task

# Open http://localhost:4200
```

### Common Commands

```bash
task                     # Start full dev environment (foreground)
task setup               # Install development tools (air, dlv, govulncheck)
task build               # Build all binaries (alias: task b:all-binaries)
task images              # Build all Docker images
task test                # Run all tests (alias: task t:all)
task lint                # Run API lint, Go lint, and vuln-check
task test:unit:pure      # Run pure unit tests only
task test:unit:db        # Run DB-backed unit tests only
task clean               # Clean build artifacts
task info                # Show build info and tool versions
task -l                  # List all available tasks
```

Namespace aliases: `b:` (build), `t:` (test), `img:` (image), `d:` (dev).

See [devenv/README.md](devenv/README.md) for detailed development environment commands.

## Contributing

Contributions are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for the PR workflow, commit conventions (Conventional Commits + DCO sign-off), squash-merge rules, and how releases work. New contributors can get a dev environment running with `task` (see [Development](#development)).

## Compatibility

The [compatibility list](https://goharbor.io/docs/edge/install-config/harbor-compatibility-list/) document provides compatibility information for the Harbor components.

* [Replication adapters](https://goharbor.io/docs/edge/install-config/harbor-compatibility-list/#replication-adapters)
* [OIDC adapters](https://goharbor.io/docs/edge/install-config/harbor-compatibility-list/#oidc-adapters)
* [Scanner adapters](https://goharbor.io/docs/edge/install-config/harbor-compatibility-list/#scanner-adapters)

## Community

* **Slack:** Join the [Cloud Native Computing Foundation](https://slack.cncf.io/) Slack and find us in the [#harbor-next](https://cloud-native.slack.com/archives/C0B3SFW2N4T) channel for discussion and questions.
* **Community Meetings:** We hold regular community meetings open to everyone. See the [meeting schedule and notes](https://hackmd.io/@harbor/meetings) for dates, agenda, and how to join.

## Demos

* **[Live Demo](https://8gcr.container-registry.dev)** - A demo environment with the latest 8gcr build.
* **[Harbor Demo Videos](https://github.com/goharbor/harbor/wiki/Video-demos-for-Harbor)** - Demos for Harbor features and continuously updated.

## Partners and Users

For a list of users, please refer to [ADOPTERS.md](ADOPTERS.md).


## License

Harbor is available under the [Apache 2 license](LICENSE).
