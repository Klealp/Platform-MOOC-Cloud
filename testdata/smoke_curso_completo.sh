#!/usr/bin/env bash
# =====================================================================
# Prueba de humo: curso "grande" que combina TODO lo que las otras
# pruebas de humo cubren por separado (texto, imagen, video, PDF) mas
# un archivo de oficina (PPTX, subido como kind=file / type=download,
# porque el contrato de la API no tiene un tipo de recurso dedicado a
# presentaciones) y un enlace externo.
#
# Estructura del curso (5 modulos):
#   Modulo 1  -> 1 unidad, solo texto
#   Modulo 2  -> 1 unidad, solo texto
#   Modulo 3  -> 3 unidades, cada una con 2-3 recursos de formatos
#                distintos a la vez (texto + PDF/imagen/video/enlace)
#   Modulo 4  -> 1 unidad, solo texto
#   Modulo 5  -> 3 unidades, mismo patron que el Modulo 3, y la ultima
#                unidad ademas trae el quiz de cierre (10 preguntas x
#                2 opciones)
#
# Los 6 archivos reales (PPTX, MP4, 2 JPG, 2 PDF) se suben UNA sola vez
# cada uno en el Modulo 3; el Modulo 5 reutiliza el mismo asset_id en
# unidades distintas (el esquema lo permite: resources.asset_id no es
# unico) para no pagar dos veces la subida de un video de varias
# decenas de MB, y de paso deja ver que un mismo archivo puede
# aparecer en mas de un recurso.
#
# Dos estudiantes, mismo curso, resultados opuestos a proposito:
#   - Estudiante A: consume TODO el material obligatorio y aprueba el
#     quiz (10/10) -> inscripcion 'approved' y SI recibe insignia.
#   - Estudiante B: tambien consume todo el material (lo "ve"), pero
#     reprueba el quiz (3/10, 30% < 70%) -> nunca llega a 'approved' y
#     NO recibe insignia, aunque vio el 100% del contenido.
#
# Uso (con tus propios archivos, en orden PPTX MP4 JPG1 JPG2 PDF1 PDF2):
#   ./testdata/smoke_curso_completo.sh 'C:\ruta\pres.pptx' 'C:\ruta\video.mp4' \
#     'C:\ruta\foto1.jpg' 'C:\ruta\foto2.jpg' 'C:\ruta\doc1.pdf' 'C:\ruta\doc2.pdf'
#
#   o con variables de entorno: PPTX_PATH, VIDEO_PATH, IMAGE1_PATH,
#   IMAGE2_PATH, PDF1_PATH, PDF2_PATH. Si no se pasa nada se usan las
#   rutas de Windows por defecto de mas abajo (se convierten solas con
#   wslpath). Las rutas de Windows (C:\...) se convierten solas.
#
# Requiere: curl, jq, sha256sum (o shasum), wslpath (si las rutas son
# de Windows).
# =====================================================================
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
STAMP="$(date +%s)"

PPTX_PATH="${1:-${PPTX_PATH:-C:\Users\kevin\Documents\UNIVERSIDAD\9 semestre\Consultoría\3. Cosechas caña azucar\presentación tesis.pptx}}"
VIDEO_PATH="${2:-${VIDEO_PATH:-C:\Users\kevin\Pictures\fotos bonitas tatacoa\Cantos bajo la Luna.mp4}}"
IMAGE1_PATH="${3:-${IMAGE1_PATH:-C:\Users\kevin\Pictures\fotos bonitas tatacoa\StarTrail Grupo APT 04-06-2023.jpg}}"
IMAGE2_PATH="${4:-${IMAGE2_PATH:-C:\Users\kevin\Pictures\El David.jpg}}"
PDF1_PATH="${5:-${PDF1_PATH:-C:\Users\kevin\Documents\MAESTRIA\3 Semestre\Desarrollo de Soluciones Cloud\04 - SLI_SLO_y_SLA.pdf}}"
PDF2_PATH="${6:-${PDF2_PATH:-C:\Users\kevin\Documents\MAESTRIA\3 Semestre\Desarrollo de Soluciones Cloud\Casos de Estudio\Caso 1 actividad-diseno-infraestructura-gcp (2).pdf}}"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

