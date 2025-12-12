# EXT_ENDPOINT vs CORE_URL - serve different purposes

EXT_ENDPOINT (External Endpoint)

Purpose: The public-facing URL that end users and external systems use to access Harbor.

Used for:
- Browser redirects - When redirecting users after login/logout (OIDC flows) - src/core/controllers/base.go:80,124, src/core/controllers/oidc.go:311
- Webhook payloads - External URLs in webhook notifications - src/controller/event/handler/webhook/artifact/replication.go:141
- P2P preheat - URLs sent to CDN/preheat providers - src/controller/p2p/preheat/enforcer.go:151
- Scanner config - Registry endpoint for vulnerability scanners - src/controller/scan/base_controller.go:155
- CSRF cookie security - Determines if HTTPS (secure cookie) - src/server/middleware/csrf/csrf.go:105

Default: https://host01.com

  ---
CORE_URL (Internal Core URL)

Purpose: The internal URL for service-to-service communication within Harbor's microservices architecture.

Used for:
- Docker registry auth - Token service endpoint for Docker V2 API auth - src/server/middleware/v2auth/auth.go:120
- Local registry identification - Identifying "Local" registry for replication - src/pkg/reg/manager.go:174, src/controller/event/handler/webhook/artifact/replication.go:235
- Internal requests - Harbor satellite adapter routing - src/pkg/reg/adapter/harborsatellite/adapter.go:79,85
- Token service endpoint - InternalTokenServiceEndpoint() = CORE_URL + /service/token

Default: http://core:8080 (Docker service name)

  ---
In Production (Kubernetes/Docker Compose)

                      Internet
                         │
                         ▼
                ┌────────────────┐
                │  Load Balancer │  ← EXT_ENDPOINT = https://harbor.example.com
                │   (nginx/ingress)
                └────────┬───────┘
                         │
          ┌──────────────┼──────────────┐
          │              │              │
          ▼              ▼              ▼
      ┌───────┐     ┌───────┐     ┌───────────┐
      │ Core  │◄───►│ Job   │◄───►│ Registry  │
      │       │     │Service│     │           │
      └───────┘     └───────┘     └───────────┘
           │
           └── CORE_URL = http://core:8080 (internal Docker network)

They are different because:
- External users hit https://harbor.example.com (with TLS termination at load balancer)
- Internal services communicate via http://core:8080 (no TLS, Docker DNS)

  ---
In Your Dev Setup (Tunnel)

For development with Cloudflare tunnel, they can be the same because everything goes through the tunnel:

EXT_ENDPOINT: https://macfly4200.8gears.ch   # User-facing URL
CORE_URL: https://macfly4200.8gears.ch       # Also used for Docker auth token URL

But in a typical production setup, they would be different:
EXT_ENDPOINT: https://harbor.example.com     # Public URL
CORE_URL: http://core:8080                   # Internal service mesh URL

  ---
Why both are needed in your tunnel case

The CORE_URL specifically controls the Docker token service URL returned in Www-Authenticate headers. Without setting it, Docker clients would be told to get tokens from http://core:8080 which doesn't exist outside the container network.
