#!/usr/bin/env bash
# =====================================================================
# Prueba de humo: curso con 2 modulos, cada uno con 2 unidades, y un
# video REAL subido por carga multipart en una de las unidades.
#
# Uso:
#   VIDEO_PATH=/mnt/c/Users/kevin/Videos/mivideo.mp4 ./testdata/smoke_video.sh
#
# o pasando la ruta como primer argumento:
#   ./testdata/smoke_video.sh /mnt/c/Users/kevin/Videos/mivideo.mp4
#
# Requiere: curl, jq, sha256sum. No requiere ffmpeg en tu maquina: el que
# transcodifica es el WORKER, dentro de su propio contenedor.
# =====================================================================
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
VIDEO_PATH="${1:-${VIDEO_PATH:-}}"
STAMP="$(date +%s)"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

if [ -z "$VIDEO_PATH" ]; then
  echo "Uso: VIDEO_PATH=/ruta/a/tu/video.mp4 $0"
  echo "  o: $0 /ruta/a/tu/video.mp4"
  exit 1
fi
if [ ! -f "$VIDEO_PATH" ]; then
  echo "No existe el archivo: $VIDEO_PATH"
  echo "(recuerda: en WSL2 tu disco C: esta en /mnt/c/...)"
  exit 1
fi

# =====================================================================
# 1. Login del administrador y creacion del profesor
# =====================================================================
say "1. Login del administrador"
ADMIN_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .access_token)

say "2. Crear profesor"
TEACHER_EMAIL="profe$STAMP@mooc.local"
curl -sS -X POST "$API/admin/users" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"full_name\":\"Profesora Demo\",\"password\":\"Profesor2026\",\"role\":\"teacher\"}" >/dev/null

TEACHER_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"password\":\"Profesor2026\"}" | jq -r .access_token)

# =====================================================================
# 2. Curso con 2 modulos x 2 unidades
# =====================================================================
say "3. Crear el curso"
COURSE=$(curl -sS -X POST "$API/courses" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Curso con video","summary":"Curso de prueba con contenido multimedia","category":"cloud","level":"beginner"}')
COURSE_ID=$(echo "$COURSE" | jq -r .id)
SLUG=$(echo "$COURSE" | jq -r .slug)
VERSION_ID=$(echo "$COURSE" | jq -r .draft_version.id)
echo "curso: $SLUG ($COURSE_ID)  version: $VERSION_ID"

# Guardamos el stable_id de la unidad que recibira el video para
# ubicarla facilmente mas adelante desde el punto de vista del estudiante.
VIDEO_UNIT_STABLE=""

say "4. Crear 2 modulos, cada uno con 2 unidades y contenido de texto"
for m in 1 2; do
  MODULE_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"title\":\"Modulo $m\"}" | jq -r .id)

  for u in 1 2; do
    UNIT=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
      -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
      -d "{\"title\":\"Unidad $m.$u\"}")
    UNIT_ID=$(echo "$UNIT" | jq -r .id)

    curl -sS -X POST "$API/units/$UNIT_ID/resources" \
      -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
      -d "{\"type\":\"rich_text\",\"title\":\"Lectura $m.$u\",
           \"content_md\":\"# Modulo $m, unidad $u\n\nContenido de prueba.\",\"required\":true}" >/dev/null
    echo "  modulo $m / unidad $u ($UNIT_ID) -> recurso de texto creado"

    # El video va en Modulo 1, Unidad 1.
    if [ "$m" = "1" ] && [ "$u" = "1" ]; then
      VIDEO_UNIT_ID="$UNIT_ID"
    fi
  done
done

# =====================================================================
# 3. Carga multipart REAL del video
# =====================================================================
say "5. Iniciar la carga multipart del video"
# Tamano del archivo, portable: wc -c funciona igual en macOS (BSD) y Linux (GNU),
# a diferencia de stat, que usa -f%z en macOS y -c%s en Linux.
FILE_SIZE=$(wc -c < "$VIDEO_PATH" | tr -d ' ')
FILE_NAME=$(basename "$VIDEO_PATH")
echo "archivo: $FILE_NAME ($((FILE_SIZE / 1024 / 1024)) MB)"

echo "calculando checksum SHA-256 (para demostrar la verificacion de integridad)..."
# macOS no trae sha256sum; usa shasum -a 256. Se detecta cual esta disponible.
if command -v sha256sum >/dev/null 2>&1; then
  CHECKSUM=$(sha256sum "$VIDEO_PATH" | awk '{print $1}')
else
  CHECKSUM=$(shasum -a 256 "$VIDEO_PATH" | awk '{print $1}')
fi

INIT=$(curl -sS -X POST "$API/uploads" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"original_name\":\"$FILE_NAME\",\"size_bytes\":$FILE_SIZE,\"kind\":\"video\",
       \"declared_mime\":\"video/mp4\",\"checksum_sha256\":\"$CHECKSUM\"}")

UPLOAD_ID=$(echo "$INIT" | jq -r .upload_id)
ASSET_ID=$(echo "$INIT" | jq -r .asset_id)
PART_SIZE=$(echo "$INIT" | jq -r .part_size)
TOTAL_PARTS=$(echo "$INIT" | jq -r .total_parts)
echo "asset_id: $ASSET_ID   partes: $TOTAL_PARTS de $((PART_SIZE / 1024 / 1024)) MB"

say "6. Subir cada parte directo al almacenamiento (MinIO)"
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

