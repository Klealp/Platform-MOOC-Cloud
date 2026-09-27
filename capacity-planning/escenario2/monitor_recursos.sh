#!/usr/bin/env bash
# =====================================================================
# Escenario 2 - Monitor de recursos (CPU, memoria, red y disco)
#
# Toma una muestra cada INTERVALO segundos y la agrega a un CSV:
#   - una fila por CONTENEDOR (docker stats): api, worker, redis, ...
#   - una fila "host" con la maquina completa (/proc): CPU, memoria,
#     disco usado y bytes de red acumulados.
#
# El enunciado pide CPU, memoria, red y disco de los servidores. Las metricas
# de la consola de GCP no traen memoria ni disco sin el Ops Agent; con este
# script no dependemos de eso.
#
# Donde se corre:
#   local : una sola vez en WSL (ve todos los contenedores de Docker Desktop)
#   nube  : uno en Web Server, otro en Worker Server y otro en la VM que
#           genera la carga (para demostrar que k6 no fue el limite).
#
# Uso:
#   ./capacity-planning/escenario2/monitor_recursos.sh <archivo.csv> [INTERVALO_S]
#   Ctrl+C para terminar (o kill, lo hace correr_nivel.sh).
# Requiere: docker (opcional: si no hay contenedores solo registra el host).
# =====================================================================
set -uo pipefail

OUT="${1:?Uso: $0 <archivo.csv> [intervalo_s]}"
INTERVALO="${2:-5}"
HOST="${HOST_ETIQUETA:-$(hostname)}"
mkdir -p "$(dirname "$OUT")"
[ -f "$OUT" ] || echo "ts_ms,host,contenedor,cpu_pct,mem_mb,mem_limite_mb,mem_pct,red_rx_mb,red_tx_mb,disco_lectura_mb,disco_escritura_mb,disco_usado_pct" > "$OUT"

# Convierte "12.5MiB", "1.2GB", "800kB", "0B" a megabytes (base 1024 o 1000 segun sufijo).
AWK_MB='
function mb(v,   n,u) {
  n = v; sub(/[A-Za-z]+$/, "", n); u = v; sub(/^[0-9.]+/, "", u)
  if (u=="B") return n/1048576
  if (u=="kB"||u=="KB") return n*1000/1048576
  if (u=="KiB") return n/1024
  if (u=="MB") return n*1000000/1048576
  if (u=="MiB") return n
  if (u=="GB") return n*1000000000/1048576
  if (u=="GiB") return n*1024
  if (u=="TB") return n*1e12/1048576
  if (u=="TiB") return n*1048576
  return n
}'

leer_cpu() { awk '/^cpu /{t=0; for(i=2;i<=NF;i++) t+=$i; print t, $5+$6}' /proc/stat; }
read -r CPU_T0 CPU_I0 < <(leer_cpu)

echo "monitor_recursos: $HOST -> $OUT (cada ${INTERVALO}s, Ctrl+C para terminar)"
trap 'echo "monitor_recursos: detenido"; exit 0' INT TERM

while :; do
  TS=$(date +%s%3N)

  # --- Contenedores ---
  if command -v docker >/dev/null 2>&1; then
    docker stats --no-stream --format '{{.Name}}|{{.CPUPerc}}|{{.MemUsage}}|{{.MemPerc}}|{{.NetIO}}|{{.BlockIO}}' 2>/dev/null |
    awk -F'|' -v ts="$TS" -v host="$HOST" "$AWK_MB"'
      {
        cpu=$2; sub(/%/,"",cpu); mp=$4; sub(/%/,"",mp)
        split($3, m, " / "); split($5, r, " / "); split($6, d, " / ")
        printf "%s,%s,%s,%s,%.1f,%.1f,%s,%.2f,%.2f,%.2f,%.2f,\n", ts, host, $1, cpu, mb(m[1]), mb(m[2]), mp, mb(r[1]), mb(r[2]), mb(d[1]), mb(d[2])
      }' >> "$OUT"
  fi

  # --- Maquina completa ---
  read -r CPU_T1 CPU_I1 < <(leer_cpu)
  DT=$((CPU_T1 - CPU_T0)); DI=$((CPU_I1 - CPU_I0))
  CPU_HOST=$(awk -v dt="$DT" -v di="$DI" 'BEGIN{ if (dt>0) printf "%.1f", 100*(dt-di)/dt; else print 0 }')
  CPU_T0=$CPU_T1; CPU_I0=$CPU_I1
  read -r MEM_TOT MEM_DISP < <(awk '/^MemTotal:/{t=$2} /^MemAvailable:/{a=$2} END{print t, a}' /proc/meminfo)
  MEM_USO=$(awk -v t="$MEM_TOT" -v a="$MEM_DISP" 'BEGIN{printf "%.1f %.1f %.1f", (t-a)/1024, t/1024, 100*(t-a)/t}')
  read -r RX TX < <(awk -F'[: ]+' 'NR>2 && $2!="lo" {rx+=$3; tx+=$11} END{printf "%.2f %.2f", rx/1048576, tx/1048576}' /proc/net/dev)
  read -r DR DW < <(awk '$3 ~ /^(sd[a-z]+|vd[a-z]+|nvme[0-9]+n[0-9]+|xvd[a-z]+)$/ {r+=$6; w+=$10} END{printf "%.2f %.2f", r*512/1048576, w*512/1048576}' /proc/diskstats)
  DISCO_PCT=$(df -P / | awk 'NR==2{gsub(/%/,"",$5); print $5}')
  read -r MU ML MP <<<"$MEM_USO"
  echo "$TS,$HOST,host,$CPU_HOST,$MU,$ML,$MP,$RX,$TX,$DR,$DW,$DISCO_PCT" >> "$OUT"

  sleep "$INTERVALO"
done
