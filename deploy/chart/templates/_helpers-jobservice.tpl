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

{{- define "harbor.jobservice.reaper.max_update_hours" -}}
{{ .Values.jobservice.reaper.max_update_hours | default 24 }}
{{- end }}

{{- define "harbor.jobservice.reaper.max_dangling_hours" -}}
{{ .Values.jobservice.reaper.max_dangling_hours | default 168 }}
{{- end }}

{{- define "harbor.jobservice.notification.webhook_job_max_retry" -}}
{{ .Values.jobservice.notification.webhook_job_max_retry | default 3 }}
{{- end }}

{{- define "harbor.jobservice.notification.webhook_job_http_client_timeout" -}}
{{ .Values.jobservice.notification.webhook_job_http_client_timeout | default 3 }}
{{- end }}

{{- define "harbor.jobservice.secretName" -}}
  {{- if eq .Values.tls.certSource "secret" -}}
    {{- .Values.jobservice.secretName -}}
  {{- else -}}
    {{- printf "%s-jobservice-internal-tls" (include "harbor.fullname" .) -}}
  {{- end -}}
{{- end -}}