# resolve_path convierte una ruta de Windows (C:\...) a la ruta que ve
# WSL2 (/mnt/c/...) usando wslpath, cuando esa utilidad esta disponible.
resolve_path() {
  local p="$1"
  if [[ "$p" == [A-Za-z]:\\* ]] && command -v wslpath >/dev/null 2>&1; then
    wslpath -u "$p"
  else
    printf '%s' "$p"
  fi
}

PPTX_PATH=$(resolve_path "$PPTX_PATH")
VIDEO_PATH=$(resolve_path "$VIDEO_PATH")
IMAGE1_PATH=$(resolve_path "$IMAGE1_PATH")
IMAGE2_PATH=$(resolve_path "$IMAGE2_PATH")
PDF1_PATH=$(resolve_path "$PDF1_PATH")
PDF2_PATH=$(resolve_path "$PDF2_PATH")

for f in "$PPTX_PATH" "$VIDEO_PATH" "$IMAGE1_PATH" "$IMAGE2_PATH" "$PDF1_PATH" "$PDF2_PATH"; do
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

# =====================================================================
# upload_asset: patron generico de carga multipart (init -> PUT directo
# a MinIO por cada parte -> complete -> polling del escaneo/transcode).
# Sirve para los 4 "kind" reales que acepta la API: pdf, image, video y
# file (este ultimo es el que usamos para el PPTX, porque el contrato
# no distingue formatos de oficina: kind=file acepta cualquier mimetype
# real, ver internal/tasks/scan.go:mimePermitido).
# =====================================================================
upload_asset() {
  local file_path="$1" kind="$2" declared_mime="$3" timeout_secs="$4"
  local file_name file_size checksum init upload_id asset_id part_size total_parts

  file_name=$(basename "$file_path")
  file_size=$(wc -c < "$file_path" | tr -d ' ')
  checksum=$(checksum_of "$file_path")

  init=$(curl -sS -X POST "$API/uploads" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"original_name\":\"$file_name\",\"size_bytes\":$file_size,\"kind\":\"$kind\",
         \"declared_mime\":\"$declared_mime\",\"checksum_sha256\":\"$checksum\"}")

  upload_id=$(echo "$init" | jq -r .upload_id)
  asset_id=$(echo "$init" | jq -r .asset_id)
  total_parts=$(echo "$init" | jq -r .total_parts)
  part_size=$(echo "$init" | jq -r .part_size)

  echo "  subiendo $file_name ($((file_size / 1024 / 1024)) MB, $total_parts parte(s)) -> asset $asset_id" >&2
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

  curl -sS -X POST "$API/uploads/$upload_id/complete" \
    -H "Authorization: Bearer $TEACHER_TOKEN" >/dev/null

  local elapsed=0 status=""
  while [ "$elapsed" -lt "$timeout_secs" ]; do
    status=$(curl -sS "$API/assets/$asset_id" -H "Authorization: Bearer $TEACHER_TOKEN" | jq -r .status)
    if [ "$status" = "ready" ] || [ "$status" = "failed" ] || [ "$status" = "infected" ]; then
      break
    fi
    sleep 3
    elapsed=$((elapsed + 3))
  done
  if [ "$status" != "ready" ]; then
    echo "El asset de $file_name no quedo listo (estado: $status). Revisa: docker compose logs worker" >&2
    exit 1
  fi

  echo "$asset_id"
}

mime_for() {
  case "${1,,}" in
    *.jpg|*.jpeg) echo "image/jpeg" ;;
    *.png)        echo "image/png" ;;
    *.mp4)        echo "video/mp4" ;;
    *.pdf)        echo "application/pdf" ;;
    *.pptx)       echo "application/vnd.openxmlformats-officedocument.presentationml.presentation" ;;
    *)            echo "application/octet-stream" ;;
  esac
}

