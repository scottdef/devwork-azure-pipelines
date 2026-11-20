{{/*
Helpers of the cool-app chart.

Two rules keep these templates honest. Structured values are emitted with
toJson, never toYaml, so indentation cannot go wrong and coolctl can render
the chart without Helm. Numbers pass through int before they are compared or
printed, because Helm hands every number over as a float.
*/}}

{{/* The public host of the app: a preview host for a pull request. */}}
{{- define "cool-app.host" -}}
{{- if gt (int .Values.preview) 0 -}}
{{ .Values.app }}-pr-{{ int .Values.preview }}.preview.{{ .Values.environment }}.{{ .Values.platform.domain }}
{{- else -}}
{{ .Values.app }}.{{ .Values.environment }}.{{ .Values.platform.domain }}
{{- end -}}
{{- end }}

{{/* The in-cluster name of the app's Service. */}}
{{- define "cool-app.service" -}}
{{ .Values.app }}.{{ .Release.Namespace }}.svc.cluster.local
{{- end }}

{{/* Labels every object carries. */}}
{{- define "cool-app.labels" -}}
{{- $l := dict "app.kubernetes.io/name" .Values.app "app.kubernetes.io/part-of" .Values.team "app.kubernetes.io/managed-by" "cooldeploy" "helm.sh/chart" (printf "%s-%s" .Chart.Name .Chart.Version) "cool/team" .Values.team "cool/app" .Values.app "cool/env" .Values.environment -}}
{{- toJson $l -}}
{{- end }}

{{/* Labels of one release's objects. Call with (dict "root" $ "release" $r). */}}
{{- define "cool-app.releaseLabels" -}}
{{- $l := dict "app.kubernetes.io/name" .root.Values.app "app.kubernetes.io/part-of" .root.Values.team "app.kubernetes.io/managed-by" "cooldeploy" "cool/team" .root.Values.team "cool/app" .root.Values.app "cool/env" .root.Values.environment "cool/release" .release.id -}}
{{- if .root.Values.identity -}}
{{- $_ := set $l "azure.workload.identity/use" "true" -}}
{{- end -}}
{{- toJson $l -}}
{{- end }}

{{/* The selector of one release's pods. */}}
{{- define "cool-app.selector" -}}
{{- toJson (dict "cool/app" .root.Values.app "cool/release" .release.id) -}}
{{- end }}

{{/* Environment variables: the manifest's, then the environment's overrides. */}}
{{- define "cool-app.env" -}}
{{- $e := dict -}}
{{- range $k, $v := .Values.env -}}{{- $_ := set $e $k (toString $v) -}}{{- end -}}
{{- range $k, $v := .Values.overrides -}}{{- $_ := set $e $k (toString $v) -}}{{- end -}}
{{- toJson $e -}}
{{- end }}

{{/* The ID of the release with the most traffic. */}}
{{- define "cool-app.active" -}}
{{- $best := dict "id" "" "weight" -1 -}}
{{- range .Values.releases -}}
{{- if gt (int .weight) (int $best.weight) -}}
{{- $_ := set $best "id" .id -}}
{{- $_ := set $best "weight" (int .weight) -}}
{{- end -}}
{{- end -}}
{{- $best.id -}}
{{- end }}

{{/* "true" when at least one release takes traffic. */}}
{{- define "cool-app.live" -}}
{{- $live := false -}}
{{- range .Values.releases -}}{{- if gt (int .weight) 0 -}}{{- $live = true -}}{{- end -}}{{- end -}}
{{- if $live -}}true{{- end -}}
{{- end }}

{{/* Weighted Istio destinations: one per release that takes traffic. */}}
{{- define "cool-app.destinations" -}}
[
{{- $first := true -}}
{{- range .Values.releases -}}
{{- if gt (int .weight) 0 -}}
{{- if not $first }}, {{ end -}}
{"destination": {"host": {{ include "cool-app.service" $ | quote }}, "subset": {{ .id | quote }}, "port": {"number": 80}}, "weight": {{ int .weight }}}
{{- $first = false -}}
{{- end -}}
{{- end -}}
]
{{- end }}

{{/* Gateways the app's hosts bind to. */}}
{{- define "cool-app.gateways" -}}
{{- if and (eq .Values.expose "public") (eq (int .Values.preview) 0) -}}
{{- toJson (list "mesh" .Values.platform.mesh.internalGateway .Values.platform.mesh.externalGateway) -}}
{{- else -}}
{{- toJson (list "mesh" .Values.platform.mesh.internalGateway) -}}
{{- end -}}
{{- end }}

{{/* Pod and container security settings, shared by app and cron pods. */}}
{{- define "cool-app.podSecurity" -}}
{"runAsNonRoot": true, "runAsUser": 65532, "runAsGroup": 65532, "fsGroup": 65532, "seccompProfile": {"type": "RuntimeDefault"}}
{{- end }}
{{- define "cool-app.containerSecurity" -}}
{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": {"drop": ["ALL"]}}
{{- end }}
