#!/usr/bin/env bash
# =====================================================================
# Escenario 2 - Paso 1: generar los perfiles multimedia de prueba
#
# A partir de UN video original (el tuyo, 1080p) crea cuatro archivos con
# caracteristicas distintas. Nunca se SUBE la resolucion del original
# (el enunciado lo prohibe); solo se recorta y se reduce.
#
#   P1 corto  : primeros 30 s, reducido a 480p   -> rendiciones 480p, 360p
#   P2 medio  : primeros 90 s, reducido a 720p   -> 720p, 480p, 360p
#   P3 largo  : el original completo (copia)     -> 1080p, 720p, 480p, 360p
#   A1 audio  : solo la pista de audio del original -> a128 (audio HLS)
#
# Deja los archivos en capacity-planning/escenario2/media/ junto con
# perfiles.json, que describe cada perfil (duracion, resolucion, tamano,
# SHA-256, partes de la carga multipart y rendiciones esperadas).
# El script de k6 y el de preparacion leen ese perfiles.json.
#
# Uso (desde la raiz del repo, en WSL):
#   ./capacity-planning/escenario2/generar_perfiles.sh "C:\Users\kevin\Pictures\fotos bonitas tatacoa\Cantos bajo la Luna.mp4"
#
# Requiere ffmpeg/ffprobe. Si no los tienes en WSL, el script usa los que
# vienen dentro de la imagen del worker (docker compose build worker).
# Requiere jq y sha256sum.
# =====================================================================
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT="$DIR/media"
DEFAULT_VIDEO='C:\Users\kevin\Pictures\fotos bonitas tatacoa\Cantos bajo la Luna.mp4'
SRC="${1:-${VIDEO_PATH:-$DEFAULT_VIDEO}}"
WORKER_IMAGE="${WORKER_IMAGE:-mooc-platform-worker}"
PART_SIZE=$((8 * 1024 * 1024))   # igual que minPartSize en internal/api/handlers_media.go

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

# Convierte C:\... a /mnt/c/... cuando estamos en WSL (igual que los smoke tests).
resolve_path() {
  local p="$1"
  if [[ "$p" == [A-Za-z]:\\* ]] && command -v wslpath >/dev/null 2>&1; then
    wslpath -u "$p"
  else
    printf '%s' "$p"
  fi
}

SRC="$(resolve_path "$SRC")"
if [ ! -f "$SRC" ]; then
  echo "No existe el archivo: $SRC" >&2
  echo "(recuerda: en WSL2 tu disco C: esta en /mnt/c/...)" >&2
  exit 1
fi
mkdir -p "$OUT"

# ---------------------------------------------------------------------
# ffmpeg / ffprobe: locales si existen; si no, los del contenedor worker.
# En modo Docker se monta la carpeta del original como /in y media/ como /out.
# ---------------------------------------------------------------------
SRC_DIR="$(cd "$(dirname "$SRC")" && pwd)"
SRC_NAME="$(basename "$SRC")"
if command -v ffmpeg >/dev/null 2>&1 && command -v ffprobe >/dev/null 2>&1; then
  MODE=local
  IN="$SRC"; O="$OUT"
  ff()  { ffmpeg -hide_banner -loglevel error -y "$@"; }
  ffp() { ffprobe -v error "$@"; }
else
  MODE=docker
  if ! docker image inspect "$WORKER_IMAGE" >/dev/null 2>&1; then
    echo "No encontre ffmpeg en WSL ni la imagen '$WORKER_IMAGE'." >&2
    echo "Opciones: sudo apt install ffmpeg   o   docker compose build worker" >&2
    exit 1
  fi
  IN="/in/$SRC_NAME"; O="/out"
  DOCKER_RUN=(docker run --rm -u "$(id -u):$(id -g)" -v "$SRC_DIR:/in:ro" -v "$OUT:/out")
  ff()  { "${DOCKER_RUN[@]}" --entrypoint ffmpeg "$WORKER_IMAGE" -hide_banner -loglevel error -y "$@"; }
  ffp() { "${DOCKER_RUN[@]}" --entrypoint ffprobe "$WORKER_IMAGE" -v error "$@"; }
fi
echo "ffmpeg: modo $MODE"

say "Original"
ORIG_H=$(ffp -select_streams v:0 -show_entries stream=height -of csv=p=0 "$IN" | head -1)
ORIG_D=$(ffp -show_entries format=duration -of csv=p=0 "$IN" | head -1)
echo "$SRC_NAME -> ${ORIG_H}p, ${ORIG_D}s"

# Recorta la duracion pedida sin pasarse del original.
min_dur() { awk -v a="$1" -v b="$ORIG_D" 'BEGIN{ print (a < b ? a : b) }'; }
# Altura de salida: nunca mayor que la original.
min_h() { if [ "$1" -lt "$ORIG_H" ]; then echo "$1"; else echo "$ORIG_H"; fi; }

