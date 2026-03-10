{{/*
=============================================================================
Database helpers
=============================================================================
*/}}

{{/*
Return the database host
*/}}
{{- define "harbor.database.host" -}}
{{- .Values.database.host -}}
{{- end }}

{{/*
Return the database port
*/}}
{{- define "harbor.database.port" -}}
{{- .Values.database.port | default 5432 -}}
{{- end }}

{{/*
Return the database name
*/}}
{{- define "harbor.database.database" -}}
{{- .Values.database.database | default "registry" -}}
{{- end }}

{{/*
Return the database username
*/}}
{{- define "harbor.database.username" -}}
{{- .Values.database.username | default "postgres" -}}
{{- end }}

{{/*
Return the database password secret name
*/}}
{{- define "harbor.database.secretName" -}}
{{- if .Values.database.existingSecret }}
{{- .Values.database.existingSecret }}
{{- else }}
{{- include "harbor.fullname" . }}-database
{{- end }}
{{- end }}

{{/*
Return the database sslmode
*/}}
{{- define "harbor.database.sslmode" -}}
{{- .Values.database.sslmode | default "disable" -}}
{{- end }}
