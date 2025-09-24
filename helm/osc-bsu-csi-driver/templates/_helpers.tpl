{{/* vim: set filetype=mustache: */}}
{{/*
Expand the name of the chart.
*/}}
{{- define "osc-bsu-csi-driver.name" -}}
{{- .Chart.Name -}}
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
{{- range $key, $value := .Values.driver.extraVolumeTags -}}
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
{{- range $key, $value := .Values.driver.extraSnapshotTags -}}
{{- $noop := printf "%s=%s" $key $value | append $result.pairs | set $result "pairs" -}}
{{- end -}}
{{- if gt (len $result.pairs) 0 -}}
{{- printf "%s=%s" "- --extra-snapshot-tags" (join "," $result.pairs) -}}
{{- end -}}
{{- end -}}