# =====================================================================
# consume_resource: envia open + los heartbeat necesarios (acotados a
# 60s de credito por evento, igual que hace el servidor) + complete,
# hasta cubrir el dwell minimo real de cada tipo de recurso
# (progress.MinDwellSeconds). Sirve igual para un texto de 15s que para
# un video de varios minutos.
# =====================================================================
consume_resource() {
  local stable_id="$1" dwell_needed="$2" label="$3"
  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $CURRENT_STUDENT_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$CURRENT_ENROLLMENT_ID\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"open\"}" >/dev/null

  local remaining="$dwell_needed" delta
  while [ "$remaining" -gt 0 ]; do
    delta=$remaining
    if [ "$delta" -gt 60 ]; then delta=60; fi
    curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $CURRENT_STUDENT_TOKEN" \
      -H 'Content-Type: application/json' \
      -d "{\"enrollment_id\":\"$CURRENT_ENROLLMENT_ID\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"heartbeat\",\"delta_secs\":$delta}" >/dev/null
    remaining=$((remaining - delta))
  done

  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $CURRENT_STUDENT_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$CURRENT_ENROLLMENT_ID\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"complete\"}" \
    | jq -c --arg r "$label" '{recurso: $r, completado: .resource.completed, progress}'
}

create_resource() {
  local unit_id="$1" body="$2"
  curl -sS -X POST "$API/units/$unit_id/resources" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "$body"
}

# =====================================================================
# 1. Administrador crea la profesora (unica via posible para crear un
#    teacher, ver restriccion 10 de CLAUDE.md)
# =====================================================================
say "1. Login del administrador"
ADMIN_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .access_token)

say "2. Crear profesora"
TEACHER_EMAIL="profe$STAMP@mooc.local"
curl -sS -X POST "$API/admin/users" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"full_name\":\"Profesora Demo Completa\",\"password\":\"Profesor2026\",\"role\":\"teacher\"}" >/dev/null

TEACHER_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"password\":\"Profesor2026\"}" | jq -r .access_token)

# =====================================================================
# 2. Curso
# =====================================================================
say "3. Crear el curso"
COURSE=$(curl -sS -X POST "$API/courses" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Curso completo multiformato","summary":"Curso de prueba con 5 modulos y todos los formatos de contenido soportados","category":"cloud","level":"intermediate"}')
SLUG=$(echo "$COURSE" | jq -r .slug)
VERSION_ID=$(echo "$COURSE" | jq -r .draft_version.id)
echo "curso: $SLUG   version: $VERSION_ID"

# =====================================================================
# 3. Modulos 1, 2 y 4: 1 unidad, solo un recurso de texto
# =====================================================================
say "4. Modulos 1, 2 y 4: cada uno con 1 unidad de solo texto"
TEXT_ONLY_TITLES=("Modulo 1: Bienvenida al curso" "Modulo 2: Fundamentos teoricos" "Modulo 4: Marco de referencia")
for t in "${TEXT_ONLY_TITLES[@]}"; do
  MODULE_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"title\":\"$t\"}" | jq -r .id)
  UNIT_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d '{"title":"Unidad 1: Lectura"}' | jq -r .id)
  create_resource "$UNIT_ID" \
    "{\"type\":\"rich_text\",\"title\":\"Lectura: $t\",\"required\":true,\"content_md\":\"# $t\n\nContenido de prueba en texto enriquecido.\"}" \
    | jq -c '{id,type,title}'
done

# =====================================================================
# 4. Modulo 3: 3 unidades, cada una con texto + 1-2 archivos reales de
#    formato distinto (aqui se suben los 6 archivos, una sola vez).
# =====================================================================
say "5. Modulo 3: crear el modulo y sus 3 unidades"
MODULE3_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Modulo 3: Material multimedia (parte 1)"}' | jq -r .id)

UNIT_3_1=$(curl -sS -X POST "$API/modules/$MODULE3_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 3.1: Confiabilidad de servicios (texto + PDF + imagen)"}' | jq -r .id)
UNIT_3_2=$(curl -sS -X POST "$API/modules/$MODULE3_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 3.2: Estudio de caso (texto + PDF + imagen)"}' | jq -r .id)
UNIT_3_3=$(curl -sS -X POST "$API/modules/$MODULE3_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 3.3: Material audiovisual (texto + video + enlace)"}' | jq -r .id)

