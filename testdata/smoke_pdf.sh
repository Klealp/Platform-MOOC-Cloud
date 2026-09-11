#!/usr/bin/env bash
# =====================================================================
# Prueba de humo: curso con 3 modulos, cada uno con 1 unidad, y cada
# unidad con 2 recursos en orden: primero el texto de contenido del
# curso (rich_text) y despues un PDF real subido por carga multipart.
# La unidad del Modulo 3 ademas lleva un quiz de 3 preguntas x 3
# opciones cada una.
#
# A proposito el estudiante SOLO presenta y aprueba el quiz: nunca
# manda eventos de progreso sobre los textos ni los PDF. Sirve para
# demostrar que aprobar el quiz no basta para la insignia: la
# inscripcion solo llega a 'approved' cuando TODO el material
# obligatorio quedo consumido (ver smoke_imagenes.sh para el caso
# contrario, donde si se consume todo y la insignia SI se emite).
#
# Uso:
#   chmod +x testdata/smoke_pdf.sh
#   ./testdata/smoke_pdf.sh 'C:\ruta\doc1.pdf' 'C:\ruta\doc2.pdf' 'C:\ruta\doc3.pdf'
#
#   o con variables de entorno:
#   PDF1_PATH=/mnt/c/.../doc1.pdf PDF2_PATH=/mnt/c/.../doc2.pdf \
#   PDF3_PATH=/mnt/c/.../doc3.pdf ./testdata/smoke_pdf.sh
#
# Si la ruta es de Windows (C:\...) se convierte sola con wslpath.
# Requiere: curl, jq, sha256sum (o shasum), wslpath (si aplica).
# =====================================================================
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
PDF1_PATH="${1:-${PDF1_PATH:-}}"
PDF2_PATH="${2:-${PDF2_PATH:-}}"
PDF3_PATH="${3:-${PDF3_PATH:-}}"
STAMP="$(date +%s)"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

if [ -z "$PDF1_PATH" ] || [ -z "$PDF2_PATH" ] || [ -z "$PDF3_PATH" ]; then
  echo "Uso: $0 'ruta/doc1.pdf' 'ruta/doc2.pdf' 'ruta/doc3.pdf'"
  echo "  o: PDF1_PATH=... PDF2_PATH=... PDF3_PATH=... $0"
  exit 1
fi

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

PDF1_PATH=$(resolve_path "$PDF1_PATH")
PDF2_PATH=$(resolve_path "$PDF2_PATH")
PDF3_PATH=$(resolve_path "$PDF3_PATH")
for f in "$PDF1_PATH" "$PDF2_PATH" "$PDF3_PATH"; do
  if [ ! -f "$f" ]; then
    echo "No existe el archivo: $f" >&2
    echo "(recuerda: en WSL2 tu disco C: esta en /mnt/c/...)" >&2
    exit 1
  fi
