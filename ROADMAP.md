# Harbor Next Roadmap

A high level roadmap to give users an idea where the next generation **Harbor** is heading towards.

Harbor next is focusing on 3 areas

- Ease of development, deployment and operation with minimum resource utilization (compute and dependencies)
- Extensibility for user self exension of Harbor
- Vendor neutral community


Legend: ✅ Delivered · 🚧 In progress · 🗓️ Planned · 💲 Commercial feature

---

## Delivered in v2.15

- ✅ **CI/CD Pipeline** — Continuous delivery readiness for reliable, extensible workflows locally and in pipelines.
- ✅ **PR Builds** — Each PR gets a set of images for users to grab, verify and use.
- ✅ **Multi-architecture builds** — Build binaries and images for different architectures starting with ARM64 and AMD64
- ✅ **Scratch Images** — Scratch images with minimal size and attack surface.
- ✅ **DEV Env** — Easy contributor onboarding with out of the box local dev environments.
- ✅ **K8s Distributions Support** — Tested support for OpenShift, Rancher, k0s
- ✅ **Built-in connection pooling** — database driver upgraded to pgx/v5 including connection pooling.
- ✅ **Public landing page** — Allow unauthenticated users to browse publicly availibe repositories.
- ✅ **Compose File** — install.sh less and non-root compose file for Docker and Podman.
- ✅ **Docker Distribution** — Use of Docker Distribution V3.
- ✅ **Harbor Satellite** —  Support for Harbor satellite image replication to EDGE.
- ✅ **Customizable Branding** 💲 — system-wide white-label branding (logo, product name, login/about skinning) via REST API and Portal.
- ✅ **Hybrid / multi authentication** 💲 — local DB users alongside an external auth backend (LDAP/OIDC).
- ✅ **SFTP replication adapter** 💲 — replication storage adapter targeting SFTP endpoints.
- ✅ **Pluggable identity providers** 💲 — generalized identity-provider framework with Workload Identity Federation.
- ✅ **Database observability (pgx monitoring)** 💲 — PostgreSQL connection-pool and query metrics exported via OpenTelemetry.
- ✅ **AWS RDS IAM authentication** 💲 — IAM auth for PostgreSQL/S3, removing static DB passwords on RDS and S3.

---

## Next

- 🚧 **Helm Chart** — Versatile Helm Chart.
- 🚧 **Helm Chart Proxy** — Proxy and replicate Charts in Chart Museum Format.
- 🗓 **Maintainer ladder automation** — Avoid single vendor dominance by automated promoting/demotion based on KPIs.
- 🗓 **Pull Through Cache** — True pull through proxy cache.
- 🗓 **Feature Flags** — for easier adoption of new features and feature sandboxing.
- 🗓 **Pluggable Scanner Spec V1.3** — Generic image analysis & chaining beyond SBOM and vulnerabilites.
- 🗓 **Extension Points** — Users can to extend Harbor at various points.
- 🗓 **New Portal** — Node/Angular less portal with simple HTML MPA.
- 🗓 **Drop Redis** — With an architectural redesign Redis is not needed for caching.
- 🗓 **3-tier Stack** — One Application/Container & Database stack.