say "6. Unidad 3.1: texto + PDF (SLI/SLO/SLA) + imagen (El David)"
create_resource "$UNIT_3_1" \
  '{"type":"rich_text","title":"Lectura: confiabilidad de servicios","required":true,"content_md":"# Confiabilidad\n\nIntroduccion antes de leer el PDF adjunto."}' \
  | jq -c '{id,type,title}'
PDF1_NAME=$(basename "$PDF1_PATH")
ASSET_PDF1=$(upload_asset "$PDF1_PATH" pdf "$(mime_for "$PDF1_NAME")" 120)
create_resource "$UNIT_3_1" \
  "{\"type\":\"pdf\",\"title\":\"Documento: $PDF1_NAME\",\"required\":true,\"downloadable\":true,\"asset_id\":\"$ASSET_PDF1\"}" \
  | jq -c '{id,type,title}'
IMAGE2_NAME=$(basename "$IMAGE2_PATH")
ASSET_IMAGE2=$(upload_asset "$IMAGE2_PATH" image "$(mime_for "$IMAGE2_NAME")" 120)
create_resource "$UNIT_3_1" \
  "{\"type\":\"image\",\"title\":\"Imagen: $IMAGE2_NAME\",\"required\":true,\"asset_id\":\"$ASSET_IMAGE2\"}" \
  | jq -c '{id,type,title}'

say "7. Unidad 3.2: texto + PDF (caso GCP) + imagen (StarTrail)"
create_resource "$UNIT_3_2" \
  '{"type":"rich_text","title":"Lectura: contexto del caso de estudio","required":true,"content_md":"# Caso de estudio\n\nContexto antes de revisar el documento y la evidencia grafica."}' \
  | jq -c '{id,type,title}'
PDF2_NAME=$(basename "$PDF2_PATH")
ASSET_PDF2=$(upload_asset "$PDF2_PATH" pdf "$(mime_for "$PDF2_NAME")" 120)
create_resource "$UNIT_3_2" \
  "{\"type\":\"pdf\",\"title\":\"Documento: $PDF2_NAME\",\"required\":true,\"downloadable\":true,\"asset_id\":\"$ASSET_PDF2\"}" \
  | jq -c '{id,type,title}'
IMAGE1_NAME=$(basename "$IMAGE1_PATH")
ASSET_IMAGE1=$(upload_asset "$IMAGE1_PATH" image "$(mime_for "$IMAGE1_NAME")" 120)
create_resource "$UNIT_3_2" \
  "{\"type\":\"image\",\"title\":\"Imagen: $IMAGE1_NAME\",\"required\":true,\"asset_id\":\"$ASSET_IMAGE1\"}" \
  | jq -c '{id,type,title}'

say "8. Unidad 3.3: texto + video + enlace externo"
create_resource "$UNIT_3_3" \
  '{"type":"rich_text","title":"Lectura: introduccion al material audiovisual","required":true,"content_md":"# Material audiovisual\n\nMira el video y revisa el enlace de referencia."}' \
  | jq -c '{id,type,title}'
VIDEO_NAME=$(basename "$VIDEO_PATH")
ASSET_VIDEO=$(upload_asset "$VIDEO_PATH" video "$(mime_for "$VIDEO_NAME")" 900)
create_resource "$UNIT_3_3" \
  "{\"type\":\"video\",\"title\":\"Video: $VIDEO_NAME\",\"required\":true,\"asset_id\":\"$ASSET_VIDEO\"}" \
  | jq -c '{id,type,title}'
create_resource "$UNIT_3_3" \
  '{"type":"external_link","title":"Enlace: documentacion de referencia","required":false,"external_url":"https://cloud.google.com/docs"}' \
  | jq -c '{id,type,title}'

