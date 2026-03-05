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
{{- end }}


{{- define "imagePullSecret" }}
{{- printf "{\"auths\":{\"%s\":{\"auth\":\"%s\"}}}" .Values.imageCredentials.registry (printf "%s:%s" .Values.imageCredentials.username .Values.imageCredentials.password | b64enc) | b64enc }}
{{- end }}
