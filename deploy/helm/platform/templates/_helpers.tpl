{{- define "platform.labels" -}}
app.kubernetes.io/name: platform
app.kubernetes.io/instance: {{ .Release.Name | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
app.kubernetes.io/part-of: k8s-internal-platform
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
{{- end -}}

{{- define "platform.selectorLabels" -}}
app.kubernetes.io/name: platform
app.kubernetes.io/instance: {{ .Release.Name | quote }}
{{- end -}}

{{- define "platform.image" -}}
{{- $repo := required "image.repository is required" .Values.image.repository -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repo .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $repo (required "image.tag or image.digest is required" .Values.image.tag) -}}
{{- end -}}
{{- end -}}
