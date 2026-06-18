{{- define "task-queue.namespace" -}}
{{- .Values.namespace -}}
{{- end -}}

{{- define "task-queue.image" -}}
{{- $registry := .Values.global.imageRegistry -}}
{{- $tag := .tag -}}
{{- if $registry }}{{ $registry }}/{{ $tag }}{{ else }}{{ $tag }}{{ end -}}
{{- end -}}

{{- define "task-queue.api.image" -}}
{{ include "task-queue.image" (dict "tag" .Values.images.tags.api "global" .Values.global) }}
{{- end -}}

{{- define "task-queue.worker.image" -}}
{{ include "task-queue.image" (dict "tag" .Values.images.tags.worker "global" .Values.global) }}
{{- end -}}

{{- define "task-queue.scheduler.image" -}}
{{ include "task-queue.image" (dict "tag" .Values.images.tags.scheduler "global" .Values.global) }}
{{- end -}}

{{- define "task-queue.redis.image" -}}
{{ include "task-queue.image" (dict "tag" .Values.images.tags.redis "global" .Values.global) }}
{{- end -}}
