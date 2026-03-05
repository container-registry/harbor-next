{{/*
Expand the name of the chart.
*/}}
{{- define "harbor.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "harbor.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "harbor.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "harbor.labels" -}}
helm.sh/chart: {{ include "harbor.chart" . }}
{{ include "harbor.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}


{{/*
Selector labels
*/}}
{{- define "harbor.selectorLabels" -}}
app.kubernetes.io/name: {{ include "harbor.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component labels - adds component name to common labels
Usage: {{ include "harbor.componentLabels" (dict "root" . "component" "core") }}
*/}}
{{- define "harbor.componentLabels" -}}
{{ include "harbor.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
Component selector labels
Usage: {{ include "harbor.componentSelectorLabels" (dict "root" . "component" "core") }}
*/}}
{{- define "harbor.componentSelectorLabels" -}}
{{ include "harbor.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{/*
=============================================================================
toEnvVars - Convert nested map to flat environment variables
=============================================================================
This is the core helper that enables future-proof configuration.
Any Harbor config option can be set in values.yaml without chart changes.

Usage in ConfigMap:
  {{- include "harbor.toEnvVars" (dict "values" .Values.core.config "prefix" "" "isSecret" false) | nindent 2 }}

Usage in Secret:
  {{- include "harbor.toEnvVars" (dict "values" .Values.core.secret "prefix" "" "isSecret" true) | nindent 2 }}

Example input:
  config:
    storage:
      type: s3
      s3:
        bucket: my-bucket
        region: us-east-1

Example output (ConfigMap):
  STORAGE_TYPE: "s3"
  STORAGE_S3_BUCKET: "my-bucket"
  STORAGE_S3_REGION: "us-east-1"

Example output (Secret):
  STORAGE_TYPE: "czM="
  STORAGE_S3_BUCKET: "bXktYnVja2V0"
  STORAGE_S3_REGION: "dXMtZWFzdC0x"
*/}}
{{- define "harbor.toEnvVars" -}}
{{- $prefix := "" }}
{{- if .prefix }}{{- $prefix = printf "%s_" (.prefix | upper) }}{{- end }}
{{- range $key, $value := .values }}
{{- if kindIs "map" $value }}
{{- /* Recursively process nested maps */ -}}
{{- include "harbor.toEnvVars" (dict "values" $value "prefix" (printf "%s%s" $prefix ($key | upper)) "isSecret" $.isSecret) }}
{{- else if kindIs "slice" $value }}
{{- /* Join arrays with comma */ -}}
{{- if $.isSecret }}
{{ $prefix }}{{ $key | upper }}: {{ $value | join "," | b64enc | quote }}
{{- else }}
{{ $prefix }}{{ $key | upper }}: {{ $value | join "," | quote }}
{{- end }}
{{- else if kindIs "bool" $value }}
{{- /* Handle booleans */ -}}
{{- if $.isSecret }}
{{ $prefix }}{{ $key | upper }}: {{ $value | toString | b64enc | quote }}
{{- else }}
{{ $prefix }}{{ $key | upper }}: {{ $value | toString | quote }}
{{- end }}
{{- else if not (kindIs "invalid" $value) }}
{{- /* Handle strings and numbers, skip nil/empty */ -}}
{{- if $.isSecret }}
{{ $prefix }}{{ $key | upper }}: {{ $value | toString | b64enc | quote }}
{{- else }}
{{ $prefix }}{{ $key | upper }}: {{ $value | toString | quote }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}


{{/*
=============================================================================
Core helpers
=============================================================================
*/}}


{{- define "harbor.secretKeyHelper" -}}
  {{- if and (not (empty .data)) (hasKey .data .key) }}
    {{- index .data .key | b64dec -}}
  {{- end -}}
{{- end -}}

{{/*
=============================================================================
Image helpers
=============================================================================
*/}}

{{/*
Return the proper image name
Usage: {{ include "harbor.image" (dict "imageRoot" .Values.core.image "global" .Values.image "chart" .Chart) }}
*/}}
{{- define "harbor.image" -}}
{{- $tag := .imageRoot.tag | default .chart.AppVersion -}}
{{- printf "%s:%s" .imageRoot.repository $tag -}}
{{- end }}

{{/*
Return image pull policy
*/}}
{{- define "harbor.imagePullPolicy" -}}
{{- .Values.image.pullPolicy | default "IfNotPresent" -}}
{{- end }}

{{/*
Return image pull secrets
*/}}
{{- define "harbor.imagePullSecrets" -}}
{{- $hasSecrets := or .Values.imagePullSecrets (and .Values.imageCredentials .Values.imageCredentials.registry) }}
{{- if $hasSecrets }}
imagePullSecrets:
{{- range .Values.imagePullSecrets }}
{{- if kindIs "map" . }}
  - name: {{ .name }}
{{- else }}
  - name: {{ . }}
{{- end }}
{{- end }}
{{- if and .Values.imageCredentials .Values.imageCredentials.registry }}
  - name: {{ .Release.Name }}-registry-creds
{{- end }}
{{- end }}
{{- end }}

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
=============================================================================
Redis helpers
=============================================================================
*/}}

{{/*
Return the Redis host
*/}}
{{- define "harbor.redis.host" -}}
{{- if .Values.valkey.enabled }}
{{- .Values.valkey.fullnameOverride | default (printf "%s-valkey" .Release.Name) }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end -}}

{{/*
Return the Redis host with port
*/}}
{{- define "harbor.redis.hostWithPort" -}}
{{- include "harbor.redis.host" . }}:{{ include "harbor.redis.port" . }}
{{- end }}

{{/*
Return the Redis port
*/}}
{{- define "harbor.redis.port" -}}
{{- if .Values.valkey.enabled }}
{{- 6379 }}
{{- else }}
{{- .Values.externalRedis.port | default 6379 }}
{{- end }}
{{- end }}

{{/*
Return the Redis password secret name
*/}}
{{- define "harbor.redis.secretName" -}}
{{- if .Values.valkey.enabled }}
{{- .Release.Name }}-valkey
{{- else if .Values.externalRedis.existingSecret }}
{{- .Values.externalRedis.existingSecret }}
{{- else }}
{{- include "harbor.fullname" . }}-redis
{{- end }}
{{- end }}

{{/*
Return the Redis password key in the secret
*/}}
{{- define "harbor.redis.secretKey" -}}
{{- if .Values.valkey.auth.enabled -}}
valkey-password
{{- else -}}
REDIS_PASSWORD
{{- end -}}
{{- end }}

{{/*
Return the Base Redis URL for Harbor components
*/}}
{{- define "harbor.redis.url" -}}
  {{- $root := . -}}
  {{- $host := include "harbor.redis.host" $root -}}
  {{- $port := include "harbor.redis.port" $root -}}
  {{- if .Values.valkey.auth.enabled -}}
    {{- printf "redis://:$(REDIS_PASSWORD)@%s:%s" $host $port -}}
  {{- else -}}
    {{- printf "redis://%s:%s" $host $port -}}
  {{- end -}}
{{- end }}

{{/*
Return the Redis URL for Harbor core
*/}}
{{- define "harbor.redis.url.core" -}}
  {{ include "harbor.redis.url" . }}/0?idle_timeout_seconds=30
{{- end -}}

{{- define "harbor.redis.masterSet" -}}
{{- .Values.externalRedis.sentinelMasterSet -}}
{{- end -}}

{{- define "harbor.redis.scheme" -}}
  {{- if .Values.valkey.enabled -}}
    {{- print "redis" -}}
  {{- else -}}
    {{- if .Values.externalRedis.sentinelMasterSet -}}
      {{- ternary "rediss+sentinel" "redis+sentinel" .Values.externalRedis.tlsOptions.enable -}}
    {{- else -}}
      {{- ternary "rediss" "redis" .Values.externalRedis.tlsOptions.enable -}}
    {{- end -}}
  {{- end -}}
{{- end -}}

{{/* scheme://[:password@]addr/db_index?idle_timeout_seconds=30 */}}
{{- define "harbor.redis.url.harbor" -}}
    {{ include "harbor.redis.url" . }}/6?idle_timeout_seconds=30
{{- end -}}

{{- define "harbor.redis.url.registry.num" -}}
2
{{- end -}}

{{- define "harbor.redis.url.registry" -}}
  {{ include "harbor.redis.url" . }}/{{ include "harbor.redis.url.registry.num" . }}?idle_timeout_seconds=30
{{- end -}}

{{- define "harbor.redis.url.jobservice" -}}
  {{ include "harbor.redis.url" . }}/1
{{- end -}}

{{- define "harbor.redis.url.trivy" -}}
  {{ include "harbor.redis.url" . }}/5
{{- end -}}

{{- define "harbor.redis.url.cache" -}}
  {{- $url := include "harbor.redis.url" . -}}
  {{- printf "%s/7?idle_timeout_seconds=30" $url -}}
{{- end -}}

{{- define "harbor.redis.password" -}}
  {{- ternary "" .Values.externalRedis.password (.Values.valkey.enabled) }}
{{- end -}}

{{- define "harbor.redis.enableTLS" -}}
  {{- ternary "true" "false" (and (not .Values.valkey.enabled) (and .Values.externalRedis .Values.externalRedis.tlsOptions .Values.externalRedis.tlsOptions.enable)) }}
{{- end -}}

{{/*
=============================================================================
Internal URL helpers
=============================================================================
*/}}

{{/*
Return the Core internal URL
*/}}
{{- define "harbor.core.url" -}}
http://{{ include "harbor.fullname" . }}-core
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.core.port" -}}
8080
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.core.service.port" -}}
80
{{- end }}

{{/* TOKEN_SERVICE_URL */}}
{{- define "harbor.token.service.url" -}}
{{ include "harbor.core.url" . }}/service/token
{{- end -}}

{{/*
Return the Portal internal URL
*/}}
{{- define "harbor.portal.url" -}}
http://{{ include "harbor.fullname" . }}-portal
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.portal.port" -}}
80
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.portal.service.port" -}}
8080
{{- end }}

{{/*
Return the Registry name
*/}}
{{- define "harbor.registry.name" -}}
{{ include "harbor.fullname" . }}-registry
{{- end }}

{{/*
Return the Registry internal URL
*/}}
{{- define "harbor.registry.url" -}}
http://{{ include "harbor.fullname" . }}-registry:5000
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.registry.port" -}}
5000
{{- end }}

{{/*
Return the Registry controller internal URL
*/}}
{{- define "harbor.registryctl.url" -}}
http://{{ include "harbor.fullname" . }}-registry:{{ include "harbor.registryctl.port" . }}
{{- end }}

{{/*
Registryctl container port
*/}}
{{- define "harbor.registryctl.port" -}}
8080
{{- end }}

{{/*
Return the Trivy adapter URL (if enabled)
*/}}
{{- define "harbor.trivy.url" -}}
http://{{ include "harbor.fullname" . }}-trivy:8080
{{- end }}

{{- define "harbor.trivy.enabled" -}}
{{ .Values.trivy.enabled }}
{{- end }}

{{/*
=============================================================================
Service Account helpers
=============================================================================
*/}}

{{/*
Create the name of the service account to use for a component
Usage: {{ include "harbor.serviceAccountName" (dict "root" . "component" "core" "serviceAccount" .Values.core.serviceAccount) }}
*/}}
{{- define "harbor.serviceAccountName" -}}
{{- if .serviceAccount.create }}
{{- default (printf "%s-%s" (include "harbor.fullname" .root) .component) .serviceAccount.name }}
{{- else }}
{{- default "default" .serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
=============================================================================
Secret Key helpers
=============================================================================
*/}}

{{/*
Return the secret key for encryption
*/}}
{{- define "harbor.secretKey" -}}
{{- if .Values.secretKey }}
{{- .Values.secretKey }}
{{- else }}
{{- /* Generate a deterministic key based on release name */ -}}
{{- $key := printf "%s-harbor-secret-key" .Release.Name | sha256sum | trunc 16 }}
{{- $key }}
{{- end }}
{{- end }}

{{/*
=============================================================================
External URL helpers
=============================================================================
*/}}

{{/*
Return the external URL
*/}}
{{- define "harbor.externalURL" -}}
{{- .Values.externalURL }}
{{- end }}

{{/*
Return the core external URL (same as externalURL for now)
*/}}
{{- define "harbor.coreURL" -}}
{{- include "harbor.externalURL" . }}
{{- end }}

{{/*
=============================================================================
TLS helpers
=============================================================================
*/}}

{{/*
Check if internal TLS is enabled
*/}}
{{- define "harbor.component.scheme" -}}
  {{- printf "http" -}}
{{- end }}

{{- define "harbor.middlware.enabled" -}}
{{- false }}
{{- end }}

{{/*
Return the TLS secret name for a component
Usage: {{ include "harbor.tlsSecretName" (dict "root" . "component" "core") }}
*/}}
{{- define "harbor.tlsSecretName" -}}
{{- if eq .component "core" }}
{{- if .root.Values.tls.customSecrets.core }}
{{- .root.Values.tls.customSecrets.core }}
{{- else }}
{{- include "harbor.fullname" .root }}-core-tls
{{- end }}
{{- else if eq .component "registry" }}
{{- if .root.Values.tls.customSecrets.registry }}
{{- .root.Values.tls.customSecrets.registry }}
{{- else }}
{{- include "harbor.fullname" .root }}-registry-tls
{{- end }}
{{- end }}
{{- end }}

{{- define "harbor.autoGenCert" -}}
  {{- .Values.ingress.autoGenCert -}}
{{- end -}}

{{- define "harbor.metrics.portName" -}}
  {{- if .Values.tls.enabled }}
    {{- printf "https-metrics" -}}
  {{- else -}}
    {{- printf "http-metrics" -}}
  {{- end -}}
{{- end -}}

{{/*
=============================================================================
Validation helpers
=============================================================================
*/}}

{{/*
Validate required values
*/}}
{{- define "harbor.validateValues" -}}
{{- if not .Values.externalURL }}
{{- fail "externalURL is required. Please set externalURL in your values." }}
{{- end }}
{{- if not .Values.database.host }}
{{- fail "database.host is required. Please set database.host in your values." }}
{{- end }}
{{- if not .Values.harborAdminPassword }}
{{- fail "harborAdminPassword is required. Please set harborAdminPassword in your values (minimum 8 characters)." }}
{{- end }}
{{- end }}


{{/*
=============================================================================
Jobservice helpers
=============================================================================
*/}}

{{- define "harbor.jobservice" -}}
  {{- printf "%s-jobservice" (include "harbor.fullname" .) -}}
{{- end -}}

{{/*
Return the Jobservice internal URL
*/}}
{{- define "harbor.jobservice.url" -}}
http://{{ include "harbor.fullname" . }}-jobservice
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.jobservice.port" -}}
8080
{{- end }}

{{- define "harbor.redis.urlForJobservice" -}}
{{ include "harbor.redis.url" . }}
{{- end -}}

{{/*
the max time to wait for a task to finish, if unfinished after max_update_hours, the task will be mark as error, but the task will continue to run, default value is 24
*/}}
{{- define "harbor.jobservice.reaper.max_update_hours" -}}
24
{{- end }}

{{/*
the max time for execution in running state without new task created
*/}}
{{- define "harbor.jobservice.reaper.max_dangling_hours" -}}
168
{{- end }}

{{- define "harbor.jobservice.notification.webhook_job_max_retry" -}}
3
{{- end }}

{{/*
in seconds
*/}}
{{- define "harbor.jobservice.notification.webhook_job_http_client_timeout" -}}
3
{{- end }}

{{- define "harbor.jobservice.secretName" -}}
  {{- if eq .Values.tls.certSource "secret" -}}
    {{- .Values.jobservice.secretName -}}
  {{- else -}}
    {{- printf "%s-jobservice-internal-tls" (include "harbor.fullname" .) -}}
  {{- end -}}
{{- end -}}


{{/*
=============================================================================
Metrics helpers
=============================================================================
*/}}


{{/*
Container subpath
*/}}
{{- define "harbor.metrics.path" -}}
/metrics
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.metrics.port" -}}
8001
{{- end }}


{{/*
=============================================================================
Proxy helpers
=============================================================================
*/}}


{{- define "harbor.portal" -}}
  {{- printf "%s-portal" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.core" -}}
  {{- printf "%s-core" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.valkey" -}}
  {{- printf "%s-valkey" .Release.Name -}}
{{- end -}}

{{- define "harbor.registry" -}}
  {{- printf "%s-registry" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.registryCtl" -}}
  {{- printf "%s-registryctl" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.database" -}}
  {{- printf "%s-database" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.trivy" -}}
  {{- printf "%s-trivy" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.nginx" -}}
  {{- printf "%s-nginx" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.exporter" -}}
  {{- printf "%s-exporter" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.ingress" -}}
  {{- printf "%s-ingress" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.ingress.secret" -}}
  {{- printf "harbor-tls" -}}
{{- end -}}


{{- define "harbor.route" -}}
  {{- printf "%s-route" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.noProxy" -}}
  {{- printf "%s,%s,%s,%s,%s,%s,%s,%s" (include "harbor.core" .) (include "harbor.jobservice" .) (include "harbor.database" .) (include "harbor.registry" .) (include "harbor.portal" .) (include "harbor.trivy" .) (include "harbor.exporter" .) .Values.proxy.noProxy -}}
{{- end -}}


{{/*
=============================================================================
Trace helpers
=============================================================================
*/}}


{{- define "harbor.trace.envs" -}}
  TRACE_ENABLED: "{{ .Values.trace.enabled }}"
  TRACE_SAMPLE_RATE: "{{ .Values.trace.sample_rate }}"
  TRACE_NAMESPACE: "{{ .Values.trace.namespace }}"
  {{- if .Values.trace.attributes }}
  TRACE_ATTRIBUTES: {{ .Values.trace.attributes | toJson | squote }}
  {{- end }}
  {{- if eq .Values.trace.provider "jaeger" }}
  TRACE_JAEGER_ENDPOINT: "{{ .Values.trace.jaeger.endpoint }}"
  TRACE_JAEGER_USERNAME: "{{ .Values.trace.jaeger.username }}"
  TRACE_JAEGER_AGENT_HOSTNAME: "{{ .Values.trace.jaeger.agent_host }}"
  TRACE_JAEGER_AGENT_PORT: "{{ .Values.trace.jaeger.agent_port }}"
  {{- else }}
  TRACE_OTEL_ENDPOINT: "{{ .Values.trace.otel.endpoint }}"
  TRACE_OTEL_URL_PATH: "{{ .Values.trace.otel.url_path }}"
  TRACE_OTEL_COMPRESSION: "{{ .Values.trace.otel.compression }}"
  TRACE_OTEL_INSECURE: "{{ .Values.trace.otel.insecure }}"
  TRACE_OTEL_TIMEOUT: "{{ .Values.trace.otel.timeout }}"
  {{- end }}
{{- end -}}

{{- define "harbor.trace.envs.core" -}}
  {{- if .Values.trace.enabled }}
  TRACE_SERVICE_NAME: "harbor-core"
  {{ include "harbor.trace.envs" . }}
  {{- end }}
{{- end -}}

{{- define "harbor.trace.envs.jobservice" -}}
  {{- if .Values.trace.enabled }}
  TRACE_SERVICE_NAME: "harbor-jobservice"
  {{ include "harbor.trace.envs" . }}
  {{- end }}
{{- end -}}

{{- define "harbor.trace.envs.registryctl" -}}
  {{- if .Values.trace.enabled }}
  TRACE_SERVICE_NAME: "harbor-registryctl"
  {{ include "harbor.trace.envs" . }}
  {{- end }}
{{- end -}}

{{- define "harbor.trace.jaeger.password" -}}
  {{- if and .Values.trace.enabled (eq .Values.trace.provider "jaeger") }}
  TRACE_JAEGER_PASSWORD: "{{ .Values.trace.jaeger.password | default "" | b64enc }}"
  {{- end }}
{{- end -}}


{{- define "imagePullSecret" }}
{{- printf "{\"auths\":{\"%s\":{\"auth\":\"%s\"}}}" .Values.imageCredentials.registry (printf "%s:%s" .Values.imageCredentials.username .Values.imageCredentials.password | b64enc) | b64enc }}
{{- end }}

