# Audit Log Forward via OTLP

## Business Requirement

**Customer driver:** Stackable (Sascha Lautenschlaeger / Nick Larsen; CTO Lars
Francke must report artifact downloads to investors) needs artifact download
analytics: how often artifacts (images, Helm charts) are pulled, and from
where. They currently parse Harbor's syslog forward, which fails for their
analytics use case:

- Syslog messages are unstructured text in mixed formats; filtering and
  grouping requires custom parsing.
- Pull-by-tag and pull-by-digest arrive as separate events that cannot be
  correlated without structured artifact fields.
- Existing forwarded syslog records do not expose request source information in
  a reliable structured field.

**Requirement:** Forward audit log events to an OTLP/HTTP endpoint as structured
OpenTelemetry log records, configurable via UI and API, so customers ingest them
into their own observability stack (Grafana/Loki, Datadog, Elastic, Splunk).
The integration is push-based and vendor-agnostic; no scraping and no custom
parsers.

**Dependencies:** Client IP and User-Agent capture for registry pull audit
events. These fields must be included in forwarded pull events when Harbor sees
the request.

**Target:** Harbor Next 2.16 (September 2026). Stackable has offered to verify
pre-release builds.

## Product Spec

### Configuration

Available in UI System Settings and the config API:

| Key | Values / notes |
| --- | --- |
| `audit_log_forward_otlp_endpoint` | Explicit OTLP/HTTP collector URL, for example `http://otel-collector:4318` or `https://otel-collector:4318` |
| `audit_log_forward_otlp_authentication` | `none` or `basic` |
| `audit_log_forward_otlp_username` / `audit_log_forward_otlp_password` | Basic auth credentials; password is encrypted and write-only |

All settings are manageable via API, including automation through OpenTofu or
similar tools.

The endpoint must include `http://` or `https://`. The URL scheme selects
plaintext HTTP or TLS. Harbor always sends protobuf OTLP logs to `/v1/logs`.

### Event Mapping

Audit events map to OpenTelemetry `LogRecord` values:

- EventName: `harbor.audit.{operation}_{resource_type}`
- Timestamp: audit event `OpTime`
- Severity: `INFO` for successful events, `WARN` for failed events
- Common attributes: `enduser.id`, `audit.action`, `audit.resource`,
  `audit.resource_type`, `harbor.project.id`, `audit.result`,
  `client.address`, `user_agent.original`
- Artifact push/pull attributes, when known: `oci.artifact.repository`,
  `oci.artifact.tag`, `oci.manifest.digest`

`oci.manifest.digest` follows the OpenTelemetry OCI semantic convention. The
repository and tag fields are emitted as `oci.artifact.*` structured attributes
so consumers can count and deduplicate pulls without parsing the opaque
`audit.resource` string.

### Behavior

- OTLP forwarding coexists with syslog forwarding; either may be active.
- `skip_audit_log_database` is allowed when at least one forward endpoint
  (syslog or OTLP) is configured.
- Delivery failures must not block or fail user-facing operations.
- Delivery is best-effort with retry; dropped records are counted and visible to
  operators through logs and `audit_log_otlp_dropped_total{reason}`.
- When `gdpr_audit_logs` is enabled, OTLP forwarding hashes `enduser.id`,
  `client.address`, and `user_agent.original` before emission using Harbor's
  existing audit-log checksum format.
- Harbor does not perform a connectivity probe when saving configuration;
  validation is syntax-only.

### Out Of Scope

- OpenTelemetry metrics signal. Audit events as logs are the delivered signal;
  metrics may be derived downstream in the customer's collector.
- OTLP/gRPC transport.
- mTLS client certificate authentication.
- OAuth2 client-credentials authentication.
- Replacing or deprecating syslog forwarding.
- Guaranteed delivery or durable queueing inside Harbor. Customers needing
  durability should run a local OTel Collector with persistent storage.
- Events Harbor never sees, such as front-proxy cache hits.

### Acceptance Criteria

1. Configure an OTLP/HTTP endpoint in the UI, pull an artifact, and see a
   structured `harbor.audit.pull_artifact` event with `oci.artifact.tag`,
   `oci.manifest.digest`, username, and `client.address` arrive in the receiving
   observability backend.
2. Configure the same OTLP/HTTP endpoint through the config API and verify the
   write-only password behavior.
3. Syslog forwarding is unchanged when OTLP forwarding is unconfigured.
4. OTLP delivery failures do not fail pull/push/API operations and increment a
   visible dropped-record counter.

## POC Implementation

Branch `audit-log-otel` in `container-registry/8gcr`.

- [x] Config keys endpoint / authentication / username / password - API + UI,
  password encrypted and write-only
- [x] OTLP/HTTP export via the OTel Logs SDK, provider rebuilt on config change
- [x] Mapping `harbor.audit.{operation}_{resource_type}` with structured
  attributes, `OpTime` timestamp, and INFO/WARN severity
- [x] `oci.artifact.repository`, `oci.artifact.tag`, and
  `oci.manifest.digest` as separate attributes
- [x] Best-effort delivery: batch + retry, drops counted via
  `audit_log_otlp_dropped_total{reason}`
- [x] Coexists with syslog; `skip_audit_log_database` accepts either endpoint
- [x] Docs include a reference OTel Collector receiver pipeline
- [x] Unit tests: mapping, endpoint parsing, HTTP/HTTPS export, basic auth,
  retry exhaustion, queue overflow, client address normalization, GDPR
  attribute hashing
- [x] AC1 (OTLP/HTTP end to end), AC2 (config API), AC3 (syslog unchanged)
- [x] GDPR mode hashes forwarded `enduser.id`, `client.address`, and
  `user_agent.original`
- [ ] Full end-to-end UI verification with Stackable pre-release build
- [ ] Client IP and User-Agent are forwarded for registry manifest pulls only;
  push/delete events do not currently carry request source fields

## References

- Internal proposal
- Source: email thread "Ideen bzgl OpenTelemetry Metriken ueber OTLP" with
  Stackable (Feb-Apr 2026)
- IP Adresses in Auditlogs (TAS-556)
- OpenTelemetry semantic conventions: OCI attributes and general naming
