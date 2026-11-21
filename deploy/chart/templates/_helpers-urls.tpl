{{/*
=============================================================================
Internal URL helpers
=============================================================================
*/}}

{{/*
Return the Core internal URL. The port stays explicit: go-containerregistry
reads a portless single-label host as a Docker Hub namespace (#790).
*/}}
{{- define "harbor.core.url" -}}
http://{{ include "harbor.fullname" . }}-core:{{ include "harbor.core.service.port" . }}
{{- end }}

{{/*
Container port
*/}}
{{- define "harbor.core.port" -}}
8080
{{- end }}

{{/*
Core Service port (core.servicePort)
*/}}
{{- define "harbor.core.service.port" -}}
{{- .Values.core.servicePort | default 80 -}}
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
  {{- printf "%s,%s,%s,%s,%s,%s,%s%s,%s" (include "harbor.core" .) (include "harbor.jobservice" .) (include "harbor.database" .) (include "harbor.registry" .) (include "harbor.portal" .) (include "harbor.trivy" .) (include "harbor.exporter" .) (include "harbor.scannerRegistration.noProxy" .) .Values.proxy.noProxy -}}
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

{{/*
=============================================================================
Scanner registration helpers
=============================================================================
*/}}

{{/*
Scanner adapters the registration Job reconciles, as YAML under `items`.
Any `harbor-scanner-*` subchart that ships a `registration` block is picked up,
so a new scanner subchart needs no template change here. `present` marks a
scanner to register, `stale` a disabled subchart whose registration the Job
removes; `registration.enabled: false` sets neither. `adopt` allows the rename
lookup by URL. Removal and adoption only apply to the chart-derived Service URL. The Service name mirrors
the subcharts' fullname helpers.
*/}}
{{- define "harbor.scannerRegistrations" -}}
{{- $root := . -}}
{{- $items := list -}}
{{- $selectors := dict (include "harbor.name" .) "the harbor chart" -}}
{{- range $key := keys .Values | sortAlpha -}}
{{- $sub := index $root.Values $key -}}
{{- if and (hasPrefix "harbor-scanner-" $key) (kindIs "map" $sub) -}}
{{- $reg := $sub.registration -}}
{{- if kindIs "map" $reg -}}
{{- $name := default $key $sub.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- $room := int (sub 62 (len $name)) -}}
{{- $release := $root.Release.Name -}}
{{- if gt (len $release) $room -}}
{{- $release = printf "%s-%s" ($release | trunc (int (max 0 (sub $room 9))) | trimSuffix "-") (printf "h%s" (sha256sum $release | trunc 7)) -}}
{{- end -}}
{{- $fullname := printf "%s-%s" $release $name | trimPrefix "-" -}}
{{- $fullname = default $fullname $sub.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- $url := default (printf "http://%s:%v" $fullname (($sub.service).port | default 8080)) $reg.url -}}
{{- $managed := ne (toString $reg.enabled) "false" -}}
{{- $deployed := eq (toString $sub.enabled) "true" -}}
{{- if $deployed -}}
{{- if hasKey $selectors $name -}}
{{- fail (printf "%s and %s share the name %q, so their Services would select each other's pods: give the scanner a different nameOverride" (get $selectors $name) $key $name) -}}
{{- end -}}
{{- $_ := set $selectors $name $key -}}
{{- end -}}
{{- $items = append $items (dict "name" ($reg.name | default "" | toString) "description" ($reg.description | default "") "url" $url "default" (eq (toString $reg.default) "true") "useInternalAddr" (ne (toString $reg.useInternalAddr) "false") "skipCertVerify" (eq (toString $reg.skipCertVerify) "true") "present" (and $deployed $managed) "stale" (and (not $deployed) $managed (empty $reg.url) (not (empty $reg.name))) "deployed" $deployed "adopt" (empty $reg.url) "host" $fullname) -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- /* --reuse-values from a release before this block existed has no scannerRegistration key */ -}}
{{- range (.Values.scannerRegistration).extra -}}
{{- $items = append $items (dict "name" .name "description" (.description | default "") "url" .url "default" (eq (toString .default) "true") "useInternalAddr" (eq (toString .useInternalAddr) "true") "skipCertVerify" (eq (toString .skipCertVerify) "true") "present" true "stale" false "deployed" false "adopt" false "host" "") -}}
{{- end -}}
{{- /* Harbor stores adapter URLs with a lowercase host and no trailing slash; match that so lookups by URL hit. */ -}}
{{- $jobOn := ne (toString (.Values.scannerRegistration).enabled) "false" -}}
{{- range $items -}}
{{- if and $jobOn (or .present .stale) -}}
{{- $u := urlParse (toString .url) -}}
{{- $rest := list (ternary "" (printf "?%s" $u.query) (empty $u.query)) (ternary "" (printf "#%s" $u.fragment) (empty $u.fragment)) | join "" -}}
{{- $_ := set . "url" (printf "%s://%s%s%s%s" (lower $u.scheme) (ternary "" (printf "%s@" $u.userinfo) (empty $u.userinfo)) (lower $u.host) (regexReplaceAll "/+$" $u.path "") $rest) -}}
{{- end -}}
{{- end -}}
{{- toYaml (dict "items" $items) -}}
{{- end -}}

{{/* "true" when the registration Job has at least one scanner to register. */}}
{{- define "harbor.scannerRegistration.enabled" -}}
{{- $active := false -}}
{{- range (fromYaml (include "harbor.scannerRegistrations" .)).items -}}
{{- if .present }}{{ $active = true }}{{ end -}}
{{- end -}}
{{- $sr := .Values.scannerRegistration | default dict -}}
{{- $missing := not (and ($sr.image).repository (or ($sr.image).tag ($sr.image).digest)) -}}
{{- range list "enabled" "username" "existingSecretPasswordKey" "backoffLimit" "activeDeadlineSeconds" "podSecurityContext" "securityContext" "resources" -}}
{{- if not (hasKey $sr .) }}{{ $missing = true }}{{ end -}}
{{- end -}}
{{- if and $active (ne (toString $sr.enabled) "false") $missing -}}
{{- fail "A scanner is enabled but the scannerRegistration defaults are missing, which happens with --reuse-values from a chart that predates them. Pass your values file (helm upgrade -f values.yaml) or use --reset-then-reuse-values." -}}
{{- end -}}
{{- and (eq (toString (.Values.scannerRegistration).enabled) "true") $active -}}
{{- end -}}

{{/*
Comma-prefixed Service names of deployed scanner subcharts, short and
namespace-qualified, for noProxy. Empty
without a proxy, so a scanner toggle does not roll core and jobservice.
*/}}
{{- define "harbor.scannerRegistration.noProxy" -}}
{{- if or .Values.proxy.httpProxy .Values.proxy.httpsProxy -}}
{{- range (fromYaml (include "harbor.scannerRegistrations" .)).items -}}
{{- if and .deployed .host -}}
,{{ .host }},{{ .host }}.{{ $.Release.Namespace }},{{ .host }}.{{ $.Release.Namespace }}.svc
{{- /* A registration.url that spells out the Service's cluster domain is bypassed as written. */ -}}
{{- $h := regexReplaceAll "^[a-z]+://([^/:?#]+).*$" (lower (toString .url)) "${1}" -}}
{{- if hasPrefix (printf "%s.%s.svc." .host $.Release.Namespace | lower) $h }},{{ $h }}{{ end -}}
{{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