for i in $(seq 1 "$TOTAL_PARTS"); do
  PART_URL=$(echo "$INIT" | jq -r ".parts[] | select(.part_number==$i) | .url")
  OFFSET=$(( (i - 1) * PART_SIZE + 1 ))
  PART_FILE="$TMP_DIR/part_$i"

  # Se extrae el pedazo exacto del archivo sin cargarlo completo a memoria.
  tail -c +"$OFFSET" "$VIDEO_PATH" | head -c "$PART_SIZE" > "$PART_FILE"

  HTTP_CODE=$(curl -sS -o /dev/null -w '%{http_code}' -X PUT \
    --data-binary @"$PART_FILE" "$PART_URL")
  if [ "$HTTP_CODE" != "200" ]; then
    echo "ERROR subiendo la parte $i (HTTP $HTTP_CODE)"
    exit 1
  fi
  echo "  parte $i/$TOTAL_PARTS subida"
  rm -f "$PART_FILE"
done

say "7. Cerrar la carga (la API ensambla el objeto y encola el escaneo)"
curl -sS -X POST "$API/uploads/$UPLOAD_ID/complete" \
  -H "Authorization: Bearer $TEACHER_TOKEN" | jq -c .

say "8. Crear el recurso de video en Modulo 1 / Unidad 1"
curl -sS -X POST "$API/units/$VIDEO_UNIT_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"type\":\"video\",\"title\":\"Video de bienvenida\",\"required\":true,\"asset_id\":\"$ASSET_ID\"}" | jq -c .

# =====================================================================
# 4. Esperar a que el worker termine: escaneo + transcodificacion HLS
# =====================================================================
say "9. Esperando a que el worker verifique y transcodifique el video"
echo "(esto puede tardar segun la duracion del video: ffmpeg corre en el contenedor 'worker')"
ELAPSED=0
STATUS=""
while [ "$ELAPSED" -lt 900 ]; do
  ASSET=$(curl -sS "$API/assets/$ASSET_ID" -H "Authorization: Bearer $TEACHER_TOKEN")
  STATUS=$(echo "$ASSET" | jq -r .status)
  echo "  [$ELAPSED s] estado del asset: $STATUS"
  if [ "$STATUS" = "ready" ] || [ "$STATUS" = "failed" ] || [ "$STATUS" = "infected" ]; then
    break
  fi
  sleep 5
  ELAPSED=$((ELAPSED + 5))
done

if [ "$STATUS" != "ready" ]; then
  echo "El asset no quedo listo (estado final: $STATUS). Revisa: docker compose logs worker"
  echo "$ASSET" | jq .
  exit 1
fi
DURATION=$(echo "$ASSET" | jq -r .duration_secs)
echo "video listo. duracion detectada por ffprobe: ${DURATION}s"

# =====================================================================
# 5. Publicar el curso
# =====================================================================
say "10. Publicar la version"
curl -sS -X POST "$API/versions/$VERSION_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}' | jq -c .

# =====================================================================
# 6. Estudiante: registro, inscripcion y obtencion de la URL del video
# =====================================================================
say "11. Registrar y verificar un estudiante"
STUDENT_EMAIL="estudiante$STAMP@mooc.local"
REG=$(curl -sS -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"full_name\":\"Estudiante Demo\",\"password\":\"Estudiante2026\"}")
curl -sS -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$(echo "$REG" | jq -r .dev_verification_token)\"}" >/dev/null

STUDENT_TOKEN=$(curl -sS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"password\":\"Estudiante2026\"}" | jq -r .access_token)

say "12. Inscribirse"
ENROLLMENT_ID=$(curl -sS -X POST "$API/enrollments" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"$SLUG\"}" | jq -r .id)

say "13. Ubicar el recurso de video en el esquema del estudiante"
OUTLINE=$(curl -sS "$API/enrollments/$ENROLLMENT_ID/outline" -H "Authorization: Bearer $STUDENT_TOKEN")
STUDENT_ASSET_ID=$(echo "$OUTLINE" | jq -r '.modules[0].units[0].resources[] | select(.type=="video") | .asset_id')

say "14. Obtener las URLs de entrega (el servidor verifica la inscripcion antes de firmar)"
DELIVERY=$(curl -sS "$API/assets/$STUDENT_ASSET_ID/url" -H "Authorization: Bearer $STUDENT_TOKEN")
DOWNLOAD_URL=$(echo "$DELIVERY" | jq -r .download_url)
PLAYLIST_URL=$(echo "$DELIVERY" | jq -r .playlist_url)

echo
echo "=================================================================="
echo " LISTO. Formas de ver el video en tu PC:"
echo "=================================================================="
echo
echo "OPCION SIMPLE (archivo original, sin autenticacion en la URL):"
echo "  Copia y pega esta URL en tu navegador de Windows o en VLC:"
echo
echo "  $DOWNLOAD_URL"
echo
echo "  (valida por ${SIGNED_TTL:-15} minutos; si expira, vuelve a pedirla con:"
echo "   curl -H \"Authorization: Bearer $STUDENT_TOKEN\" $API/assets/$STUDENT_ASSET_ID/url | jq -r .download_url)"
echo
echo "OPCION AVANZADA (reproduccion adaptativa HLS real, requiere ffplay):"
echo "  ffplay -headers \"Authorization: Bearer $STUDENT_TOKEN\r\n\" \"$PLAYLIST_URL\""
echo
echo "  Esta es la ruta que en el futuro usara el frontend: el reproductor"
echo "  debe mandar el header Authorization en cada peticion al .m3u8."
echo "  Un navegador normal, sin JavaScript de por medio, NO puede hacer eso;"
echo "  por eso la opcion simple usa download_url en vez de playlist_url."
echo "=================================================================="
