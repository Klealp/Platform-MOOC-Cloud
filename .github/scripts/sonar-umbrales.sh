#!/usr/bin/env bash
# =====================================================================
# Verifica los umbrales de calidad de la Entrega 3 sobre TODO el codigo
# analizado (no solo el nuevo) y deja un resumen trazable al commit.
#
#   Duplicacion de codigo        <= 20 %
#   Mantenibilidad               A, B o C  (sqale_rating 1..3)
#   Deuda tecnica estimada       se registra (primera medicion de referencia)
#
# Se ejecuta despues del escaner: lee .scannerwork/report-task.txt, espera a
# que el servidor termine de procesar ESE analisis y consulta sus medidas.
#
# Variables:
#   SONAR_TOKEN       token con permiso de lectura del proyecto (obligatorio)
#   SONAR_BRANCH      rama analizada (en el pipeline, github.ref_name)
#   GITHUB_STEP_SUMMARY  si existe, el resumen tambien va ahi
# =====================================================================
set -euo pipefail

: "${SONAR_TOKEN:?falta SONAR_TOKEN}"
REPORTE="${REPORTE:-.scannerwork/report-task.txt}"
DUP_MAX=20
MANT_MAX=3   # 1=A 2=B 3=C 4=D 5=E

[ -f "$REPORTE" ] || { echo "No existe $REPORTE: el escaner no corrio o fallo." >&2; exit 1; }
leer() { grep -m1 "^$1=" "$REPORTE" | cut -d= -f2-; }
SERVIDOR="$(leer serverUrl)"
PROYECTO="$(leer projectKey)"
TAREA_URL="$(leer ceTaskUrl)"
PANEL="$(leer dashboardUrl)"

api() { curl -fsS -u "$SONAR_TOKEN:" "$@"; }

# 1. El escaner solo SUBE el reporte; el servidor lo procesa despues. Hasta
#    que la tarea no termine, las medidas serian las del analisis anterior.
for i in $(seq 1 60); do
  ESTADO="$(api "$TAREA_URL" | jq -r '.task.status')"
  case "$ESTADO" in
    SUCCESS) break ;;
    FAILED|CANCELED) echo "El servidor no pudo procesar el analisis ($ESTADO)." >&2; exit 1 ;;
  esac
  [ "$i" -eq 60 ] && { echo "El analisis sigue en cola tras 5 min." >&2; exit 1; }
  sleep 5
done

# 2. Medidas del codigo completo de la rama.
METRICAS="ncloc,duplicated_lines_density,sqale_rating,sqale_index,sqale_debt_ratio,code_smells,bugs,vulnerabilities,security_hotspots,coverage"
URL="$SERVIDOR/api/measures/component?component=$PROYECTO&metricKeys=$METRICAS"
[ -n "${SONAR_BRANCH:-}" ] && URL="$URL&branch=$SONAR_BRANCH"
MEDIDAS="$(api "$URL")"
valor() { echo "$MEDIDAS" | jq -r --arg m "$1" '.component.measures[] | select(.metric==$m) | .value' | head -1; }

DUP="$(valor duplicated_lines_density)"
MANT="$(valor sqale_rating)"
DEUDA_MIN="$(valor sqale_index)"
[ -n "$DUP" ] && [ -n "$MANT" ] || { echo "El servidor no devolvio duplicacion o mantenibilidad: $MEDIDAS" >&2; exit 1; }

letra() { case "${1%.*}" in 1) echo A ;; 2) echo B ;; 3) echo C ;; 4) echo D ;; *) echo E ;; esac; }
# sqale_index viene en minutos; SonarQube cuenta jornadas de 8 h.
horas() { awk -v m="$1" 'BEGIN { printf "%.1f h (%.1f jornadas de 8 h)", m/60, m/480 }'; }

OK_DUP=$(awk -v d="$DUP" -v x="$DUP_MAX" 'BEGIN { print (d <= x) ? "si" : "no" }')
OK_MANT=$(awk -v r="$MANT" -v x="$MANT_MAX" 'BEGIN { print (r <= x) ? "si" : "no" }')

RESUMEN="$(cat <<EOF
## Calidad del codigo (SonarQube)

Commit \`${GITHUB_SHA:-$(git rev-parse HEAD 2>/dev/null || echo desconocido)}\` · rama \`${SONAR_BRANCH:-principal}\` · [panel]($PANEL)

| Indicador | Valor | Umbral | Cumple |
|---|---|---|---|
| Duplicacion | ${DUP} % | <= ${DUP_MAX} % | ${OK_DUP} |
| Mantenibilidad | $(letra "$MANT") | A, B o C | ${OK_MANT} |
| Deuda tecnica estimada | $(horas "${DEUDA_MIN:-0}") | se registra | - |
| Ratio de deuda | $(valor sqale_debt_ratio) % | - | - |
| Lineas de codigo | $(valor ncloc) | - | - |
| Code smells / bugs / vulnerabilidades | $(valor code_smells) / $(valor bugs) / $(valor vulnerabilities) | - | - |
| Security hotspots | $(valor security_hotspots) | - | - |
| Cobertura | $(valor coverage) % | - | - |
EOF
)"
echo "$RESUMEN"
[ -n "${GITHUB_STEP_SUMMARY:-}" ] && echo "$RESUMEN" >> "$GITHUB_STEP_SUMMARY"

if [ "$OK_DUP" != si ] || [ "$OK_MANT" != si ]; then
  echo "No se cumplen los umbrales de la Entrega 3." >&2
  exit 1
fi
