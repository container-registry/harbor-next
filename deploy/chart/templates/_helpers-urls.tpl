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
{{- if .Values.portal.url -}}
{{- .Values.portal.url -}}
{{- else -}}
http://{{ include "harbor.fullname" . }}-portal
{{- end -}}
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
Trivy runs in one of two mutually exclusive modes:
  - built-in (2.16 default): this chart's own trivy.* templates, gated on
    trivy.enabled
  - subchart: the harbor-scanner-trivy dependency, gated on
    harbor-scanner-trivy.enabled (the Chart.yaml condition)
The subchart wins when both flags are set — the built-in templates disable
themselves so the two modes can never deploy side by side.
TODO(2.17): drop the built-in templates and make the subchart the only mode.
*/}}
{{- define "harbor.trivy.subchart" -}}
{{- if (index .Values "harbor-scanner-trivy").enabled }}true{{- else }}false{{- end }}
{{- end }}

{{- define "harbor.trivy.builtin" -}}
{{- if and .Values.trivy.enabled (not (index .Values "harbor-scanner-trivy").enabled) }}true{{- else }}false{{- end }}
{{- end }}

{{/*
Return the Trivy adapter URL (if enabled)
*/}}
{{- define "harbor.trivy.url" -}}
{{- if eq (include "harbor.trivy.subchart" .) "true" -}}
http://{{ include "harbor.trivy" . }}:{{ (((index .Values "harbor-scanner-trivy").service).port) | default 8080 }}
{{- else -}}
http://{{ include "harbor.trivy" . }}:8080
{{- end -}}
{{- end }}

{{- define "harbor.trivy.enabled" -}}
{{ or .Values.trivy.enabled (index .Values "harbor-scanner-trivy").enabled }}
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

{{/*
In subchart mode the Trivy resource names come from harbor-scanner-trivy's
own fullname helper (its .Chart.Name plus any nameOverride/fullnameOverride
set under the harbor-scanner-trivy key). Replicate that helper exactly so
core's TRIVY_ADAPTER_URL and noProxy stay correct: keep in sync with
harbor-scanner-trivy's _helpers.tpl on dependency bumps. The shipped
default (nameOverride: trivy) names resources `<release>-trivy`.
*/}}
{{- define "harbor.trivy" -}}
  {{- if eq (include "harbor.trivy.subchart" .) "true" -}}
    {{- $sub := index .Values "harbor-scanner-trivy" -}}
    {{- if $sub.fullnameOverride -}}
      {{- $sub.fullnameOverride | trunc 63 | trimSuffix "-" -}}
    {{- else -}}
      {{- $name := default "harbor-scanner-trivy" $sub.nameOverride -}}
      {{- if contains $name .Release.Name -}}
        {{- .Release.Name | trunc 63 | trimSuffix "-" -}}
      {{- else -}}
        {{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
      {{- end -}}
    {{- end -}}
  {{- else -}}
    {{- printf "%s-trivy" (include "harbor.fullname" .) -}}
  {{- end -}}
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
{{- printf "%s-ingress-tls" (include "harbor.fullname" .) | trunc 63 | trimSuffix "-" -}}
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
