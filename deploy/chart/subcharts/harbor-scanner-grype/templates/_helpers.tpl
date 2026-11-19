{{- define "harbor-scanner-grype.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/* Helm shares template names across charts, so helpers carry the chart name. The parent chart mirrors fullname to derive the adapter URL. */}}
{{- define "harbor-scanner-grype.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "harbor-scanner-grype.name" . -}}
{{- /* Long release names lose their tail, not the scanner name, and gain a letter-first hash so two releases stay apart. */ -}}
{{- $room := int (sub 62 (len $name)) -}}
{{- $release := .Release.Name -}}
{{- if gt (len $release) $room -}}
{{- $release = printf "%s-%s" ($release | trunc (int (max 0 (sub $room 9))) | trimSuffix "-") (printf "h%s" (sha256sum $release | trunc 7)) -}}
{{- end -}}
{{- printf "%s-%s" $release $name | trimPrefix "-" | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}

{{- define "harbor-scanner-grype.selectorLabels" -}}
app.kubernetes.io/name: {{ include "harbor-scanner-grype.name" . | quote }}
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}

{{- define "harbor-scanner-grype.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "harbor-scanner-grype.selectorLabels" . }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/component: scanner
{{- range $k, $v := omit .Values.commonLabels "helm.sh/chart" "app.kubernetes.io/name" "app.kubernetes.io/instance" "app.kubernetes.io/version" "app.kubernetes.io/managed-by" "app.kubernetes.io/component" }}
{{ $k | quote }}: {{ toString $v | quote }}
{{- end }}
{{- end -}}

{{- define "harbor-scanner-grype.image" -}}
{{- $registry := (.Values.global).imageRegistry | default .Values.image.registry -}}
{{- $ref := ternary (printf "%s/%s" $registry .Values.image.repository) .Values.image.repository (ne $registry "") -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $ref .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $ref (.Values.image.tag | default "latest") -}}
{{- end -}}
{{- end -}}
