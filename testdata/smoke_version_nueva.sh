#!/usr/bin/env bash
# =====================================================================
# Prueba de humo: "una version publicada es inmutable" (restriccion 7
# de CLAUDE.md) visto desde el lado del estudiante que ya se gano su
# insignia.
#
# 1. Un profesor nuevo crea un curso de 1 modulo y 2 unidades, solo con
#    texto y una imagen (Unidad 2). Lo publica (version 1).
# 2. Un estudiante se inscribe, consume TODO el material y aprueba el
#    curso -> insignia emitida. Se guarda su codigo publico.
# 3. El profesor crea una version 2 (POST /courses/{id}/versions clona
#    la version 1 conservando los stable_id de modulos/unidades/
#    recursos, tal como exige la restriccion 7) y en ESA version nueva:
#      - cambia el titulo y el texto (content_md) de la lectura de la
#        Unidad 1 (mismo recurso, mismo stable_id: la API si permite
#        editar contenido dentro de una version en borrador).
#      - reemplaza la imagen de la Unidad 2: borra el recurso de
#        imagen original y sube una imagen distinta como recurso nuevo
#        (la API no tiene un endpoint para cambiar el asset_id de un
#        recurso existente). La imagen nueva se marca 'required:false'
#        a proposito, para que sea contenido agregado sin mover la
#        meta de recursos obligatorios que el estudiante ya cumplio.
#    Publica la version 2.
# 4. Se confirma que el estudiante que ya habia aprobado:
#      - sigue viendo su inscripcion en 'state=approved' (su progreso
#        sobrevive porque los recursos que SI contaban para su
#        aprobacion conservaron su stable_id).
#      - sigue teniendo exactamente la MISMA insignia (mismo
#        public_code, mismo issued_at) y esta se sigue verificando en
#        GET /public/badges/{code}, sin importar que el curso haya
#        cambiado de contenido debajo de sus pies.
#
# Uso:
#   ./testdata/smoke_version_nueva.sh 'C:\ruta\imagen_v1.jpg' 'C:\ruta\imagen_v2.jpg'
#
#   o con variables de entorno: IMAGE1_PATH (imagen de la version 1),
#   IMAGE2_PATH (la que la reemplaza en la version 2). Si no se pasa
#   nada se usan por defecto las mismas dos imagenes de
#   smoke_curso_completo.sh (rutas de Windows, se convierten solas con
#   wslpath).
#
# Requiere: curl, jq, sha256sum (o shasum), wslpath (si las rutas son
# de Windows).
# =====================================================================
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
STAMP="$(date +%s)"

IMAGE1_PATH="${1:-${IMAGE1_PATH:-C:\Users\kevin\Pictures\El David.jpg}}"
IMAGE2_PATH="${2:-${IMAGE2_PATH:-C:\Users\kevin\Pictures\fotos bonitas tatacoa\StarTrail Grupo APT 04-06-2023.jpg}}"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

resolve_path() {
  local p="$1"
  if [[ "$p" == [A-Za-z]:\\* ]] && command -v wslpath >/dev/null 2>&1; then
    wslpath -u "$p"
  else
    printf '%s' "$p"
  fi
}
IMAGE1_PATH=$(resolve_path "$IMAGE1_PATH")
IMAGE2_PATH=$(resolve_path "$IMAGE2_PATH")

for f in "$IMAGE1_PATH" "$IMAGE2_PATH"; do
  if [ ! -f "$f" ]; then
    echo "No existe el archivo: $f" >&2
    echo "(recuerda: en WSL2 tu disco C: esta en /mnt/c/...)" >&2
    exit 1
  fi
done

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

checksum_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}
mime_for_image() {
  case "${1,,}" in
    *.jpg|*.jpeg) echo "image/jpeg" ;;
    *.png)        echo "image/png" ;;
    *)            echo "application/octet-stream" ;;
  esac
}

