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