VIDEO_DURATION=$(curl -sS "$API/assets/$ASSET_VIDEO" -H "Authorization: Bearer $TEACHER_TOKEN" | jq -r .duration_secs)
echo "duracion detectada del video (ffprobe, en el worker): ${VIDEO_DURATION}s"

# =====================================================================
# 5. Modulo 5: 3 unidades. Las unidades 5.1 y 5.2 reutilizan el PPTX
#    (subido aqui, unica vez) y los assets ya subidos en el Modulo 3;
#    la unidad 5.3 trae el quiz de cierre.
# =====================================================================
say "9. Modulo 5: crear el modulo y sus 3 unidades"
MODULE5_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Modulo 5: Material multimedia (parte 2) y evaluacion"}' | jq -r .id)

UNIT_5_1=$(curl -sS -X POST "$API/modules/$MODULE5_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 5.1: Presentacion del proyecto (texto + PPTX + enlace)"}' | jq -r .id)
UNIT_5_2=$(curl -sS -X POST "$API/modules/$MODULE5_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 5.2: Documentacion de referencia (texto + PDF + imagen)"}' | jq -r .id)
UNIT_5_3=$(curl -sS -X POST "$API/modules/$MODULE5_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 5.3: Evaluacion final (texto + video + quiz)"}' | jq -r .id)

say "10. Unidad 5.1: texto + PPTX (kind=file / type=download) + enlace"
create_resource "$UNIT_5_1" \
  '{"type":"rich_text","title":"Lectura: presentacion del proyecto","required":true,"content_md":"# Presentacion\n\nDescarga la presentacion adjunta antes de continuar."}' \
  | jq -c '{id,type,title}'
PPTX_NAME=$(basename "$PPTX_PATH")
ASSET_PPTX=$(upload_asset "$PPTX_PATH" file "$(mime_for "$PPTX_NAME")" 180)
create_resource "$UNIT_5_1" \
  "{\"type\":\"download\",\"title\":\"Presentacion: $PPTX_NAME\",\"required\":true,\"downloadable\":true,\"asset_id\":\"$ASSET_PPTX\"}" \
  | jq -c '{id,type,title}'
create_resource "$UNIT_5_1" \
  '{"type":"external_link","title":"Enlace: repositorio del proyecto","required":false,"external_url":"https://github.com"}' \
  | jq -c '{id,type,title}'

say "11. Unidad 5.2: texto + PDF (reutiliza asset de la 3.1) + imagen (reutiliza asset de la 3.2)"
create_resource "$UNIT_5_2" \
  '{"type":"rich_text","title":"Lectura: repaso de la documentacion tecnica","required":true,"content_md":"# Repaso\n\nMaterial de referencia ya visto en el Modulo 3, disponible tambien aqui."}' \
  | jq -c '{id,type,title}'
create_resource "$UNIT_5_2" \
  "{\"type\":\"pdf\",\"title\":\"Documento (repaso): $PDF1_NAME\",\"required\":true,\"downloadable\":true,\"asset_id\":\"$ASSET_PDF1\"}" \
  | jq -c '{id,type,title}'
create_resource "$UNIT_5_2" \
  "{\"type\":\"image\",\"title\":\"Imagen (repaso): $IMAGE1_NAME\",\"required\":true,\"asset_id\":\"$ASSET_IMAGE1\"}" \
  | jq -c '{id,type,title}'

say "12. Unidad 5.3: texto + video (reutiliza asset de la 3.3) + quiz de cierre"
create_resource "$UNIT_5_3" \
  '{"type":"rich_text","title":"Lectura: antes de la evaluacion","required":true,"content_md":"# Evaluacion final\n\nRepasa el video y presenta el quiz de cierre."}' \
  | jq -c '{id,type,title}'
create_resource "$UNIT_5_3" \
  "{\"type\":\"video\",\"title\":\"Video (repaso): $VIDEO_NAME\",\"required\":true,\"asset_id\":\"$ASSET_VIDEO\"}" \
  | jq -c '{id,type,title}'

