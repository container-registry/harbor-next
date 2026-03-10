{{/*
=============================================================================
Redis helpers
=============================================================================
*/}}

{{/*
Return the Redis host
*/}}
{{- define "harbor.redis.host" -}}
{{- if .Values.valkey.enabled }}
{{- .Values.valkey.fullnameOverride | default (printf "%s-valkey" .Release.Name) }}
{{- else }}
{{- .Values.externalRedis.host }}
{{- end }}
{{- end -}}

{{/*
Return the Redis host with port
*/}}
{{- define "harbor.redis.hostWithPort" -}}
{{- include "harbor.redis.host" . }}:{{ include "harbor.redis.port" . }}
{{- end }}

{{/*
Return the Redis port
*/}}
{{- define "harbor.redis.port" -}}
{{- if .Values.valkey.enabled }}
{{- 6379 }}
{{- else }}
{{- .Values.externalRedis.port | default 6379 }}
{{- end }}
{{- end }}

{{/*
Return the Redis password secret name
*/}}
{{- define "harbor.redis.secretName" -}}
{{- if .Values.valkey.enabled }}
{{- .Release.Name }}-valkey
{{- else if .Values.externalRedis.existingSecret }}
{{- .Values.externalRedis.existingSecret }}
{{- else }}
{{- include "harbor.fullname" . }}-redis
{{- end }}
{{- end }}

{{/*
Return the Redis password key in the secret
*/}}
{{- define "harbor.redis.secretKey" -}}
{{- if .Values.valkey.auth.enabled -}}
valkey-password
{{- else -}}
REDIS_PASSWORD
{{- end -}}
{{- end }}

{{/*
Return the Base Redis URL for Harbor components
*/}}
{{- define "harbor.redis.url" -}}
  {{- $root := . -}}
  {{- $host := include "harbor.redis.host" $root -}}
  {{- $port := include "harbor.redis.port" $root -}}
  {{- if .Values.valkey.auth.enabled -}}
    {{- printf "redis://:$(REDIS_PASSWORD)@%s:%s" $host $port -}}
  {{- else -}}
    {{- printf "redis://%s:%s" $host $port -}}
  {{- end -}}
{{- end }}

{{/*
Return the Redis URL for Harbor core
*/}}
{{- define "harbor.redis.url.core" -}}
  {{ include "harbor.redis.url" . }}/0?idle_timeout_seconds=30
{{- end -}}

{{- define "harbor.redis.masterSet" -}}
{{- .Values.externalRedis.sentinelMasterSet -}}
{{- end -}}

{{- define "harbor.redis.scheme" -}}
  {{- if .Values.valkey.enabled -}}
    {{- print "redis" -}}
  {{- else -}}
    {{- if .Values.externalRedis.sentinelMasterSet -}}
      {{- ternary "rediss+sentinel" "redis+sentinel" .Values.externalRedis.tlsOptions.enable -}}
    {{- else -}}
      {{- ternary "rediss" "redis" .Values.externalRedis.tlsOptions.enable -}}
    {{- end -}}
  {{- end -}}
{{- end -}}

{{/* scheme://[:password@]addr/db_index?idle_timeout_seconds=30 */}}
{{- define "harbor.redis.url.harbor" -}}
    {{ include "harbor.redis.url" . }}/6?idle_timeout_seconds=30
{{- end -}}

{{- define "harbor.redis.url.registry.num" -}}
2
{{- end -}}

{{- define "harbor.redis.url.registry" -}}
  {{ include "harbor.redis.url" . }}/{{ include "harbor.redis.url.registry.num" . }}?idle_timeout_seconds=30
{{- end -}}

{{- define "harbor.redis.url.jobservice" -}}
  {{ include "harbor.redis.url" . }}/1
{{- end -}}

{{- define "harbor.redis.url.trivy" -}}
  {{ include "harbor.redis.url" . }}/5
{{- end -}}

{{- define "harbor.redis.url.cache" -}}
  {{- $url := include "harbor.redis.url" . -}}
  {{- printf "%s/7?idle_timeout_seconds=30" $url -}}
{{- end -}}

{{- define "harbor.redis.password" -}}
  {{- ternary "" .Values.externalRedis.password (.Values.valkey.enabled) }}
{{- end -}}

{{- define "harbor.redis.enableTLS" -}}
  {{- ternary "true" "false" (and (not .Values.valkey.enabled) (and .Values.externalRedis .Values.externalRedis.tlsOptions .Values.externalRedis.tlsOptions.enable)) }}
{{- end -}}