done

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# =====================================================================
# 1. Administrador crea el profesor (unica via posible)
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
# 2. Curso con 3 modulos, cada uno con 1 unidad
# =====================================================================
say "3. Crear el curso"
COURSE=$(curl -sS -X POST "$API/courses" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Confiabilidad y arquitectura en la nube","summary":"Curso de prueba con lecturas en PDF","category":"cloud","level":"intermediate"}')
SLUG=$(echo "$COURSE" | jq -r .slug)
VERSION_ID=$(echo "$COURSE" | jq -r .draft_version.id)
echo "curso: $SLUG  version: $VERSION_ID"

# =====================================================================
# 3. Funcion para subir un PDF y devolver el asset_id (mismo patron de
#    carga multipart de smoke_video.sh / smoke_quiz.sh).
# =====================================================================
upload_pdf() {
  local file_path="$1"
  local file_name checksum init upload_id asset_id part_size total_parts i part_url offset part_file http_code
  file_name=$(basename "$file_path")
  local file_size
  file_size=$(wc -c < "$file_path" | tr -d ' ')

  if command -v sha256sum >/dev/null 2>&1; then
    checksum=$(sha256sum "$file_path" | awk '{print $1}')
  else
    checksum=$(shasum -a 256 "$file_path" | awk '{print $1}')
  fi

  init=$(curl -sS -X POST "$API/uploads" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"original_name\":\"$file_name\",\"size_bytes\":$file_size,\"kind\":\"pdf\",
         \"declared_mime\":\"application/pdf\",\"checksum_sha256\":\"$checksum\"}")

  upload_id=$(echo "$init" | jq -r .upload_id)
  asset_id=$(echo "$init" | jq -r .asset_id)
  total_parts=$(echo "$init" | jq -r .total_parts)
  part_size=$(echo "$init" | jq -r .part_size)

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

  # El escaneo (worker) no transcodifica PDFs: solo checksum, sniff de mime
  # y antimalware. Deberia quedar listo en pocos segundos.
  local elapsed=0 status=""
  while [ "$elapsed" -lt 60 ]; do
    status=$(curl -sS "$API/assets/$asset_id" -H "Authorization: Bearer $TEACHER_TOKEN" | jq -r .status)
    if [ "$status" = "ready" ] || [ "$status" = "failed" ] || [ "$status" = "infected" ]; then
      break
    fi
    sleep 1
    elapsed=$((elapsed + 1))
  done
  if [ "$status" != "ready" ]; then
    echo "El asset de $file_name no quedo listo (estado: $status)" >&2
    exit 1
  fi

  echo "$asset_id"
}

# =====================================================================
# 4. Un modulo por PDF: 1 unidad con 2 recursos (texto, luego PDF)
# =====================================================================
MODULE_TITLES=("Modulo 1: Arquitectura de la plataforma" "Modulo 2: Casos de estudio" "Modulo 3: Confiabilidad de servicios")
UNIT_TITLES=("Unidad 1: Documento de arquitectura" "Unidad 1: Diseno de infraestructura en GCP" "Unidad 1: SLI, SLO y SLA")
TEXT_TITLES=("Lectura: introduccion a la arquitectura" "Lectura: contexto del caso de estudio" "Lectura: fundamentos de confiabilidad")
PDF_PATHS=("$PDF1_PATH" "$PDF2_PATH" "$PDF3_PATH")

for i in 0 1 2; do
  n=$((i + 1))
  say "$((3 + i * 2 + 1)). Modulo $n: crear modulo, unidad y recurso de texto"
  MODULE_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"title\":\"${MODULE_TITLES[$i]}\"}" | jq -r .id)

  UNIT_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"title\":\"${UNIT_TITLES[$i]}\"}" | jq -r .id)

  curl -sS -X POST "$API/units/$UNIT_ID/resources" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"type\":\"rich_text\",\"title\":\"${TEXT_TITLES[$i]}\",\"required\":true,\"content_md\":\"# ${MODULE_TITLES[$i]}\n\nLee el documento adjunto en esta unidad.\"}" \
    | jq -c '{id,type,title}'

  say "$((3 + i * 2 + 2)). Modulo $n: subir e insertar el PDF"
  PDF_NAME=$(basename "${PDF_PATHS[$i]}")
  ASSET_ID=$(upload_pdf "${PDF_PATHS[$i]}")
  echo "pdf listo: $PDF_NAME -> $ASSET_ID"
  curl -sS -X POST "$API/units/$UNIT_ID/resources" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"type\":\"pdf\",\"title\":\"Documento: $PDF_NAME\",\"required\":true,\"downloadable\":true,\"asset_id\":\"$ASSET_ID\"}" \
    | jq -c '{id,type,title}'
done

# =====================================================================
# 5. Quiz de 3 preguntas x 3 opciones, en la unidad del Modulo 3 (tema:
#    SLI/SLO/SLA, igual que el PDF de esa unidad). $UNIT_ID quedo
#    apuntando a esa unidad al salir del ciclo anterior.
# =====================================================================
say "10. Crear el recurso de quiz en el Modulo 3"
QUIZ_RES=$(curl -sS -X POST "$API/units/$UNIT_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"quiz","title":"Quiz: SLI, SLO y SLA","required":true}')
QUIZ_ID=$(echo "$QUIZ_RES" | jq -r .quiz_id)
echo "quiz_id: $QUIZ_ID"

say "11. Agregar 3 preguntas, cada una con 3 opciones"
QUIZ_PROMPTS=(
  "Que mide un SLI (Service Level Indicator)?"
  "Que representa el error budget en el contexto de un SLO?"
  "A diferencia de un SLO, un SLA generalmente incluye:"
)
QUIZ_OPT1=("Una metrica cuantitativa del comportamiento real del servicio" "El margen tolerado de incumplimiento antes de violar el SLO" "Consecuencias contractuales por incumplimiento")
QUIZ_OPT2=("El contrato legal firmado con el cliente" "El costo economico de una caida del servicio" "Unicamente metricas internas del equipo")
QUIZ_OPT3=("El numero de servidores disponibles en el cluster" "La cantidad de servidores en produccion" "El codigo fuente del servicio")
QUIZ_CORRECT_POS=(1 1 1)

for i in "${!QUIZ_PROMPTS[@]}"; do
  BODY=$(jq -n --arg p "${QUIZ_PROMPTS[$i]}" \
    --arg o1 "${QUIZ_OPT1[$i]}" --arg o2 "${QUIZ_OPT2[$i]}" --arg o3 "${QUIZ_OPT3[$i]}" \
    --argjson idx "${QUIZ_CORRECT_POS[$i]}" \
    '{prompt:$p, options:[
        {text:$o1, is_correct:($idx==1)},
        {text:$o2, is_correct:($idx==2)},
        {text:$o3, is_correct:($idx==3)}
      ]}')
  curl -sS -X POST "$API/quizzes/$QUIZ_ID/questions" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "$BODY" | jq -c '{id,position,options}'
