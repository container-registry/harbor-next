{{/*
Expand the name of the chart.
*/}}
{{- define "harbor.dnsSafeName" -}}
{{- regexReplaceAll "-+" (regexReplaceAll "[^a-z0-9-]+" (lower .) "-") "-" | trimAll "-" -}}
{{- end }}

{{- define "harbor.name" -}}
{{- include "harbor.dnsSafeName" (default .Chart.Name .Values.nameOverride) | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "harbor.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- include "harbor.dnsSafeName" .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := include "harbor.name" . }}
{{- if contains $name (lower .Release.Name) }}
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
{{- printf "%s-%s" (include "harbor.name" .) .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels

commonLabels are appended AFTER selectorLabels so they land on metadata.labels
but never on selectors (harbor.selectorLabels is the only thing selectors use).
*/}}
{{- define "harbor.labels" -}}
helm.sh/chart: {{ include "harbor.chart" . }}
{{ include "harbor.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Merged annotations: commonAnnotations plus an optional local map (local wins on
key collision). A fresh (dict) is the merge target so neither source map is mutated.
Emits nothing when both are empty, so callers can guard with `with`.
Usage: {{ include "harbor.annotations" (dict "root" . "local" .Values.core.annotations) }}
*/}}
{{- define "harbor.annotations" -}}
{{- $common := .root.Values.commonAnnotations | default dict -}}
{{- with merge (dict) (.local | default dict) $common }}
{{- toYaml . }}
{{- end }}
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
Pod scheduling helpers
=============================================================================
*/}}

{{/*
Pod scheduling block (topologySpreadConstraints, nodeSelector, affinity, tolerations).
Usage: {{ include "harbor.podScheduling" (dict "component" .Values.core "root" $) }}
*/}}
{{- define "harbor.podScheduling" -}}
{{- with .component.topologySpreadConstraints }}
topologySpreadConstraints:
  {{- tpl (toYaml .) $.root | nindent 2 }}
{{- end }}
{{- with .component.nodeSelector }}
nodeSelector:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .component.affinity }}
affinity:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .component.tolerations }}
tolerations:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with .component.hostAliases }}
hostAliases:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
=============================================================================
Probe helpers
=============================================================================
*/}}

{{/*
Render startup/liveness/readiness probes from a component's `probes` block.
Each probe is a full Kubernetes probe spec rendered verbatim; a probe set
to null (or an absent probes block) is simply omitted.
Usage: {{ include "harbor.probes" .Values.core.probes | nindent 10 }}
*/}}
{{- define "harbor.probes" -}}
{{- $probes := . | default dict }}
{{- with $probes.startup }}
startupProbe:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with $probes.liveness }}
livenessProbe:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- with $probes.readiness }}
readinessProbe:
  {{- toYaml . | nindent 2 }}
{{- end }}
{{- end }}

{{/*
=============================================================================
Registry helpers
=============================================================================
*/}}

{{/*
Chart-managed blocks merged on top of .Values.registry.config when
rendering /etc/registry/config.yml. Sensitive values (redis.password,
http.secret) come via env-var overrides on the Deployment, not via this
ConfigMap-rendered block. User's `registry.config` keys win on collision
(see registry.configmap.yaml mustMergeOverwrite call).

The mount path under storage.filesystem is intentionally NOT set here —
it's part of the user's config (or default) and the Deployment derives
the volumeMount path from .Values.registry.config.storage.filesystem.rootdirectory.
*/}}
{{- define "harbor.registry.chartManagedConfig" -}}
log:
  {{- if eq .Values.logLevel "warning" }}
  level: warn
  {{- else if eq .Values.logLevel "fatal" }}
  level: error
  {{- else }}
  level: {{ .Values.logLevel }}
  {{- end }}
redis:
  addrs:
    - {{ include "harbor.redis.hostWithPort" . | quote }}
  {{- if contains "sentinel" (include "harbor.redis.scheme" .) }}
  sentinelMasterSet: {{ include "harbor.redis.masterSet" . | quote }}
  {{- end }}
  db: {{ include "harbor.redis.url.registry.num" . | int }}
  readtimeout: 10s
  writetimeout: 10s
  dialtimeout: 10s
  enableTLS: {{ eq (include "harbor.redis.enableTLS" . | trim) "true" }}
  pool:
    maxidle: 100
    maxactive: 500
    idletimeout: 60s
http:
  addr: ":5000"
  debug:
    {{- if .Values.metrics.enabled }}
    addr: ":{{ include "harbor.metrics.port" . }}"
    prometheus:
      enabled: true
      path: {{ include "harbor.metrics.path" . | quote }}
    {{- else }}
    addr: "localhost:5001"
    {{- end }}
{{- end }}

{{/*
Filesystem rootdirectory honored by the storage volumeMount on the
registry/registryctl containers. Reads from .Values.registry.config.storage.filesystem.rootdirectory
with fallback to /storage.
*/}}
{{- define "harbor.registry.storageMountPath" -}}
{{- $storage := dig "storage" "filesystem" "rootdirectory" "/storage" (.Values.registry.config | default dict) -}}
{{- $storage -}}
{{- end -}}

