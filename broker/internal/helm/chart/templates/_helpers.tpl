{{- define "application.labels" -}}
app.kubernetes.io/name: application
app.kubernetes.io/instance: {{ .Release.Name | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service | quote }}
app.kubernetes.io/part-of: k8s-internal-platform
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | quote }}
platform.local/app-id: {{ required "appId is required" .Values.appId | quote }}
{{- end -}}

{{- define "application.selectorLabels" -}}
app.kubernetes.io/instance: {{ .Release.Name | quote }}
platform.local/app-id: {{ .Values.appId | quote }}
{{- end -}}

{{- define "application.image" -}}
{{- $repository := required "image.repository is required" .Values.image.repository -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" $repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" $repository (required "image.tag or image.digest is required" .Values.image.tag) -}}
{{- end -}}
{{- end -}}

{{- define "application.probe" -}}
{{- if eq .type "http" }}
httpGet:
  path: {{ .path | quote }}
  port: http
{{- else }}
tcpSocket:
  port: http
{{- end }}
initialDelaySeconds: {{ .initialDelaySeconds }}
periodSeconds: {{ .periodSeconds }}
timeoutSeconds: {{ .timeoutSeconds }}
failureThreshold: {{ .failureThreshold }}
{{- end -}}

{{- define "application.validateEnvironment" -}}
{{- $names := dict -}}
{{- range $name, $_ := .Values.env -}}
{{- $_ := set $names $name true -}}
{{- end -}}
{{- range .Values.secretRefs -}}
{{- if hasKey $names .name -}}
{{- fail (printf "environment variable %s is declared more than once in env/secretRefs" .name) -}}
{{- end -}}
{{- $_ := set $names .name true -}}
{{- end -}}
{{- end -}}
