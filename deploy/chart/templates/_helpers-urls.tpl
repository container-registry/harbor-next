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
Component name helpers (used by noProxy and other cross-component references)
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

{{- define "harbor.ingress.primaryHost" -}}
{{- if gt (len .Values.ingress.hosts) 0 -}}
{{- (index .Values.ingress.hosts 0).host -}}
{{- else -}}
{{- (urlParse .Values.externalURL).hostname -}}
{{- end -}}
{{- end -}}

{{- define "harbor.route" -}}
  {{- printf "%s-route" (include "harbor.fullname" .) -}}
{{- end -}}

{{- define "harbor.noProxy" -}}
  {{- printf "%s,%s,%s,%s,%s,%s,%s,%s" (include "harbor.core" .) (include "harbor.jobservice" .) (include "harbor.database" .) (include "harbor.registry" .) (include "harbor.portal" .) (include "harbor.trivy" .) (include "harbor.exporter" .) .Values.proxy.noProxy -}}
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