{{/*
Chart-managed blocks merged on top of .Values.jobservice.config when
rendering /etc/jobservice/config.yml. Sensitive values (redis URL with
auth) come via env-var overrides on the Deployment.
User's `jobservice.config` keys win on collision.
*/}}
{{- define "harbor.jobservice.chartManagedConfig" -}}
protocol: {{ include "harbor.component.scheme" . | quote }}
port: {{ include "harbor.jobservice.port" . }}
worker_pool:
  backend: "redis"
  redis_pool:
    redis_url: {{ include "harbor.redis.url.jobservice" . | quote }}
    namespace: "harbor_job_service_namespace"
    idle_timeout_second: 3600
{{- if .Values.metrics.enabled }}
metric:
  enabled: true
  path: {{ include "harbor.metrics.path" . | quote }}
  port: {{ include "harbor.metrics.port" . | int }}
{{- end }}
{{- end }}

{{/*
Detect the registry storage provider from the user's registry.config.storage
block. Harbor Core needs this as REGISTRY_STORAGE_PROVIDER_NAME to compute
redirects and presigned URLs. A driver is any storage key that is not one of
distribution's meta sections (cache/delete/maintenance/redirect/tag), so new
distribution drivers are picked up without chart changes. Defaults to
"filesystem" when no driver is set (mirrors registry.configmap.yaml).
*/}}
{{- define "harbor.registry.storageType" -}}
{{- $storage := dig "storage" (dict) (.Values.registry.config | default dict) -}}
{{- $driver := "filesystem" -}}
{{- range $k, $v := $storage -}}
{{- if and (not (has $k (list "cache" "delete" "maintenance" "redirect" "tag"))) (not (kindIs "invalid" $v)) -}}
{{- $driver = $k -}}
{{- end -}}
{{- end -}}
{{- $driver -}}
{{- end -}}

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
Generate a random secret value, or fail when auto-generation is disabled.

GitOps engines that render client-side (Argo CD) re-template on every sync;
`lookup` returns nothing there, so an auto-generated value would rotate on
each sync and roll every workload via the checksum annotations. With
`autoGenSecrets: false` the chart refuses to generate and names the value
to pin instead.

Expects a dict: "root" ($), "len" (int), "hint" (the values to set).
*/}}
{{- define "harbor.autoGenValue" -}}
{{- if .root.Values.autoGenSecrets -}}
{{- randAlphaNum (.len | int) -}}
{{- else -}}
{{- fail (printf "autoGenSecrets is false: set %s to a fixed value" .hint) -}}
{{- end -}}
{{- end -}}

{{/*
=============================================================================
Image helpers
=============================================================================
*/}}

{{/*
Per-source image defaults. `image.source` (8gcr | upstream) picks a registry and
the per-component repository path. Upstream goharbor renames two images
(`registry-photon`, `trivy-adapter-photon`), so this is a real map, not a host
swap — keep it in sync with goharbor/harbor-helm on appVersion bumps.
*/}}
{{- define "harbor.image.sourceMap" -}}
8gcr:
  registry: 8gears.container-registry.com
  repos:
    core: 8gcr/harbor-core
    jobservice: 8gcr/harbor-jobservice
    registry: 8gcr/harbor-registry
    registryctl: 8gcr/harbor-registryctl
    portal: 8gcr/harbor-portal
    trivy: 8gcr/trivy-adapter
    exporter: 8gcr/harbor-exporter
upstream:
  registry: docker.io
  repos:
    core: goharbor/harbor-core
    jobservice: goharbor/harbor-jobservice
    registry: goharbor/registry-photon
    registryctl: goharbor/harbor-registryctl
    portal: goharbor/harbor-portal
    trivy: goharbor/trivy-adapter-photon
    exporter: goharbor/harbor-exporter
{{- end -}}

{{/*
Return the fully-qualified image reference for a component.
Usage: {{ include "harbor.image" (dict "imageRoot" .Values.core.image "component" "core" "root" .) }}

Resolution (per-component overrides win, except global.imageRegistry which wins
over everything so an air-gapped mirror can be forced in one place):
  registry   = global.imageRegistry | imageRoot.registry | sourceMap[source].registry
  repository = imageRoot.repository | sourceMap[source].repos[component]
  registry   = global.imageRegistry | imageRoot.registry
               | (sourceMap[source].registry IF repository has no host)
  ref        = registry ? "{registry}/{repository}" : repository
  digest set -> "{ref}@{digest}" ; else "{ref}:{tag | AppVersion}"

Back-compat: when `repository` already carries a registry host (its first
path segment contains a "." or ":") and no explicit registry override is given,
it is used verbatim — so a full-path `repository` (the legacy / upstream-chart
style, e.g. ttl.sh/foo/harbor-core) is NOT double-prefixed by the source map.
*/}}
{{- define "harbor.image" -}}
{{- $root := .root -}}
{{- $img := .imageRoot | default dict -}}
{{- $source := $root.Values.image.source | default "8gcr" -}}
{{- $cfg := index (fromYaml (include "harbor.image.sourceMap" .)) $source -}}
{{- $repository := $img.repository | default (index $cfg.repos .component) -}}
{{- $firstSeg := splitList "/" $repository | first -}}
{{- $repoHasHost := and (contains "/" $repository) (or (contains "." $firstSeg) (contains ":" $firstSeg)) -}}
{{- $registry := "" -}}
{{- if $root.Values.global.imageRegistry -}}
{{-   $registry = $root.Values.global.imageRegistry -}}
{{- else if $img.registry -}}
{{-   $registry = $img.registry -}}
{{- else if not $repoHasHost -}}
{{-   $registry = $cfg.registry -}}
{{- end -}}
{{- $ref := $repository -}}
{{- if $registry -}}{{- $ref = printf "%s/%s" $registry $repository -}}{{- end -}}
{{- if $img.digest -}}
{{- printf "%s@%s" $ref $img.digest -}}
{{- else -}}
{{- $tag := $img.tag | default $root.Chart.AppVersion -}}
{{- printf "%s:%s" $ref $tag -}}
{{- end -}}
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
Return the secret key for encryption.

