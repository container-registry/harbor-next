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

{{- define "harbor.jobservice.secretName" -}}
  {{- if eq .Values.tls.certSource "secret" -}}
    {{- .Values.jobservice.secretName -}}
  {{- else -}}
    {{- printf "%s-jobservice-internal-tls" (include "harbor.fullname" .) -}}
  {{- end -}}
{{- end -}}
