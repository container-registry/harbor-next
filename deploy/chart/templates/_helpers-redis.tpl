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
{{- printf "%s-auth" (.Values.valkey.fullnameOverride | default (printf "%s-valkey" .Release.Name)) }}
{{- else if .Values.externalRedis.existingSecret }}
{{- .Values.externalRedis.existingSecret }}
{{- else }}
{{- include "harbor.fullname" . }}-redis
{{- end }}
{{- end }}

{{/*
Return the Redis password key in the secret.
For valkey: `default-password` (matches valkey subchart's ACL `default` user key).
For external Redis with existingSecret: user-supplied `externalRedis.existingSecretKey`.
Otherwise: `REDIS_PASSWORD` (the generated external-redis Secret).
*/}}
{{- define "harbor.redis.secretKey" -}}
{{- if .Values.valkey.enabled -}}
default-password
{{- else if .Values.externalRedis.existingSecret -}}
{{- .Values.externalRedis.existingSecretKey | default "REDIS_PASSWORD" -}}
{{- else -}}
REDIS_PASSWORD
{{- end -}}
{{- end }}

{{/*
Return the Base Redis URL for Harbor components.
Honors `harbor.redis.scheme` (redis/rediss/redis+sentinel/rediss+sentinel) and
includes `$(REDIS_PASSWORD)` whenever auth is required:
  - valkey.auth.enabled
  - externalRedis.existingSecret (key supplies a password)
  - externalRedis.password is non-empty
*/}}
{{- define "harbor.redis.url" -}}
  {{- $root := . -}}
  {{- $scheme := include "harbor.redis.scheme" $root -}}
  {{- $host := include "harbor.redis.host" $root -}}
  {{- $port := include "harbor.redis.port" $root -}}
  {{- $masterSet := include "harbor.redis.masterSet" $root -}}
  {{- $extAuth := and (not .Values.valkey.enabled) (or (ne (.Values.externalRedis.existingSecret | toString) "") (ne (.Values.externalRedis.password | toString) "")) -}}
  {{- $needPw := or (eq (.Values.valkey.auth.enabled | toString) "true") $extAuth -}}
  {{- $creds := ternary ":$(REDIS_PASSWORD)@" "" (eq ($needPw | toString) "true") -}}
  {{- if $masterSet -}}
    {{- printf "%s://%s%s:%s/%s" $scheme $creds $host $port $masterSet -}}
  {{- else -}}
    {{- printf "%s://%s%s:%s" $scheme $creds $host $port -}}
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

{{/*
Whether the user supplied a custom CA bundle for external Redis (and,
incidentally, any other private-CA endpoint Harbor talks to over TLS —
S3, OIDC, LDAP). When true the chart mounts that Secret at
/etc/harbor/extra-ca/ on every component that opens a TLS connection
and sets SSL_CERT_DIR so Go's crypto/x509 also reads from that dir on
top of the default system bundle.
*/}}
{{- define "harbor.redis.caBundleEnabled" -}}
{{- $hasCA := false -}}
{{- if and .Values.externalRedis .Values.externalRedis.tlsOptions -}}
  {{- if ne (.Values.externalRedis.tlsOptions.existingCaSecret | toString) "" -}}
    {{- $hasCA = true -}}
  {{- end -}}
{{- end -}}
{{- if and (not .Values.valkey.enabled) $hasCA -}}true{{- end -}}
{{- end -}}

{{/*
Volume block — the Secret with the user-supplied CA bundle. Key
defaults to `ca.crt` so it Just Works for cert-manager Secrets, but
overridable via `externalRedis.tlsOptions.existingCaSecretKey`.
*/}}
{{- define "harbor.extraCA.volume" -}}
{{- if eq (include "harbor.redis.caBundleEnabled" .) "true" }}
- name: extra-ca
  secret:
    secretName: {{ .Values.externalRedis.tlsOptions.existingCaSecret }}
    items:
      - key: {{ .Values.externalRedis.tlsOptions.existingCaSecretKey | default "ca.crt" }}
        path: ca.crt
{{- end }}
{{- end }}

{{/*
VolumeMount block — mounted read-only at /etc/harbor/extra-ca.
*/}}
{{- define "harbor.extraCA.volumeMount" -}}
{{- if eq (include "harbor.redis.caBundleEnabled" .) "true" }}
- name: extra-ca
  mountPath: /etc/harbor/extra-ca
  readOnly: true
{{- end }}
{{- end }}

{{/*
SSL_CERT_DIR env — Go's crypto/x509 reads CAs from ALL paths in this
colon-separated list, supplementing (not replacing) the default system
bundle (`/etc/ssl/certs/ca-certificates.crt`) that the scratch-based
Harbor image already copies in. Order does not affect trust; any cert
in any listed dir is trusted.
*/}}
{{- define "harbor.extraCA.env" -}}
{{- if eq (include "harbor.redis.caBundleEnabled" .) "true" }}
- name: SSL_CERT_DIR
  value: "/etc/ssl/certs:/etc/harbor/extra-ca"
{{- end }}
{{- end }}