upload_image() {
  local file_path="$1" token="$2"
  local file_name file_size checksum init upload_id asset_id part_size total_parts

  file_name=$(basename "$file_path")
  file_size=$(wc -c < "$file_path" | tr -d ' ')
  checksum=$(checksum_of "$file_path")

  init=$(curl -sS -X POST "$API/uploads" \
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    -d "{\"original_name\":\"$file_name\",\"size_bytes\":$file_size,\"kind\":\"image\",
         \"declared_mime\":\"$(mime_for_image "$file_name")\",\"checksum_sha256\":\"$checksum\"}")

  upload_id=$(echo "$init" | jq -r .upload_id)
  asset_id=$(echo "$init" | jq -r .asset_id)
  total_parts=$(echo "$init" | jq -r .total_parts)
  part_size=$(echo "$init" | jq -r .part_size)

  local i offset part_file part_url http_code
  for i in $(seq 1 "$total_parts"); do
    part_url=$(echo "$init" | jq -r ".parts[] | select(.part_number==$i) | .url")
    offset=$(( (i - 1) * part_size + 1 ))
    part_file="$TMP_DIR/part_${asset_id}_$i"
    tail -c +"$offset" "$file_path" | head -c "$part_size" > "$part_file"
    http_code=$(curl -sS -o /dev/null -w '%{http_code}' -X PUT --data-binary @"$part_file" "$part_url")
    if [ "$http_code" != "200" ]; then
      echo "ERROR subiendo la parte $i de $file_name (HTTP $http_code)" >&2
      exit 1
    fi
    rm -f "$part_file"
  done

  curl -sS -X POST "$API/uploads/$upload_id/complete" -H "Authorization: Bearer $token" >/dev/null

  local elapsed=0 status=""
  while [ "$elapsed" -lt 120 ]; do
    status=$(curl -sS "$API/assets/$asset_id" -H "Authorization: Bearer $token" | jq -r .status)
    if [ "$status" = "ready" ] || [ "$status" = "failed" ] || [ "$status" = "infected" ]; then
      break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
  done
  if [ "$status" != "ready" ]; then
    echo "El asset de $file_name no quedo listo (estado: $status)" >&2
    exit 1
  fi
  echo "$asset_id"
}

consume_resource() {
  local token="$1" enrollment_id="$2" stable_id="$3" dwell_needed="$4" label="$5"
  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$enrollment_id\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"open\"}" >/dev/null
  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$enrollment_id\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"heartbeat\",\"delta_secs\":$dwell_needed}" >/dev/null
  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $token" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$enrollment_id\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"complete\"}" \
    | jq -c --arg r "$label" '{recurso: $r, completado: .resource.completed, progress}'
}

# =====================================================================
# 1. Administrador crea el profesor
# =====================================================================
say "1. Login del administrador"
ADMIN_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .access_token)

say "2. Crear profesor"
TEACHER_EMAIL="profe.version$STAMP@mooc.local"
curl -sS -X POST "$API/admin/users" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"full_name\":\"Profesor Version Nueva\",\"password\":\"Profesor2026\",\"role\":\"teacher\"}" >/dev/null
TEACHER_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"password\":\"Profesor2026\"}" | jq -r .access_token)

