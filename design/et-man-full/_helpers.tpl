{{/*
Expand the name of the chart.
*/}}
{{- define "easytrade.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "easytrade.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "easytrade.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "easytrade.labels" -}}
helm.sh/chart: {{ include "easytrade.chart" . }}
{{ include "easytrade.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- range $key, $val := .Values.labels }}
{{ $key }}: {{ $val }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "easytrade.selectorLabels" -}}
app.kubernetes.io/name: {{ include "easytrade.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "easytrade.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "easytrade.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Database connection string
*/}}
{{- define "easytrade.databaseConnection" -}}
Server={{ .Values.database.name }},{{ .Values.database.service.port }};Database={{ .Values.database.env.databaseName }};User Id=sa;Password={{ .Values.database.env.saPassword }};
{{- end }}

{{/*
Image pull policy
*/}}
{{- define "easytrade.imagePullPolicy" -}}
{{- .Values.global.imagePullPolicy | default "Always" }}
{{- end }}

{{/*
Full image name
*/}}
{{- define "easytrade.image" -}}
{{- $registry := .Values.global.imageRegistry -}}
{{- $repository := .repository -}}
{{- $tag := .tag | default $.Values.global.imageTag -}}
{{- printf "%s/%s:%s" $registry $repository $tag }}
{{- end }}