done

# =====================================================================
# 6. Previsualizacion y publicacion
# =====================================================================
say "12. Previsualizacion: la version deberia ser publicable"
curl -sS "$API/versions/$VERSION_ID" -H "Authorization: Bearer $TEACHER_TOKEN" \
  | jq '{publishable, problems}'

say "13. Publicar"
curl -sS -X POST "$API/versions/$VERSION_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}' | jq -c .

# =====================================================================
# 7. Nuevo estudiante: inscripcion y verificacion del esquema
# =====================================================================
say "14. Registrar y verificar un estudiante nuevo"
STUDENT_EMAIL="estudiante$STAMP@mooc.local"
REG=$(curl -sS -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"full_name\":\"Estudiante Demo\",\"password\":\"Estudiante2026\"}")
curl -sS -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$(echo "$REG" | jq -r .dev_verification_token)\"}" >/dev/null

STUDENT_TOKEN=$(curl -sS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"password\":\"Estudiante2026\"}" | jq -r .access_token)

ENROLLMENT_ID=$(curl -sS -X POST "$API/enrollments" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"$SLUG\"}" | jq -r .id)

say "15. Esquema visto por el estudiante: 3 modulos, 1 unidad c/u"
OUTLINE=$(curl -sS "$API/enrollments/$ENROLLMENT_ID/outline" -H "Authorization: Bearer $STUDENT_TOKEN")
echo "$OUTLINE" | jq '{modulos: (.modules | length),
  detalle: [.modules[] | {modulo: .title, unidades: [.units[] | {unidad: .title, tipos: [.resources[].type]}]}]}'

say "16. El estudiante obtiene la URL firmada del primer PDF (pero NO lo va a 'consumir')"
FIRST_PDF_ASSET=$(echo "$OUTLINE" | jq -r '[.modules[].units[].resources[]? | select(.type=="pdf")][0].asset_id')
curl -sS "$API/assets/$FIRST_PDF_ASSET/url" -H "Authorization: Bearer $STUDENT_TOKEN" \
  | jq '{asset_id, download_url_presente: (.download_url | length > 0)}'

# =====================================================================
# 8. El estudiante SOLO presenta y aprueba el quiz. A proposito NO se
#    manda ningun evento de progreso (open/heartbeat/complete) sobre
#    los textos ni los PDF: quedan sin consumir.
# =====================================================================
say "17. Presentar y aprobar el quiz (3 de 3 correctas), sin tocar los PDF ni los textos"
STUDENT_QUIZ_ID=$(echo "$OUTLINE" | jq -r '[.modules[].units[].resources[]? | select(.type=="quiz")][0].quiz_id')
ATTEMPT=$(curl -sS -X POST "$API/quizzes/$STUDENT_QUIZ_ID/attempts" -H "Authorization: Bearer $STUDENT_TOKEN")
ATTEMPT_ID=$(echo "$ATTEMPT" | jq -r .id)

ANSWERS='{}'
for i in "${!QUIZ_PROMPTS[@]}"; do
  correct_text="${QUIZ_OPT1[$i]}"
  question_stable=$(echo "$ATTEMPT" | jq -r --argjson i "$i" '.questions[$i].stable_id')
  option_stable=$(echo "$ATTEMPT" | jq -r --argjson i "$i" --arg t "$correct_text" \
    '.questions[$i].options[] | select(.text == $t) | .stable_id')
  ANSWERS=$(echo "$ANSWERS" | jq --arg q "$question_stable" --arg o "$option_stable" '. + {($q): [$o]}')
done

curl -sS -X POST "$API/attempts/$ATTEMPT_ID/submit" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: quiz-pdf-$STAMP" \
  -d "{\"answers\":$ANSWERS}" | jq -c '{score,passed,progress}'

# =====================================================================
# 9. Verificar que NO se emitio insignia: el quiz esta aprobado, pero
#    los 3 textos y los 3 PDF siguen sin consumirse, asi que la
#    inscripcion NUNCA llega a 'approved' (ver el campo state y
#    required_done/required_total en la respuesta del paso anterior).
# =====================================================================
say "18. Confirmar que NO hay insignia (falta consumir textos y PDF)"
BADGES=$(curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_TOKEN")
echo "$BADGES" | jq -c '.data'
if [ "$(echo "$BADGES" | jq '.data | length')" = "0" ]; then
  echo "OK: sin insignia, como se esperaba (quiz aprobado != curso aprobado)."
else
  echo "INESPERADO: aparecio una insignia sin haber consumido el material obligatorio." >&2
fi

say "Prueba de humo terminada"