Resolution order:
  1. Explicit `.Values.secretKey` if set.
  2. The `SECRET_KEY` value persisted in the existing core Secret on
     upgrade (looked up at template time), so upgrades reuse the
     original key.
  3. A fresh 16-char alphanumeric on first install. Random, not
     derivable from release name.
*/}}
{{- define "harbor.secretKey" -}}
{{- if .Values.secretKey }}
{{- .Values.secretKey }}
{{- else }}
{{- $existing := (lookup "v1" "Secret" .Release.Namespace (include "harbor.core" .)) | default dict }}
{{- $existingKey := index ($existing.data | default dict) "SECRET_KEY" }}
{{- if $existingKey }}
{{- $existingKey | b64dec }}
{{- else }}
{{- include "harbor.autoGenValue" (dict "root" . "len" 16 "hint" "secretKey or existingSecretSecretKey") }}
{{- end }}
{{- end }}
{{- end }}

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
{{- if and (not .Values.harborAdminPassword) (not .Values.existingSecretAdminPassword) }}
{{- fail "harborAdminPassword or existingSecretAdminPassword is required. Please set one in your values." }}
{{- end }}
{{- $enabledExposeMethods := 0 }}
{{- if .Values.ingress.enabled }}
{{- $enabledExposeMethods = add1 $enabledExposeMethods }}
{{- end }}
{{- if .Values.gateway.enabled }}
{{- $enabledExposeMethods = add1 $enabledExposeMethods }}
{{- end }}
{{- if .Values.expose.clusterIP.enabled }}
{{- $enabledExposeMethods = add1 $enabledExposeMethods }}
{{- end }}
{{- if .Values.expose.nodePort.enabled }}
{{- $enabledExposeMethods = add1 $enabledExposeMethods }}
{{- end }}
{{- if .Values.expose.loadBalancer.enabled }}
{{- $enabledExposeMethods = add1 $enabledExposeMethods }}
{{- end }}
{{- if .Values.expose.route.enabled }}
{{- $enabledExposeMethods = add1 $enabledExposeMethods }}
{{- end }}
{{- if gt $enabledExposeMethods 1 }}
{{- fail "Only one expose method can be enabled at a time (ingress, gateway, expose.clusterIP, expose.nodePort, expose.loadBalancer, expose.route)." }}
{{- end }}
{{- if and .Values.metrics.serviceMonitor.enabled (not .Values.metrics.enabled) }}
{{- fail "metrics.serviceMonitor.enabled requires metrics.enabled=true. Without metrics enabled, Harbor pods do not expose the /metrics endpoint the ServiceMonitor would scrape." }}
{{- end }}
{{- /* HPA min/max sanity — fail fast at template time rather than letting K8s reject. */}}
{{- range $name, $cfg := dict "core" .Values.core "registry" .Values.registry "jobservice" .Values.jobservice "portal" .Values.portal "trivy" .Values.trivy }}
{{- if and $cfg.autoscaling $cfg.autoscaling.enabled }}
{{- if not $cfg.autoscaling.maxReplicas }}
{{- fail (printf "%s.autoscaling.enabled=true requires %s.autoscaling.maxReplicas to be set." $name $name) }}
{{- end }}
{{- if gt (int ($cfg.autoscaling.minReplicas | default 1)) (int $cfg.autoscaling.maxReplicas) }}
{{- fail (printf "%s.autoscaling.minReplicas (%v) must be <= %s.autoscaling.maxReplicas (%v)." $name $cfg.autoscaling.minReplicas $name $cfg.autoscaling.maxReplicas) }}
{{- end }}
{{- end }}
{{- end }}
{{- end }}


{{- define "imagePullSecret" }}
{{- printf "{\"auths\":{\"%s\":{\"auth\":\"%s\"}}}" .Values.imageCredentials.registry (printf "%s:%s" .Values.imageCredentials.username .Values.imageCredentials.password | b64enc) | b64enc }}
{{- end }}
