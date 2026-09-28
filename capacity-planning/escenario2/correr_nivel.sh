#!/usr/bin/env bash
# =====================================================================
# Escenario 2 - Ejecuta UN nivel de carga de principio a fin
#
#   1. Crea resultados/<fecha>_<nivel>_<modo>/ y guarda las condiciones
#      (metadata.json): nivel, version de k6, commit, concurrencia de
#      workers, maquina que genera la carga...
#   2. Arranca los monitores (modo local): recursos (docker stats) y cola
#      (monitorcola). En modo nube recuerda arrancarlos en las VMs.
#   3. Corre k6 con el nivel pedido. Mientras corre, el panel en vivo de k6
#      queda en http://localhost:5665 (util para el video).
#   4. Espera a que la cola se vacie (drenaje) y detiene los monitores.
#   5. Ejecuta el analizador -> analisis.md, cargas.csv y graficas/.
#
# Uso (desde la raiz del repo):
#   ./capacity-planning/escenario2/correr_nivel.sh humo          # prueba rapida local
#   ./capacity-planning/escenario2/correr_nivel.sh base
#   MODO=nube ./capacity-planning/escenario2/correr_nivel.sh n1  # desde la VM generadora
#
# Variables opcionales:
#   MODO=local|nube (local)   API_URL (la de datos/seed.json)
#   REDIS_ADDR (127.0.0.1:6379, solo modo local)
#   WORKER_CONCURRENCY (se lee de .env en local; en nube escribela tu)
#   CARGAS_POR_MIN, ESPECTADORES, DURACION, CALENTAMIENTO  (sobrescriben el nivel)
#   TIMEOUT_DRENAJE_S (1800)
# Requiere: k6, jq, curl y haber corrido generar_perfiles.sh,
# preparar_datos.sh y construir_herramientas.sh.
# =====================================================================
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$DIR/../.." && pwd)"
NIVEL="${1:?Uso: $0 <nivel>   (niveles en niveles.json: humo, base, n1, n2, n3, estres)}"
MODO="${MODO:-local}"
SEED="$DIR/datos/seed.json"
PERFILES="$DIR/media/perfiles.json"
REDIS_ADDR="${REDIS_ADDR:-127.0.0.1:6379}"
TIMEOUT_DRENAJE_S="${TIMEOUT_DRENAJE_S:-1800}"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }
die() { echo "ERROR: $*" >&2; exit 1; }

command -v k6 >/dev/null || die "k6 no esta instalado (ver README.md, paso 0)"
command -v jq >/dev/null || die "falta jq (sudo apt install jq)"
[ -f "$SEED" ] || die "falta $SEED: corre preparar_datos.sh"
[ -f "$PERFILES" ] || die "falta $PERFILES: corre generar_perfiles.sh"
[ -x "$DIR/bin/analizador" ] || die "faltan las herramientas: corre construir_herramientas.sh"
jq -e --arg n "$NIVEL" '.niveles[$n]' "$DIR/niveles.json" >/dev/null || die "nivel '$NIVEL' no existe en niveles.json"

API_URL="${API_URL:-$(jq -r .api_url "$SEED")}"
EDAD_SEED=$(( $(date +%s) - $(date -d "$(jq -r .generado "$SEED")" +%s) ))
[ "$EDAD_SEED" -lt 82800 ] || die "seed.json tiene mas de 23 h: los tokens (24 h) van a vencer. Vuelve a correr preparar_datos.sh"

RUN="$DIR/resultados/$(date +%Y%m%d-%H%M)_${NIVEL}_${MODO}"
mkdir -p "$RUN"
echo "Resultados de esta corrida: $RUN"

# ---------------------------------------------------------------------
say "1. Condiciones de la corrida (metadata.json)"
if [ -z "${WORKER_CONCURRENCY:-}" ] && [ -f "$REPO/.env" ]; then
  WORKER_CONCURRENCY=$(grep -E '^WORKER_CONCURRENCY=' "$REPO/.env" | cut -d= -f2 || true)
fi
NIVEL_EF=$(jq --arg n "$NIVEL" --arg c "${CARGAS_POR_MIN:-}" --arg e "${ESPECTADORES:-}" \
  --arg d "${DURACION:-}" --arg w "${CALENTAMIENTO:-}" '.niveles[$n]
  | (if $c != "" then .cargas_por_min = ($c|tonumber) else . end)
  | (if $e != "" then .espectadores = ($e|tonumber) else . end)
  | (if $d != "" then .duracion = $d else . end)
  | (if $w != "" then .calentamiento = $w else . end)' "$DIR/niveles.json")
jq -n --arg nivel "$NIVEL" --arg modo "$MODO" --arg inicio "$(date -Iseconds)" --arg api "$API_URL" \
  --arg k6 "$(k6 version | head -1)" --arg commit "$(git -C "$REPO" rev-parse --short HEAD 2>/dev/null || echo '?')" \
  --arg wc "${WORKER_CONCURRENCY:-?}" \
  --arg gen "$(hostname), $(nproc) vCPU, $(awk '/MemTotal/{printf "%.1f GiB", $2/1048576}' /proc/meminfo)" \
  --argjson nivel_ef "$NIVEL_EF" --argjson comun "$(jq .comun "$DIR/niveles.json")" \
  --argjson perfiles "$(jq '[.perfiles[] | del(.sha256)]' "$PERFILES")" \
  --argjson datos "$(jq '{curso_contenido: .curso_contenido.slug, profesores: (.profesores|length), estudiantes: (.estudiantes|length), contenido: [.curso_contenido.assets[] | {perfil, duracion_s, variantes}]}' "$SEED")" \
  '{nivel:$nivel, modo:$modo, inicio:$inicio, api_url:$api, k6_version:$k6, git_commit:$commit,
    worker_concurrency:$wc, generador:$gen, nivel_efectivo:$nivel_ef, comun:$comun, perfiles:$perfiles, datos:$datos}' > "$RUN/metadata.json"
