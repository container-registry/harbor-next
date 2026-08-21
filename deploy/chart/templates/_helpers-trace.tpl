{{/*
=============================================================================
Trace helpers
=============================================================================
*/}}

{{- define "harbor.trace.envs" -}}
  {{- /* Only emit keys whose source values are set. An unset .Values.*
         field renders as the literal string `<no value>` inside quotes,
         which then becomes runtime config and confuses Harbor. */ -}}
  TRACE_ENABLED: "{{ .Values.trace.enabled }}"
  {{- /* Use hasKey so an explicit `sample_rate: 0` (sample nothing) is preserved
         instead of being clobbered by sprig `default` (which treats 0 as empty). */}}
  TRACE_SAMPLE_RATE: "{{ if hasKey .Values.trace "sample_rate" }}{{ .Values.trace.sample_rate }}{{ else }}1{{ end }}"
  {{- with .Values.trace.namespace }}
  TRACE_NAMESPACE: "{{ . }}"
  {{- end }}
  {{- if .Values.trace.attributes }}
  TRACE_ATTRIBUTES: {{ .Values.trace.attributes | toJson | squote }}
  {{- end }}
  {{- if eq .Values.trace.provider "jaeger" }}
  {{- with .Values.trace.jaeger.endpoint }}
  TRACE_JAEGER_ENDPOINT: "{{ . }}"
  {{- end }}
  {{- with .Values.trace.jaeger.username }}
  TRACE_JAEGER_USERNAME: "{{ . }}"
  {{- end }}
  {{- with .Values.trace.jaeger.agent_host }}
  TRACE_JAEGER_AGENT_HOSTNAME: "{{ . }}"
  {{- end }}
  {{- with .Values.trace.jaeger.agent_port }}
  TRACE_JAEGER_AGENT_PORT: "{{ . }}"
  {{- end }}
  {{- else }}
  {{- with .Values.trace.otel.endpoint }}
  TRACE_OTEL_ENDPOINT: "{{ . }}"
  {{- end }}
  {{- with .Values.trace.otel.url_path }}
  TRACE_OTEL_URL_PATH: "{{ . }}"
  {{- end }}
  {{- with .Values.trace.otel.compression }}
  TRACE_OTEL_COMPRESSION: "{{ . }}"
  {{- end }}
  {{- with .Values.trace.otel.insecure }}
  TRACE_OTEL_INSECURE: "{{ . }}"
  {{- end }}
  {{- with .Values.trace.otel.timeout }}
  TRACE_OTEL_TIMEOUT: "{{ . }}"
  {{- end }}
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