QUIZ_RES=$(create_resource "$UNIT_5_3" '{"type":"quiz","title":"Quiz de cierre","required":true}')
QUIZ_ID=$(echo "$QUIZ_RES" | jq -r .quiz_id)
echo "quiz_id: $QUIZ_ID"

# =====================================================================
# 6. Quiz de 10 preguntas x 2 opciones cada una.
# =====================================================================
say "13. Agregar 10 preguntas, cada una con 2 opciones"
QUIZ_PROMPTS=(
  "La API de esta plataforma guarda estado en memoria entre peticiones?"
  "Los archivos pasan por la API antes de llegar al almacenamiento?"
  "Postgres es la fuente de verdad y Redis solo cache/colas/rate limit?"
  "Se puede guardar un binario de un recurso directo en la base relacional?"
  "El porcentaje de avance de un estudiante lo calcula el cliente?"
  "La clave correcta de un quiz puede llegar al navegador del estudiante?"
  "Una version publicada del curso se puede editar en el mismo lugar?"
  "El acceso a un recurso de otra persona responde 403 en vez de 404?"
  "Un administrador puede registrarse como profesor por el registro publico?"
  "Debe quedar siempre al menos un administrador activo en el sistema?"
)
QUIZ_OPT_TRUE=(
  "No, no guarda estado local"
  "No, se suben con URLs prefirmadas directo al almacenamiento"
  "Si, esa es la regla de la plataforma"
  "No, solo se guarda la llave del objeto"
  "No, siempre lo calcula el servidor"
  "No, solo la usa gradeAttempt en el servidor"
  "No, se crea una version nueva clonando la anterior"
  "No, responde 404 para no confirmar que el recurso existe"
  "No, el registro publico siempre crea un estudiante"
  "Si, esa es una restriccion obligatoria"
)
QUIZ_OPT_FALSE=(
  "Si, mantiene cache en memoria por rendimiento"
  "Si, la API las recibe y las reenvia al almacenamiento"
  "No, Redis es la fuente de verdad principal"
  "Si, siempre que el archivo sea pequeno"
  "Si, el cliente envia el porcentaje calculado"
  "Si, viaja en el cuerpo del intento para validarlo en el navegador"
  "Si, se puede modificar directamente sin crear una version nueva"
  "Si, para que el cliente sepa que el recurso existe"
  "Si, si marca la casilla de profesor al registrarse"
  "No, puede quedar el sistema sin ningun administrador activo"
)

QUIZ_QUESTION_IDS=()
for i in "${!QUIZ_PROMPTS[@]}"; do
  BODY=$(jq -n --arg p "${QUIZ_PROMPTS[$i]}" \
    --arg ot "${QUIZ_OPT_TRUE[$i]}" --arg of "${QUIZ_OPT_FALSE[$i]}" \
    '{prompt:$p, options:[
        {text:$ot, is_correct:true},
        {text:$of, is_correct:false}
      ]}')
  Q=$(curl -sS -X POST "$API/quizzes/$QUIZ_ID/questions" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "$BODY")
  echo "$Q" | jq -c '{id,position,options}'
  QUIZ_QUESTION_IDS+=("$(echo "$Q" | jq -r .id)")
done

# =====================================================================
# 7. Previsualizacion y publicacion
# =====================================================================
say "14. Previsualizacion: la version deberia ser publicable"
curl -sS "$API/versions/$VERSION_ID" -H "Authorization: Bearer $TEACHER_TOKEN" \
  | jq '{publishable, problems}'

say "15. Publicar"
curl -sS -X POST "$API/versions/$VERSION_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}' | jq -c .

# =====================================================================
# 8. Funciones compartidas por los dos estudiantes: registrar,
#    verificar, inscribirse y consumir TODO el material obligatorio
#    (todo excepto el quiz, que se presenta aparte).
# =====================================================================
register_and_enroll() {
  local email="$1" name="$2"
  local reg token enrollment_id
  reg=$(curl -sS -X POST "$API/auth/register" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$email\",\"full_name\":\"$name\",\"password\":\"Estudiante2026\"}")
  curl -sS -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' \
    -d "{\"token\":\"$(echo "$reg" | jq -r .dev_verification_token)\"}" >/dev/null
  token=$(curl -sS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$email\",\"password\":\"Estudiante2026\"}" | jq -r .access_token)
  enrollment_id=$(curl -sS -X POST "$API/enrollments" \
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    -d "{\"slug\":\"$SLUG\"}" | jq -r .id)
  echo "$token|$enrollment_id"
}

