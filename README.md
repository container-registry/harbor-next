# Harbor Next

<p align="center"><img alt="Harbor Next" width="256px" src="https://raw.githubusercontent.com/container-registry/harbor-next/refs/heads/main/docs/img/harbor-next-logo.svg"></p>

Harbor is a CNCF graduated open-source container registry to store and manage container images and other OCI artifacts securely with policies, role-based access control, vulnerability scans and signing.

Harbor is hosted by the [Cloud Native Computing Foundation](https://cncf.io)
(CNCF).
If you are an organization that wants to help shape the evolution of Harbor,
[reach out](https://container-registry.com/contact/) to us.

## What is Harbor Next

Harbor Next is a community-driven evolution of Harbor, designed to accelerate innovation and lower the barrier to entry for contributors.
It serves as the foundation for [8gcr](https://container-registry.com/8gcr) (8gears Container Registry), used across enterprises, government agencies, and cloud providers.

We're developing Harbor Next as a [community proposal](https://github.com/goharbor/community/pull/272) with the goal of advancing Harbor and the container registry ecosystem.

- Harbor Next and CNCF Harbor cross-pollinate: Harbor Next cherry-picks features and fixes from Harbor, and upstream Harbor adopts features and concepts proven in Harbor Next.
- Harbor Next follows the same release versioning and cadence as Harbor.
- Harbor Next is a drop-in replacement for Harbor.


## Notable Changes in Harbor Next
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
- Prepending vetted features not yet upstream — see the [release notes](https://github.com/container-registry/harbor-next/releases)
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
* **Easy deployment**: Deploy via Docker/Podman Compose or the Harbor Next Helm chart.


## Architecture

For the architecture design of Harbor Next, see [Architecture Overview](docs/architecture-overview.md).

## API

Harbor Next exposes a RESTful API for administrative operations and integration. Explore it in the [Swagger editor](https://editor.swagger.io/?url=https://raw.githubusercontent.com/container-registry/harbor-next/main/api/v2.0/swagger.yaml) or from the source spec [`api/v2.0/swagger.yaml`](api/v2.0/swagger.yaml).

## Install & Run

**System requirements:** Docker Engine 24+ with Compose v2.24+.

**Docker Compose** — see [deploy/compose/README.md](deploy/compose/README.md):

```bash
cd deploy/compose
cp .env.example .env          # set EXT_ENDPOINT, TLS_CERT/TLS_KEY, and secrets
docker compose up -d
```

**Kubernetes** — install the Harbor Next Helm chart (`deploy/chart/`, also published as an OCI artifact; under active development). Platform guides live in [deploy/chart/docs/guide/](deploy/chart/docs/guide/) (k3s, OpenShift, Rancher, Nutanix).

## Development

Harbor Next uses [Taskfile](https://taskfile.dev) for local development and in pipelines for a fast, hybrid development environment with hot reload capabilities.

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

* **Twitter:** [@project_harbor](https://twitter.com/project_harbor)
* **User Group:** Join Harbor user email group: [harbor-users@lists.cncf.io](https://lists.cncf.io/g/harbor-users) to get update of Harbor's news, features, releases, or to provide suggestion and feedback.
* **Developer Group:** Join Harbor developer group: [harbor-dev@lists.cncf.io](https://lists.cncf.io/g/harbor-dev) for discussion on Harbor development and contribution.
* **Slack:** Join Harbor's community for discussion and ask questions: [Cloud Native Computing Foundation](https://slack.cncf.io/), channel: [#harbor](https://cloud-native.slack.com/messages/harbor/) and [#harbor-dev](https://cloud-native.slack.com/messages/harbor-dev/)

## Demos

* **[Live Demo](https://8gcr.container-registry.dev)** - A demo environment with the latest Harbor-Next build.
* **[Harbor Demo Videos](https://github.com/goharbor/harbor/wiki/Video-demos-for-Harbor)** - Demos for Harbor features and continuously updated.

## Partners and Users

For a list of users, please refer to [ADOPTERS.md](ADOPTERS.md).


## License

Harbor is available under the [Apache 2 license](LICENSE).
