# Forward audit logs with OTLP/HTTP

Harbor can forward each enabled extended audit event as an OpenTelemetry
`LogRecord` while continuing to write the event to syslog and the audit
database. Configure forwarding under **Administration > Configuration > Audit
Log**.

Set `audit_log_forward_otlp_endpoint` to an explicit `http://` or `https://`
collector URL. Harbor sends protobuf OTLP requests to `/v1/logs`; the URL
scheme controls whether the connection uses TLS. HTTPS certificates must be
trusted by the `harbor-core` container's system CA store.

Authentication can be `none` or `basic`. Basic authentication requires
`audit_log_forward_otlp_username` and `audit_log_forward_otlp_password`. The
password is write-only: configuration GET responses omit it, and update
requests preserve the stored password unless they include the password field.

## Delivery behavior

Forwarding is best effort and never fails an artifact or API operation. Harbor
uses the OpenTelemetry SDK's bounded in-memory batch processor, timeout, and
retry behavior. There is no disk spool or replay after a core restart.

Exporter health is available through:

```text
audit_log_otlp_dropped_total{reason="queue_full"}
audit_log_otlp_dropped_total{reason="export_error"}
audit_log_otlp_dropped_total{reason="shutdown"}
```

These counters describe exporter failures; Harbor does not convert audit events
into measurements. Each accepted audit event remains one OTLP log record.

## Collector verification

The following Collector configuration receives Harbor audit log records and
prints them for verification:

```yaml
extensions:
  basicauth/harbor:
    htpasswd:
      file: /etc/otelcol/harbor.htpasswd

receivers:
  otlp/harbor:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
        auth:
          authenticator: basicauth/harbor

exporters:
  debug:
    verbosity: detailed

service:
  extensions: [basicauth/harbor]
  pipelines:
    logs/harbor:
      receivers: [otlp/harbor]
      exporters: [debug]
```

Configure Harbor with `http://otel-collector:4318` for an unencrypted local
receiver or `https://otel-collector:4318` when the receiver has TLS configured.
Harbor validates configuration syntax locally and does not probe collector
reachability when settings are saved.