consume_all_required() {
  local outline="$1"
  local stable_id rtype
  while IFS=$'\t' read -r stable_id rtype; do
    [ -z "$stable_id" ] && continue
    case "$rtype" in
      video)     consume_resource "$stable_id" "$(( (VIDEO_DURATION * 8 + 9) / 10 ))" "video ($rtype)" ;;
      rich_text|pdf) consume_resource "$stable_id" 15 "$rtype" ;;
      *)         consume_resource "$stable_id" 5 "$rtype" ;;
    esac
  done < <(echo "$outline" | jq -r '[.modules[].units[].resources[]?] | .[] | select(.type != "quiz") | "\(.stable_id)\t\(.type)"')
}

# =====================================================================
# 9. Estudiante A: consume TODO y aprueba el quiz (10/10) -> insignia
# =====================================================================
say "16. Estudiante A: registro e inscripcion"
STUDENT_A_EMAIL="estudiante.aprueba$STAMP@mooc.local"
IFS='|' read -r STUDENT_A_TOKEN ENROLLMENT_A_ID < <(register_and_enroll "$STUDENT_A_EMAIL" "Estudiante Aprueba")
echo "estudiante A: $STUDENT_A_EMAIL   inscripcion: $ENROLLMENT_A_ID"

OUTLINE_A=$(curl -sS "$API/enrollments/$ENROLLMENT_A_ID/outline" -H "Authorization: Bearer $STUDENT_A_TOKEN")
echo "$OUTLINE_A" | jq '{modulos: (.modules | length),
  detalle: [.modules[] | {modulo: .title, unidades: [.units[] | {unidad: .title, tipos: [.resources[].type]}]}]}'

say "17. Estudiante A: consumir TODO el material obligatorio (texto, PDF, imagenes, video)"
CURRENT_STUDENT_TOKEN="$STUDENT_A_TOKEN"
CURRENT_ENROLLMENT_ID="$ENROLLMENT_A_ID"
consume_all_required "$OUTLINE_A"

say "18. Estudiante A: presentar el quiz con las 10 respuestas correctas"
STUDENT_A_QUIZ_ID=$(echo "$OUTLINE_A" | jq -r '[.modules[].units[].resources[]? | select(.type=="quiz")][0].quiz_id')
ATTEMPT_A=$(curl -sS -X POST "$API/quizzes/$STUDENT_A_QUIZ_ID/attempts" -H "Authorization: Bearer $STUDENT_A_TOKEN")
ATTEMPT_A_ID=$(echo "$ATTEMPT_A" | jq -r .id)

ANSWERS_A='{}'
for i in "${!QUIZ_PROMPTS[@]}"; do
  q_stable=$(echo "$ATTEMPT_A" | jq -r --argjson i "$i" '.questions[$i].stable_id')
  o_stable=$(echo "$ATTEMPT_A" | jq -r --argjson i "$i" --arg t "${QUIZ_OPT_TRUE[$i]}" \
    '.questions[$i].options[] | select(.text == $t) | .stable_id')
  ANSWERS_A=$(echo "$ANSWERS_A" | jq --arg q "$q_stable" --arg o "$o_stable" '. + {($q): [$o]}')
done

