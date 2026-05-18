{{- define "chart.name" -}}
{{- if .Chart }}
  {{- if .Chart.Name }}
    {{- .Chart.Name | trunc 63 | trimSuffix "-" }}
  {{- else if .Values.nameOverride }}
    {{ .Values.nameOverride | trunc 63 | trimSuffix "-" }}
  {{- else }}
    metal-operator
  {{- end }}
{{- else }}
  metal-operator
{{- end }}
{{- end }}


{{- define "chart.labels" -}}
{{- if .Chart.Version -}}
helm.sh/chart: {{ .Chart.Version | quote }}
{{- end }}
app.kubernetes.io/name: {{ include "chart.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}


{{- define "chart.selectorLabels" -}}
app.kubernetes.io/name: {{ include "chart.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}


{{- define "chart.hasMutatingWebhooks" -}}
{{- $hasMutating := false }}
{{- range . }}
  {{- if eq .type "mutating" }}
    $hasMutating = true }}{{- end }}
{{- end }}
{{ $hasMutating }}}}{{- end }}


{{- define "chart.hasValidatingWebhooks" -}}
{{- $hasValidating := false }}
{{- range . }}
  {{- if eq .type "validating" }}
    $hasValidating = true }}{{- end }}
{{- end }}
{{ $hasValidating }}}}{{- end }}

{{/*
chart.redfishLabelFlag renders a single CLI flag string from a map of
kubernetes-label-key -> prometheus-label-name entries.

Usage: {{ include "chart.redfishLabelFlag" (dict "flag" "redfish-metric-labels-from-bmc" "map" .Values.redfishLabels.bmc) }}
*/}}
{{- define "chart.redfishLabelFlag" -}}
{{- $pairs := list -}}
{{- range $k, $v := .map -}}
  {{- $pairs = append $pairs (printf "%s=%s" $k $v) -}}
{{- end -}}
{{- printf "--%s=%s" .flag (join "," ($pairs | sortAlpha)) -}}
{{- end }}
