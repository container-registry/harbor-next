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

{{- define "harbor.middleware.enabled" -}}
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