curl -sS -X POST "$API/attempts/$ATTEMPT_A_ID/submit" \
  -H "Authorization: Bearer $STUDENT_A_TOKEN" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: quiz-pass-$STAMP" \
  -d "{\"answers\":$ANSWERS_A}" | jq -c '{score,passed,progress}'

say "19. Estudiante A: verificar insignia (state=approved, SI hay insignia)"
sleep 3
BADGES_A=$(curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_A_TOKEN")
echo "$BADGES_A" | jq -c '.data'
CODE_A=$(echo "$BADGES_A" | jq -r '.data[0].public_code // empty')
if [ -n "$CODE_A" ]; then
  echo "OK: insignia emitida. Verificacion publica (sin sesion):"
  curl -sS "$API/public/badges/$CODE_A" | jq -c .
else
  echo "ADVERTENCIA: no aparecio insignia para el estudiante que aprobo todo. Revisa 'docker compose logs worker'." >&2
fi

# =====================================================================
# 10. Estudiante B: consume TODO el material (lo "ve"), pero reprueba
#     el quiz (3/10 correctas, 30% < 70%) -> NUNCA llega a 'approved'
#     ni recibe insignia, aunque vio el 100% del contenido.
# =====================================================================
say "20. Estudiante B: registro e inscripcion"
STUDENT_B_EMAIL="estudiante.reprueba$STAMP@mooc.local"
IFS='|' read -r STUDENT_B_TOKEN ENROLLMENT_B_ID < <(register_and_enroll "$STUDENT_B_EMAIL" "Estudiante Reprueba")
echo "estudiante B: $STUDENT_B_EMAIL   inscripcion: $ENROLLMENT_B_ID"

OUTLINE_B=$(curl -sS "$API/enrollments/$ENROLLMENT_B_ID/outline" -H "Authorization: Bearer $STUDENT_B_TOKEN")

say "21. Estudiante B: consumir TODO el material obligatorio igual que el estudiante A"
CURRENT_STUDENT_TOKEN="$STUDENT_B_TOKEN"
CURRENT_ENROLLMENT_ID="$ENROLLMENT_B_ID"
consume_all_required "$OUTLINE_B"

say "22. Estudiante B: presentar el quiz con solo 3 de 10 correctas (30%, se espera passed=false)"
STUDENT_B_QUIZ_ID=$(echo "$OUTLINE_B" | jq -r '[.modules[].units[].resources[]? | select(.type=="quiz")][0].quiz_id')
ATTEMPT_B=$(curl -sS -X POST "$API/quizzes/$STUDENT_B_QUIZ_ID/attempts" -H "Authorization: Bearer $STUDENT_B_TOKEN")
ATTEMPT_B_ID=$(echo "$ATTEMPT_B" | jq -r .id)

N_CORRECT_B=3
ANSWERS_B='{}'
for i in "${!QUIZ_PROMPTS[@]}"; do
  q_stable=$(echo "$ATTEMPT_B" | jq -r --argjson i "$i" '.questions[$i].stable_id')
  if [ "$i" -lt "$N_CORRECT_B" ]; then
    target_text="${QUIZ_OPT_TRUE[$i]}"
  else
    target_text="${QUIZ_OPT_FALSE[$i]}"
  fi
  o_stable=$(echo "$ATTEMPT_B" | jq -r --argjson i "$i" --arg t "$target_text" \
    '.questions[$i].options[] | select(.text == $t) | .stable_id')
  ANSWERS_B=$(echo "$ANSWERS_B" | jq --arg q "$q_stable" --arg o "$o_stable" '. + {($q): [$o]}')
done

curl -sS -X POST "$API/attempts/$ATTEMPT_B_ID/submit" \
  -H "Authorization: Bearer $STUDENT_B_TOKEN" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: quiz-fail-$STAMP" \
  -d "{\"answers\":$ANSWERS_B}" | jq -c '{score,passed,progress}'

say "23. Estudiante B: confirmar que NO hay insignia (vio todo, pero reprobo el quiz)"
BADGES_B=$(curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_B_TOKEN")
echo "$BADGES_B" | jq -c '.data'
if [ "$(echo "$BADGES_B" | jq '.data | length')" = "0" ]; then
  echo "OK: sin insignia, como se esperaba (consumir todo el material no basta si el quiz reprueba)."
else
  echo "INESPERADO: aparecio una insignia habiendo reprobado el quiz." >&2
fi

say "Prueba de humo terminada"