jq -c '.nivel_efectivo' "$RUN/metadata.json"

# ---------------------------------------------------------------------
say "2. Monitores"
PIDS=()
parar_monitores() {
  for p in "${PIDS[@]}"; do kill -TERM "$p" 2>/dev/null || true; done
  for p in "${PIDS[@]}"; do wait "$p" 2>/dev/null || true; done
  PIDS=()
}
trap 'parar_monitores' EXIT

if [ "$MODO" = "local" ]; then
  HOST_ETIQUETA=local "$DIR/monitor_recursos.sh" "$RUN/recursos_local.csv" 5 > "$RUN/monitor_recursos.log" 2>&1 &
  PIDS+=($!)
  "$DIR/bin/monitorcola" -redis "$REDIS_ADDR" -salida "$RUN" > "$RUN/monitor_cola.log" 2>&1 &
  PIDS+=($!)
  sleep 3
  kill -0 "${PIDS[1]}" 2>/dev/null || die "monitorcola no arranco: $(cat "$RUN/monitor_cola.log")"
  echo "monitor de recursos y monitor de cola corriendo"
else
  HOST_ETIQUETA=generador "$DIR/monitor_recursos.sh" "$RUN/recursos_generador.csv" 5 > "$RUN/monitor_recursos.log" 2>&1 &
  PIDS+=($!)
  cat <<EOF
MODO NUBE: este equipo solo registra sus propios recursos (recursos_generador.csv).
Antes de continuar, confirma que en las VMs estan corriendo (ver README, seccion Nube):
  Web Server   : ./monitor_recursos.sh ~/e2/recursos_web.csv 5
  Worker Server: ./monitor_recursos.sh ~/e2/recursos_worker.csv 5
                 ./monitorcola -redis 127.0.0.1:6379 -salida ~/e2
EOF
  read -r -p "Presiona Enter cuando esten corriendo... " _
fi

# ---------------------------------------------------------------------
say "3. Metricas de la API al inicio"
curl -s -m 5 "$API_URL/metrics" > "$RUN/metricas_api_inicio.txt" || echo "(no se pudo leer /metrics)"

say "4. k6 - nivel $NIVEL (panel en vivo: http://localhost:5665)"
cd "$DIR/k6"
set +e
K6_NO_USAGE_REPORT=true K6_WEB_DASHBOARD=true K6_WEB_DASHBOARD_EXPORT="$RUN/reporte_k6.html" \
NIVEL="$NIVEL" API_URL="$API_URL" \
  k6 run \
    --out "json=$RUN/crudo_k6.json.gz" \
    --console-output="$RUN/consola_k6.log" --log-format=json \
    --summary-export="$RUN/resumen_k6.json" \
    escenario2.js 2>&1 | tee "$RUN/salida_k6.txt"
K6_EXIT=${PIPESTATUS[0]}
set -e
cd "$REPO"
echo "k6 termino con codigo $K6_EXIT (99 = algun umbral no se cumplio; eso es un resultado, no un error)"

# ---------------------------------------------------------------------
say "5. Drenaje de la cola"
if [ "$MODO" = "local" ]; then
  # k6 ya espero a que cada carga llegara a ready; aqui se confirma que la
  # cola (default + low) quedo vacia en 3 muestras seguidas.
  INICIO=$(date +%s); VACIAS=0
  while [ $(( $(date +%s) - INICIO )) -lt "$TIMEOUT_DRENAJE_S" ]; do
    sleep 4
    ULT=$(tail -n 3 "$RUN/cola.csv" | awk -F, '$2=="default"||$2=="low"{d+=$4+$5+$6+$7} END{print d+0}')
    if [ "$ULT" = "0" ]; then VACIAS=$((VACIAS + 1)); else VACIAS=0; fi
    echo "  trabajos en cola: $ULT"
    [ "$VACIAS" -ge 3 ] && break
  done
  sleep 5
else
  echo "Observa en la Worker Server que la cola llegue a 0 (log de monitorcola)."
  read -r -p "Cuando este vacia, deten los monitores de las VMs y presiona Enter... " _
fi
parar_monitores
curl -s -m 5 "$API_URL/metrics" > "$RUN/metricas_api_fin.txt" || true

# ---------------------------------------------------------------------
if [ "$MODO" != "local" ]; then
  cat <<EOF

MODO NUBE: copia a $RUN los archivos de las VMs antes de analizar:
  cola.csv, tareas_eventos.csv, tareas_final.csv, recursos_web.csv, recursos_worker.csv
Luego ejecuta:
  $DIR/bin/analizador -corrida $RUN
EOF
  exit 0
fi

say "6. Analisis"
"$DIR/bin/analizador" -corrida "$RUN"
echo
echo "Listo. Abre $RUN/analisis.md (y reporte_k6.html en el navegador)."