# =====================================================================
# 2. Curso: 1 modulo, 2 unidades (texto, y texto + imagen)
# =====================================================================
say "3. Crear el curso"
COURSE=$(curl -sS -X POST "$API/courses" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Curso con insignia persistente","summary":"Curso de prueba para versiones nuevas e insignias ya emitidas","category":"cloud","level":"beginner"}')
COURSE_ID=$(echo "$COURSE" | jq -r .id)
SLUG=$(echo "$COURSE" | jq -r .slug)
V1_ID=$(echo "$COURSE" | jq -r .draft_version.id)
echo "curso: $SLUG ($COURSE_ID)   version 1: $V1_ID"

MODULE_ID=$(curl -sS -X POST "$API/versions/$V1_ID/modules" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Modulo 1: Fundamentos"}' | jq -r .id)

UNIT1_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 1: Introduccion"}' | jq -r .id)
curl -sS -X POST "$API/units/$UNIT1_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"rich_text","title":"Lectura: introduccion (version 1)","required":true,"content_md":"# Introduccion\n\nContenido de la version 1 del curso."}' \
  | jq -c '{id,type,title}'

UNIT2_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 2: Evidencia visual"}' | jq -r .id)
curl -sS -X POST "$API/units/$UNIT2_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"rich_text","title":"Lectura: evidencia (version 1)","required":true,"content_md":"# Evidencia\n\nMira la imagen adjunta."}' \
  | jq -c '{id,type,title}'

say "4. Subir la imagen de la version 1 y crear el recurso"
IMAGE1_NAME=$(basename "$IMAGE1_PATH")
ASSET1_ID=$(upload_image "$IMAGE1_PATH" "$TEACHER_TOKEN")
curl -sS -X POST "$API/units/$UNIT2_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"type\":\"image\",\"title\":\"Imagen (version 1): $IMAGE1_NAME\",\"required\":true,\"asset_id\":\"$ASSET1_ID\"}" \
  | jq -c '{id,type,title}'

say "5. Publicar la version 1"
curl -sS "$API/versions/$V1_ID" -H "Authorization: Bearer $TEACHER_TOKEN" | jq '{publishable, problems}'
curl -sS -X POST "$API/versions/$V1_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}' | jq -c .

# =====================================================================
# 3. Estudiante: inscripcion, consumo total y aprobacion
# =====================================================================
say "6. Registrar, verificar e inscribir al estudiante"
STUDENT_EMAIL="estudiante.version$STAMP@mooc.local"
REG=$(curl -sS -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"full_name\":\"Estudiante Version\",\"password\":\"Estudiante2026\"}")
curl -sS -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$(echo "$REG" | jq -r .dev_verification_token)\"}" >/dev/null
STUDENT_TOKEN=$(curl -sS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"password\":\"Estudiante2026\"}" | jq -r .access_token)
ENROLLMENT_ID=$(curl -sS -X POST "$API/enrollments" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"$SLUG\"}" | jq -r .id)
echo "inscripcion: $ENROLLMENT_ID"

OUTLINE_V1=$(curl -sS "$API/enrollments/$ENROLLMENT_ID/outline" -H "Authorization: Bearer $STUDENT_TOKEN")
echo "$OUTLINE_V1" | jq '{version_id, unidades: [.modules[0].units[] | {title, tipos: [.resources[].type]}]}'

say "7. Consumir TODO el material obligatorio (2 textos + 1 imagen)"
while IFS=$'\t' read -r stable_id rtype; do
  case "$rtype" in
    rich_text) dwell=15 ;;
    *)         dwell=5 ;;
  esac
  consume_resource "$STUDENT_TOKEN" "$ENROLLMENT_ID" "$stable_id" "$dwell" "$rtype"
done < <(echo "$OUTLINE_V1" | jq -r '[.modules[].units[].resources[]?] | .[] | "\(.stable_id)\t\(.type)"')

say "8. Verificar aprobacion e insignia (ANTES de la version 2)"
sleep 3
BADGES_BEFORE=$(curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_TOKEN")
echo "$BADGES_BEFORE" | jq -c '.data'
CODE=$(echo "$BADGES_BEFORE" | jq -r '.data[0].public_code // empty')
ISSUED_AT_BEFORE=$(echo "$BADGES_BEFORE" | jq -r '.data[0].issued_at // empty')
if [ -z "$CODE" ]; then
  echo "ERROR: el estudiante no recibio insignia con la version 1. Revisa 'docker compose logs worker'." >&2
  exit 1
fi
echo "codigo publico de la insignia: $CODE"
curl -sS "$API/public/badges/$CODE" | jq -c .

# =====================================================================
# 4. El profesor crea la version 2 (clon de la 1) y edita contenido
# =====================================================================
say "9. Crear la version 2 (clona modulos/unidades/recursos conservando stable_id)"
V2=$(curl -sS -X POST "$API/courses/$COURSE_ID/versions" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}')
V2_ID=$(echo "$V2" | jq -r .id)
echo "version 2: $V2_ID (clonada de $(echo "$V2" | jq -r .cloned_from))"

say "10. Ubicar en la version 2 el recurso de texto de la Unidad 1 y el de imagen de la Unidad 2"
V2_OUTLINE=$(curl -sS "$API/versions/$V2_ID" -H "Authorization: Bearer $TEACHER_TOKEN")
V2_TEXT1_ID=$(echo "$V2_OUTLINE" | jq -r '.modules[0].units[0].resources[] | select(.type=="rich_text") | .id')
V2_UNIT2_ID=$(echo "$V2_OUTLINE" | jq -r '.modules[0].units[1].id')
V2_IMAGE_ID=$(echo "$V2_OUTLINE" | jq -r '.modules[0].units[1].resources[] | select(.type=="image") | .id')
echo "recurso de texto (unidad 1): $V2_TEXT1_ID"
echo "unidad 2: $V2_UNIT2_ID   recurso de imagen a reemplazar: $V2_IMAGE_ID"

say "11. Cambiar el titulo y el texto de la lectura de la Unidad 1 (mismo stable_id)"
curl -sS -X PATCH "$API/resources/$V2_TEXT1_ID" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Lectura: introduccion (version 2, actualizada)"}' | jq -c .
curl -sS -X PUT "$API/resources/$V2_TEXT1_ID/content" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"content_md":"# Introduccion (actualizada)\n\nEste texto se edito en la version 2, despues de que un estudiante ya habia aprobado la version 1."}' \
  | jq -c .

