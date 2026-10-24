#!/usr/bin/env bash
# =====================================================================
# Ejecuta una corrida de k6 contra el despliegue y deja evidencia
# reproducible. Corre en cualquier maquina FUERA de Web Server y Worker
# Server (el PC de un integrante o una VM de pruebas), con k6 instalado o,
# si no lo esta, con la imagen oficial de Docker.
#
#   export BASE_URL=https://<ip-con-guiones>.sslip.io/api/v1
#   export ADMIN_PASSWORD='...'            # ~/.mooc-secrets/<proyecto>/admin_password
#   ./deploy/gcp/monitor/k6.sh capacity-planning/k6/escenario1_actividad_academica.js e1-nivel1
#   RATE_SCALE=0.2 ./deploy/gcp/monitor/k6.sh capacity-planning/k6/escenario1_actividad_academica.js e1-humo
#
# Opcional: PROM_RW_URL para que k6 envie sus metricas al Prometheus del Web
# Server (con el tunel IAP abierto) y verlas en Grafana junto a las de la API:
#   PROM_RW_URL=http://localhost:9090/api/v1/write          (k6 nativo)
#   PROM_RW_URL=http://host.docker.internal:9090/api/v1/write (k6 en Docker)
#
# Por cada corrida deja en capacity-planning/resultados/<corrida>/:
#   raw.json.gz     todas las muestras (--out json), para rehacer el analisis
#   summary.json    resumen de k6 (p50/p95/p99, checks, metricas propias)
#   condiciones.txt version de k6, commit, generador y parametros: el
#                   enunciado exige poder reconstruir bajo que condiciones
#                   se midio
# =====================================================================
set -euo pipefail

K6_IMG="grafana/k6:1.8.1"
script="${1:?uso: k6.sh <script.js> <corrida> [args de k6]}"
corrida="${2:?falta el nombre de la corrida (p. ej. e1-nivel1)}"
shift 2
: "${BASE_URL:?exporta BASE_URL (https://<dominio>/api/v1)}"
: "${ADMIN_PASSWORD:?exporta ADMIN_PASSWORD}"

RAIZ="$(cd "$(dirname "$0")/../../.." && pwd)"
out="capacity-planning/resultados/$corrida"
[ -e "$RAIZ/$out" ] && { echo "Ya existe $out: usa otro nombre de corrida." >&2; exit 1; }
mkdir -p "$RAIZ/$out"

params=(BASE_URL ADMIN_EMAIL ADMIN_PASSWORD N_STUDENTS RATE_SCALE PRE_VUS MAX_VUS)
salidas=(--out "json=$out/raw.json" --summary-export="$out/summary.json")
[ -n "${PROM_RW_URL:-}" ] && salidas+=(--out experimental-prometheus-rw)

if command -v k6 >/dev/null 2>&1; then
  version=$(k6 version | head -1)
  cmd=(k6)
else
  version="$K6_IMG (Docker)"
  envs=()
  for v in "${params[@]}" PROM_RW_URL; do [ -n "${!v:-}" ] && envs+=(-e "$v=${!v}"); done
  [ -n "${PROM_RW_URL:-}" ] && envs+=(-e "K6_PROMETHEUS_RW_SERVER_URL=$PROM_RW_URL")
  # En Git Bash (Windows) Docker necesita la ruta en formato Windows y no hay
  # que dejar que MSYS reescriba /repo.
  raiz_docker=$(cd "$RAIZ" && (pwd -W 2>/dev/null || pwd))
  export MSYS_NO_PATHCONV=1
  cmd=(docker run --rm "${envs[@]}" -v "$raiz_docker:/repo" -w /repo "$K6_IMG")
fi
export K6_PROMETHEUS_RW_SERVER_URL="${PROM_RW_URL:-}"
export K6_PROMETHEUS_RW_TREND_STATS="p(50),p(95),p(99),max"

{
  echo "corrida:   $corrida"
  echo "inicio:    $(date -u +%FT%TZ)"
  echo "script:    $script"
  echo "commit:    $(git -C "$RAIZ" rev-parse --short HEAD 2>/dev/null || echo desconocido)"
  echo "k6:        $version"
  echo "generador: $(hostname) ($(uname -s))"
  echo "base_url:  $BASE_URL"
  for v in N_STUDENTS RATE_SCALE PRE_VUS MAX_VUS; do echo "$v: ${!v:-<default del script>}"; done
  echo "prom_rw:   ${PROM_RW_URL:-no}"
  echo "args:      $*"
} | tee "$RAIZ/$out/condiciones.txt"

estado=0
(cd "$RAIZ" && "${cmd[@]}" run --tag testid="$corrida" "${salidas[@]}" "$@" "$script") || estado=$?

gzip -f "$RAIZ/$out/raw.json" 2>/dev/null || true
echo "fin:       $(date -u +%FT%TZ)  (salida k6: $estado)" | tee -a "$RAIZ/$out/condiciones.txt"
echo "Resultados en $out"
exit "$estado"