say "P1 corto (30 s, 480p)"
H=$(min_h 480)
ff -i "$IN" -t "$(min_dur 30)" -vf "scale=-2:$H" -c:v libx264 -preset veryfast -crf 23 \
   -c:a aac -b:a 128k -movflags +faststart "$O/p1_corto_${H}p.mp4"

say "P2 medio (90 s, 720p)"
H=$(min_h 720)
ff -i "$IN" -t "$(min_dur 90)" -vf "scale=-2:$H" -c:v libx264 -preset veryfast -crf 23 \
   -c:a aac -b:a 128k -movflags +faststart "$O/p2_medio_${H}p.mp4"

say "P3 largo (original completo, sin recodificar)"
ff -i "$IN" -c copy -movflags +faststart "$O/p3_largo_${ORIG_H}p.mp4"

say "A1 audio (pista de audio del original)"
ff -i "$IN" -vn -c:a copy "$O/a1_audio.m4a"

# ---------------------------------------------------------------------
# perfiles.json: la "ficha tecnica" de cada archivo. Se mide sobre el
# archivo generado (no se asume nada).
# ---------------------------------------------------------------------
say "Midiendo los archivos generados -> perfiles.json"
# Rendiciones que el worker DEBE producir (misma regla que internal/tasks/transcode.go:
# escalera 1080/720/480/360 sin upscaling; audio -> a128).
renditions_for() {
  local kind="$1" h="$2" out=()
  if [ "$kind" = "audio" ]; then echo '["a128"]'; return; fi
  for r in 1080 720 480 360; do [ "$r" -le "$h" ] && out+=("\"v$r\""); done
  if [ ${#out[@]} -eq 0 ]; then out=("\"v$h\""); fi
  (IFS=,; echo "[${out[*]}]")
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | awk '{print $1}'
  else shasum -a 256 "$1" | awk '{print $1}'; fi
}

ENTRIES=()
for spec in "P1:p1_corto_*.mp4:video:video/mp4" "P2:p2_medio_*.mp4:video:video/mp4" \
            "P3:p3_largo_*.mp4:video:video/mp4" "A1:a1_audio.m4a:audio:audio/mp4"; do
  IFS=: read -r ID PATTERN KIND MIME <<<"$spec"
  FILE=$(cd "$OUT" && ls $PATTERN | head -1)
  REF="$O/$FILE"
  DUR=$(ffp -show_entries format=duration -of csv=p=0 "$REF" | head -1)
  W=0; H=0
  if [ "$KIND" = "video" ]; then
    W=$(ffp -select_streams v:0 -show_entries stream=width -of csv=p=0 "$REF" | head -1)
    H=$(ffp -select_streams v:0 -show_entries stream=height -of csv=p=0 "$REF" | head -1)
  fi
  BR=$(ffp -show_entries format=bit_rate -of csv=p=0 "$REF" | head -1)
  SIZE=$(wc -c < "$OUT/$FILE" | tr -d ' ')
  PARTS=$(( (SIZE + PART_SIZE - 1) / PART_SIZE ))
  SHA=$(sha256_of "$OUT/$FILE")
  RENDS=$(renditions_for "$KIND" "$H")
  ENTRIES+=("$(jq -n --arg id "$ID" --arg f "$FILE" --arg k "$KIND" --arg m "$MIME" \
      --argjson d "$DUR" --argjson w "$W" --argjson h "$H" --argjson br "${BR:-0}" \
      --argjson s "$SIZE" --argjson p "$PARTS" --arg sha "$SHA" --argjson r "$RENDS" \
      '{id:$id, archivo:$f, kind:$k, mime:$m, duracion_s:($d|floor), ancho:$w, alto:$h,
        bitrate_kbps:(($br/1000)|floor), tamano_bytes:$s, partes_multipart:$p,
        sha256:$sha, rendiciones_esperadas:$r}')")
  printf '  %-3s %-22s %6.1fs  %4sp  %6.1f MB  %d parte(s)  %s\n' \
    "$ID" "$FILE" "$DUR" "$H" "$(awk -v s="$SIZE" 'BEGIN{print s/1048576}')" "$PARTS" "$RENDS"
done

printf '%s\n' "${ENTRIES[@]}" | jq -s --arg src "$SRC_NAME" --arg fecha "$(date -Iseconds)" \
  '{origen:$src, generado:$fecha, tamano_parte_bytes:8388608, perfiles:.}' > "$OUT/perfiles.json"

echo
echo "Listo. Archivos en: $OUT"
echo "Ficha tecnica:      $OUT/perfiles.json"