say "12. Reemplazar la imagen: borrar la original y subir una nueva (no hay endpoint para cambiar el asset_id de un recurso existente)"
curl -sS -X DELETE "$API/resources/$V2_IMAGE_ID" -H "Authorization: Bearer $TEACHER_TOKEN" | jq -c .
IMAGE2_NAME=$(basename "$IMAGE2_PATH")
ASSET2_ID=$(upload_image "$IMAGE2_PATH" "$TEACHER_TOKEN")
# required:false a proposito: es contenido NUEVO (stable_id distinto), y no
# debe sumar a la meta de recursos obligatorios que el estudiante ya cumplio.
curl -sS -X POST "$API/units/$V2_UNIT2_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"type\":\"image\",\"title\":\"Imagen (version 2, actualizada): $IMAGE2_NAME\",\"required\":false,\"asset_id\":\"$ASSET2_ID\"}" \
  | jq -c '{id,type,title}'

say "13. Previsualizar y publicar la version 2"
curl -sS "$API/versions/$V2_ID" -H "Authorization: Bearer $TEACHER_TOKEN" | jq '{publishable, problems}'
curl -sS -X POST "$API/versions/$V2_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}' | jq -c .

# =====================================================================
# 5. El estudiante que ya aprobo: sigue aprobado y con la MISMA insignia
# =====================================================================
say "14. El estudiante vuelve a consultar su curso: ahora ve la version 2"
OUTLINE_V2=$(curl -sS "$API/enrollments/$ENROLLMENT_ID/outline" -H "Authorization: Bearer $STUDENT_TOKEN")
echo "$OUTLINE_V2" | jq '{version_id, unidades: [.modules[0].units[] | {title, tipos: [.resources[] | {type,title,required}]}], progress}'

STATE_AFTER=$(echo "$OUTLINE_V2" | jq -r .progress.state)
if [ "$STATE_AFTER" = "approved" ]; then
  echo "OK: la inscripcion sigue en 'approved' aunque el contenido cambio de version."
else
  echo "INESPERADO: la inscripcion paso a '$STATE_AFTER' tras publicar la version 2." >&2
fi

say "15. Confirmar que la insignia es EXACTAMENTE la misma (mismo codigo, mismo issued_at)"
BADGES_AFTER=$(curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_TOKEN")
echo "$BADGES_AFTER" | jq -c '.data'
CODE_AFTER=$(echo "$BADGES_AFTER" | jq -r '.data[0].public_code // empty')
ISSUED_AT_AFTER=$(echo "$BADGES_AFTER" | jq -r '.data[0].issued_at // empty')

echo "antes:   codigo=$CODE   emitida=$ISSUED_AT_BEFORE"
echo "despues: codigo=$CODE_AFTER   emitida=$ISSUED_AT_AFTER"
if [ "$CODE" = "$CODE_AFTER" ] && [ "$ISSUED_AT_BEFORE" = "$ISSUED_AT_AFTER" ]; then
  echo "OK: es la misma insignia, sin reemision, sin importar el cambio de version."
else
  echo "INESPERADO: la insignia cambio despues de publicar la version 2." >&2
  exit 1
fi

say "16. Verificacion publica de la insignia (sin sesion), tal como la veria cualquiera con el enlace"
curl -sS "$API/public/badges/$CODE_AFTER" | jq -c .

say "Prueba de humo terminada"
