{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "osc-bsu-csi-driver.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "osc-bsu-csi-driver.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "osc-bsu-csi-driver.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{/*
Common labels
*/}}
{{- define "osc-bsu-csi-driver.labels" -}}
{{ include "osc-bsu-csi-driver.selectorLabels" . }}
{{- if ne .Release.Name "kustomize" }}
helm.sh/chart: {{ include "osc-bsu-csi-driver.chart" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}
{{- end -}}

{{/*
Common selector labels
*/}}
{{- define "osc-bsu-csi-driver.selectorLabels" -}}
app.kubernetes.io/name: {{ include "osc-bsu-csi-driver.name" . }}
{{- if ne .Release.Name "kustomize" }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
{{- end -}}

{{/*
Convert the `--extra-volume-tags` command line arg from a map.
*/}}
{{- define "osc-bsu-csi-driver.extra-volume-tags" -}}
{{- $result := dict "pairs" (list) -}}
{{- range $key, $value := .Values.extraVolumeTags -}}
{{- $noop := printf "%s=%s" $key $value | append $result.pairs | set $result "pairs" -}}
{{- end -}}
{{- if gt (len $result.pairs) 0 -}}
{{- printf "%s=%s" "- --extra-volume-tags" (join "," $result.pairs) -}}
{{- end -}}
{{- end -}}

{{/*
Convert the `--extra-snapshot-tags` command line arg from a map.
*/}}
{{- define "osc-bsu-csi-driver.extra-snapshot-tags" -}}
{{- $result := dict "pairs" (list) -}}
{{- range $key, $value := .Values.extraSnapshotTags -}}
{{- $noop := printf "%s=%s" $key $value | append $result.pairs | set $result "pairs" -}}
{{- end -}}
{{- if gt (len $result.pairs) 0 -}}
{{- printf "%s=%s" "- --extra-snapshot-tags" (join "," $result.pairs) -}}
{{- end -}}
{{- end -}}
