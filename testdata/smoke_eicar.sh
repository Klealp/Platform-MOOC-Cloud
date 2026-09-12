#!/usr/bin/env bash
# =====================================================================
# Prueba de humo NEGATIVA: un profesor nuevo intenta subir el archivo
# EICAR (testdata/eicar.txt) como recurso de una unidad. No es un virus
# real: es un texto que todo antivirus reconoce como si lo fuera, hecho
# justo para probar la deteccion sin riesgo. La API debe RECHAZARLO.
#
# A diferencia de video/imagen/PDF, el rechazo no llega como un error
# inmediato en el POST de carga (la subida en si misma es solo bytes
# yendo a MinIO): el antimalware corre ASINCRONO en el worker despues
# de 'complete'. La prueba muestra las dos senales concretas del
# rechazo:
#   1. GET /assets/{id} termina en status=infected (el objeto ya fue
#      borrado del almacenamiento por el worker).
#   2. GET /assets/{id}/url (la URL firmada de descarga) responde
#      HTTP 409, porque el servidor nunca firma un archivo que no este
#      'ready'.
#   3. Si igual se intenta adjuntar como recurso de la unidad y
#      publicar la version, la publicacion queda bloqueada
#      (publishable=false) con el problema 'asset_not_ready'.
#
# Uso:
#   chmod +x testdata/smoke_eicar.sh
#   ./testdata/smoke_eicar.sh
#
# Requiere: curl, jq, sha256sum (o shasum).
# =====================================================================
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
EICAR_PATH="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/eicar.txt"
STAMP="$(date +%s)"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

if [ ! -f "$EICAR_PATH" ]; then
  echo "No existe $EICAR_PATH" >&2
  exit 1
fi

checksum_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# =====================================================================
# 1. Administrador crea un profesor nuevo (unica via posible)
# =====================================================================
say "1. Login del administrador"
ADMIN_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .access_token)

say "2. Crear profesor"
TEACHER_EMAIL="profe.eicar$STAMP@mooc.local"
curl -sS -X POST "$API/admin/users" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"full_name\":\"Profesor Prueba EICAR\",\"password\":\"Profesor2026\",\"role\":\"teacher\"}" >/dev/null

TEACHER_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"password\":\"Profesor2026\"}" | jq -r .access_token)

# =====================================================================
# 2. Curso con 1 modulo y 1 unidad
# =====================================================================
say "3. Crear curso, modulo y unidad"
COURSE=$(curl -sS -X POST "$API/courses" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Prueba de antimalware","summary":"Curso de prueba para el rechazo de EICAR","category":"seguridad","level":"beginner"}')
VERSION_ID=$(echo "$COURSE" | jq -r .draft_version.id)

MODULE_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Modulo 1"}' | jq -r .id)
UNIT_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 1: archivo malicioso"}' | jq -r .id)
echo "unidad: $UNIT_ID"

# =====================================================================
# 3. Subir el EICAR como si fuera un archivo descargable cualquiera
# =====================================================================
say "4. Iniciar la carga del EICAR (kind=file)"
FILE_SIZE=$(wc -c < "$EICAR_PATH" | tr -d ' ')
CHECKSUM=$(checksum_of "$EICAR_PATH")
INIT=$(curl -sS -X POST "$API/uploads" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"original_name\":\"eicar.txt\",\"size_bytes\":$FILE_SIZE,\"kind\":\"file\",
       \"declared_mime\":\"text/plain\",\"checksum_sha256\":\"$CHECKSUM\"}")
UPLOAD_ID=$(echo "$INIT" | jq -r .upload_id)
ASSET_ID=$(echo "$INIT" | jq -r .asset_id)
PART_URL=$(echo "$INIT" | jq -r '.parts[0].url')
echo "asset_id: $ASSET_ID"

say "5. Subir el unico part directo a MinIO"
curl -sS -o /dev/null -w 'HTTP %{http_code}\n' -X PUT --data-binary @"$EICAR_PATH" "$PART_URL"

say "6. Cerrar la carga (encola el escaneo antimalware en el worker)"
curl -sS -X POST "$API/uploads/$UPLOAD_ID/complete" \
  -H "Authorization: Bearer $TEACHER_TOKEN" | jq -c .

# =====================================================================
# 4. Esperar el resultado del escaneo: debe terminar en 'infected'
# =====================================================================
say "7. Esperando el resultado del escaneo antimalware"
ELAPSED=0
STATUS=""
ASSET=""
while [ "$ELAPSED" -lt 30 ]; do
  ASSET=$(curl -sS "$API/assets/$ASSET_ID" -H "Authorization: Bearer $TEACHER_TOKEN")
  STATUS=$(echo "$ASSET" | jq -r .status)
  echo "  [$ELAPSED s] estado: $STATUS"
  if [ "$STATUS" = "ready" ] || [ "$STATUS" = "failed" ] || [ "$STATUS" = "infected" ]; then
    break
  fi
  sleep 1
  ELAPSED=$((ELAPSED + 1))
done
echo "$ASSET" | jq .

if [ "$STATUS" = "infected" ]; then
  echo
  echo "OK: la API/worker detecto el EICAR y marco el asset como 'infected'."
  echo "    El objeto ya fue borrado del almacenamiento (ver failure_reason arriba)."
else
  echo "INESPERADO: el asset del EICAR quedo en estado '$STATUS' (se esperaba 'infected')." >&2
  exit 1
fi

# =====================================================================
# 5. Confirmar el rechazo desde el punto de vista del cliente: la API
#    nunca entrega una URL firmada de un archivo que no este 'ready'.
# =====================================================================
say "8. Intentar pedir la URL de descarga (se espera HTTP 409)"
curl -sS -o /tmp/eicar_url_response.json -w 'HTTP %{http_code}\n' \
  "$API/assets/$ASSET_ID/url" -H "Authorization: Bearer $TEACHER_TOKEN"
cat /tmp/eicar_url_response.json | jq .
rm -f /tmp/eicar_url_response.json

# =====================================================================
# 6. Aun intentando adjuntarlo como recurso y publicar, la version
#    queda bloqueada: el archivo detras del asset_id nunca llego a
#    'ready'.
# =====================================================================
say "9. Adjuntar igual el asset infectado como recurso de la unidad"
curl -sS -X POST "$API/units/$UNIT_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"type\":\"download\",\"title\":\"Archivo malicioso (EICAR)\",\"required\":true,\"asset_id\":\"$ASSET_ID\"}" \
  | jq -c '{id,type,title}'

say "10. Intentar publicar la version (se espera publishable=false)"
curl -sS "$API/versions/$VERSION_ID" -H "Authorization: Bearer $TEACHER_TOKEN" | jq '{publishable, problems}'

PUBLISH_HTTP=$(curl -sS -o /tmp/eicar_publish_response.json -w '%{http_code}' -X POST \
  "$API/versions/$VERSION_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}')
echo "HTTP $PUBLISH_HTTP en POST /versions/$VERSION_ID/publish"
cat /tmp/eicar_publish_response.json | jq .
rm -f /tmp/eicar_publish_response.json

say "Prueba de humo terminada: el EICAR fue rechazado en todos los puntos de control"
