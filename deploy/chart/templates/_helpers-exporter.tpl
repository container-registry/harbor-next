{{/*
=============================================================================
Exporter helpers
=============================================================================
*/}}

{{/*
The chart-managed exporter env, emitted as a YAML list rather than inline in
the Deployment so `exporter.deployment.yaml` can filter it against the
`exporter.config` / `exporter.secret` passthrough.

Order is load-bearing: REDIS_PASSWORD must precede HARBOR_REDIS_URL, because
Kubernetes only expands $(VAR) from variables earlier in the env list.

Every `value` must be a string. The Deployment round-trips this list through
fromYamlArray/toYaml, so an unquoted number would come back as an int and
Kubernetes rejects a non-string env value.
*/}}
{{- define "harbor.exporter.chartEnv" -}}
- name: HARBOR_EXPORTER_PORT
  value: "8001"
- name: HARBOR_EXPORTER_METRICS_PATH
  value: "/metrics"
- name: HARBOR_EXPORTER_METRICS_ENABLED
  value: "true"
- name: HARBOR_EXPORTER_MAX_REQUESTS
  value: "30"
- name: HARBOR_EXPORTER_TLS_CERT
  value: ""
- name: HARBOR_EXPORTER_TLS_KEY
  value: ""
- name: HARBOR_SERVICE_SCHEME
  value: "http"
- name: HARBOR_SERVICE_HOST
  value: {{ include "harbor.fullname" . }}-core
- name: HARBOR_SERVICE_PORT
  value: "80"
- name: HARBOR_DATABASE_HOST
  value: {{ include "harbor.database.host" . | quote }}
- name: HARBOR_DATABASE_PORT
  value: {{ include "harbor.database.port" . | quote }}
- name: HARBOR_DATABASE_USERNAME
  value: {{ include "harbor.database.username" . | quote }}
- name: HARBOR_DATABASE_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "harbor.database.secretName" . }}
      key: {{ .Values.database.existingSecretKey | default "POSTGRESQL_PASSWORD" }}
- name: HARBOR_DATABASE_DBNAME
  value: {{ include "harbor.database.database" . | quote }}
- name: HARBOR_DATABASE_SSLMODE
  value: {{ include "harbor.database.sslmode" . | quote }}
- name: HARBOR_DATABASE_MAX_OPEN_CONNS
  value: {{ .Values.database.maxOpenConns | default 100 | quote }}
- name: HARBOR_DATABASE_MIN_CONNS
  value: {{ include "harbor.database.minConns" . | quote }}
- name: HARBOR_DATABASE_CONN_MAX_IDLE_TIME
  value: {{ .Values.database.connMaxIdleTime | quote }}
- name: HARBOR_DATABASE_CONN_MAX_LIFETIME
  value: {{ .Values.database.connMaxLifetime | quote }}
{{- $extAuth := and (not .Values.valkey.enabled) (or .Values.externalRedis.existingSecret .Values.externalRedis.password) -}}
{{- if or .Values.valkey.auth.enabled $extAuth }}
- name: REDIS_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ include "harbor.redis.secretName" . }}
      key: {{ include "harbor.redis.secretKey" . }}
{{- end }}
- name: HARBOR_REDIS_URL
  value: {{ include "harbor.redis.url.jobservice" . | quote }}
- name: HARBOR_REDIS_NAMESPACE
  value: "harbor_job_service_namespace"
- name: HARBOR_REDIS_TIMEOUT
  value: "3600"
{{- end }}

{{/*
The env-var names the user has claimed via `exporter.config` / `exporter.secret`,
as a YAML map (values are irrelevant — only the keys are used).

Both blocks are normalized through harbor.toEnvVars first, so the flat form
(`config: {HARBOR_DATABASE_MIN_CONNS: 0}`) and the nested form
(`config: {harbor: {database: {min_conns: 0}}}`) are both recognized.

NOTE: toEnvVars adds no prefix of its own. The exporter reads env through
viper with SetEnvPrefix("harbor"), so a key only reaches the process if it
carries the HARBOR_ prefix itself.
*/}}
{{- define "harbor.exporter.envOverrides" -}}
{{- $cfg := include "harbor.toEnvVars" (dict "values" .Values.exporter.config "prefix" "" "isSecret" false) -}}
{{- $secret := include "harbor.toEnvVars" (dict "values" .Values.exporter.secret "prefix" "" "isSecret" true) -}}
{{- printf "%s\n%s" $cfg $secret -}}
{{- end }}
