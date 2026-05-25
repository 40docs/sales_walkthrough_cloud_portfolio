{{- define "walkthrough.name" -}}walkthrough{{- end -}}

{{- define "walkthrough.fullname" -}}
{{- printf "%s-%s" .Release.Name (include "walkthrough.name" .) | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "walkthrough.labels" -}}
app.kubernetes.io/name: {{ include "walkthrough.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "walkthrough.selectorLabels" -}}
app.kubernetes.io/name: {{ include "walkthrough.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "walkthrough.image" -}}
{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}
{{- end -}}

{{- define "walkthrough.databaseSecretName" -}}
{{- if .Values.postgres.cnpg.enabled -}}
{{ include "walkthrough.fullname" . }}-pg-app
{{- else -}}
{{ include "walkthrough.fullname" . }}-external-db
{{- end -}}
{{- end -}}
