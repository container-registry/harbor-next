{{/*
=============================================================================
Database helpers
=============================================================================
*/}}

{{/*
Return the database host
*/}}
{{- define "harbor.database.host" -}}
{{- .Values.database.host -}}
{{- end }}

{{/*
Return the database port
*/}}
{{- define "harbor.database.port" -}}
{{- .Values.database.port | default 5432 -}}
{{- end }}

{{/*
Return the database name
*/}}
{{- define "harbor.database.database" -}}
{{- .Values.database.database | default "registry" -}}
{{- end }}

{{/*
Return the database username
*/}}
{{- define "harbor.database.username" -}}
{{- .Values.database.username | default "postgres" -}}
{{- end }}

{{/*
Return the database password secret name
*/}}
{{- define "harbor.database.secretName" -}}
{{- if .Values.database.existingSecret }}
{{- .Values.database.existingSecret }}
{{- else }}
{{- include "harbor.fullname" . }}-database
{{- end }}
{{- end }}

{{/*
Return the database sslmode
*/}}
{{- define "harbor.database.sslmode" -}}
{{- .Values.database.sslmode | default "disable" -}}
{{- end }}

{{/*
Whether the chart should mount a TLS cert Secret + inject POSTGRESQL_URL
env for client-side cert verification (or mTLS). Tracks upstream
goharbor/harbor-helm#1859 — managed PG with `verify-full` sslmode had
no way to inject sslrootcert/sslcert/sslkey because the chart only
templated host/port/user/password/sslmode into env vars.
*/}}
{{- define "harbor.database.tlsEnabled" -}}
{{- if ne (.Values.database.existingTlsSecret | toString) "" -}}true{{- end -}}
{{- end -}}

{{/*
Auto-built libpq DSN — passed as POSTGRESQL_URL env. Harbor's
src/lib/dbpool BuildDSN returns cfg.URL verbatim when set, so this
overrides the individual-field DSN and includes the cert paths libpq
needs.

References $(POSTGRESQL_PASSWORD) which K8s substitutes from the env
var defined immediately before (env order matters for substitution).
Client cert + key only included when `database.clientCertEnabled=true`
(libpq fails if sslcert is set but the file is missing).

NOTE: This affects the runtime DB pool only. Harbor's migration tool
(src/common/dao/pgsql.go:NewMigrator) builds its own URL from fields
and does not honor cfg.URL — schema upgrades against a server that
requires mTLS will still fail. Use sslmode=verify-ca or coordinate
migrations via a separate trusted client until that's fixed upstream.
*/}}
{{- define "harbor.database.dsn" -}}
{{- $host := .Values.database.host -}}
{{- $port := .Values.database.port | default 5432 -}}
{{- $user := .Values.database.username | default "postgres" -}}
{{- $db := .Values.database.database | default "registry" -}}
{{- $mode := .Values.database.sslmode | default "verify-full" -}}
{{- $base := printf "host=%s port=%v user=%s password='$(POSTGRESQL_PASSWORD)' dbname=%s sslmode=%s sslrootcert=/etc/harbor/db-tls/ca.crt" $host $port $user $db $mode -}}
{{- if .Values.database.clientCertEnabled -}}
{{- printf "%s sslcert=/etc/harbor/db-tls/tls.crt sslkey=/etc/harbor/db-tls/tls.key" $base -}}
{{- else -}}
{{- $base -}}
{{- end -}}
{{- end -}}

{{/*
Volume for the PG TLS Secret — mounted on core/jobservice/exporter.
*/}}
{{- define "harbor.database.tlsVolume" -}}
{{- if eq (include "harbor.database.tlsEnabled" .) "true" }}
- name: db-tls
  secret:
    secretName: {{ .Values.database.existingTlsSecret }}
    defaultMode: 0400
{{- end }}
{{- end }}

{{/*
VolumeMount block — mounted read-only at /etc/harbor/db-tls.
*/}}
{{- define "harbor.database.tlsVolumeMount" -}}
{{- if eq (include "harbor.database.tlsEnabled" .) "true" }}
- name: db-tls
  mountPath: /etc/harbor/db-tls
  readOnly: true
{{- end }}
{{- end }}

{{/*
POSTGRESQL_URL env entry. Caller must ensure POSTGRESQL_PASSWORD is
listed before this entry in the env list — K8s $(VAR) substitution is
sequence-sensitive.
*/}}
{{- define "harbor.database.tlsEnv" -}}
{{- if eq (include "harbor.database.tlsEnabled" .) "true" }}
- name: POSTGRESQL_URL
  value: {{ include "harbor.database.dsn" . | quote }}
{{- end }}
{{- end }}
