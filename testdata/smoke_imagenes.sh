#!/usr/bin/env bash
# =====================================================================
# Prueba de humo: curso con 1 modulo y 3 unidades (texto + 2 imagenes) y
# un quiz de 5 preguntas con 4 opciones cada una. El estudiante consume
# TODO el material obligatorio (texto + imagenes) y aprueba el quiz, asi
# que la inscripcion queda 'approved' y se emite la insignia.
#
# A diferencia de smoke.sh (1 sola pregunta) esta prueba cubre la
# calificacion sobre varias preguntas: un primer intento con 3/5
# respuestas correctas (deberia reprobar, 60% < 70%) y un segundo
# intento con las 5 correctas (deberia aprobar).
#
# Las imagenes de las unidades 2 y 3 pueden ser TUYAS: pasa las rutas
# como argumentos o en IMAGE1_PATH / IMAGE2_PATH. Si es una ruta de
# Windows (C:\...) se convierte sola con wslpath. Si no pasas nada, se
# generan dos PNG minimos en caliente y no se agrega ningun binario a
# testdata/ (ver testdata/README.md).
#
# Uso:
#   chmod +x testdata/smoke_imagenes.sh
#   ./testdata/smoke_imagenes.sh 'C:\Users\kevin\Pictures\foto1.jpg' 'C:\Users\kevin\Pictures\foto2.jpg'
#
#   o con variables de entorno:
#   IMAGE1_PATH=/mnt/c/Users/kevin/Pictures/foto1.jpg \
#   IMAGE2_PATH=/mnt/c/Users/kevin/Pictures/foto2.jpg \
#   ./testdata/smoke_imagenes.sh
#
# Requiere: curl, jq, sha256sum (o shasum), base64.
# =====================================================================
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
IMAGE1_PATH="${1:-${IMAGE1_PATH:-}}"
IMAGE2_PATH="${2:-${IMAGE2_PATH:-}}"
STAMP="$(date +%s)"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# resolve_path convierte una ruta de Windows (C:\...) a la ruta que ve
# WSL2 (/mnt/c/...) usando wslpath, cuando esa utilidad esta disponible.
# Una ruta que ya es de Linux se devuelve intacta.
resolve_path() {
  local p="$1"
  if [[ "$p" == [A-Za-z]:\\* ]] && command -v wslpath >/dev/null 2>&1; then
    wslpath -u "$p"
  else
    printf '%s' "$p"
  fi
}

# mime_for_image adivina el declared_mime a partir de la extension. Solo es
# informativo (para el header del objeto en el almacenamiento): el worker
# igual valida el tipo real por los bytes, no por esto.
mime_for_image() {
  case "${1,,}" in
    *.jpg|*.jpeg) echo "image/jpeg" ;;
    *.png)        echo "image/png" ;;
    *.gif)        echo "image/gif" ;;
    *.webp)       echo "image/webp" ;;
    *)            echo "application/octet-stream" ;;
  esac
}

# =====================================================================
# 0. Las dos imagenes de las unidades 2 y 3: las que paso el usuario, o
#    dos PNG minimos (48x32, un solo color) generados en caliente.
# =====================================================================
if [ -n "$IMAGE1_PATH" ] && [ -n "$IMAGE2_PATH" ]; then
  IMAGE1_PATH=$(resolve_path "$IMAGE1_PATH")
  IMAGE2_PATH=$(resolve_path "$IMAGE2_PATH")
  for f in "$IMAGE1_PATH" "$IMAGE2_PATH"; do
    if [ ! -f "$f" ]; then
      echo "No existe el archivo: $f" >&2
      echo "(recuerda: en WSL2 tu disco C: esta en /mnt/c/...)" >&2
      exit 1
    fi
  done
  echo "usando imagenes propias:"
  echo "  unidad 2: $IMAGE1_PATH"
  echo "  unidad 3: $IMAGE2_PATH"
else
  echo "no se paso IMAGE1_PATH/IMAGE2_PATH: generando 2 imagenes de prueba"
  decode_b64() {
    if ! printf '%s' "$1" | base64 -d > "$2" 2>/dev/null; then
      printf '%s' "$1" | base64 -D > "$2"
    fi
  }

  RED_PNG_B64="iVBORw0KGgoAAAANSUhEUgAAADAAAAAgCAIAAADbtmxLAAAAMUlEQVR42u3OMQ0AAAgDsMmZfz2IwQXhaFIBzbSvREhISEhISEhISEhISEhISEjo0gKGYAhqgt6aDAAAAABJRU5ErkJggg=="
  BLUE_PNG_B64="iVBORw0KGgoAAAANSUhEUgAAADAAAAAgCAIAAADbtmxLAAAAMUlEQVR42u3OQQ0AAAgEoItjJrMbxhbOBxsBSPW8EiEhISEhISEhISEhISEhISGhSwt1OTR5dp3ImwAAAABJRU5ErkJggg=="

  decode_b64 "$RED_PNG_B64" "$TMP_DIR/portada.png"
  decode_b64 "$BLUE_PNG_B64" "$TMP_DIR/diagrama.png"
  IMAGE1_PATH="$TMP_DIR/portada.png"
  IMAGE2_PATH="$TMP_DIR/diagrama.png"
fi

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
# 2. Curso con 1 modulo y 3 unidades
# =====================================================================
say "3. Crear el curso"
COURSE=$(curl -sS -X POST "$API/courses" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Fundamentos de computacion en la nube","summary":"Curso de prueba con texto, imagenes y quiz","category":"cloud","level":"beginner"}')
COURSE_ID=$(echo "$COURSE" | jq -r .id)
SLUG=$(echo "$COURSE" | jq -r .slug)
VERSION_ID=$(echo "$COURSE" | jq -r .draft_version.id)
echo "curso: $SLUG ($COURSE_ID)  version: $VERSION_ID"

MODULE_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Modulo 1: Introduccion a la nube"}' | jq -r .id)

say "4. Unidad 1: recurso de texto"
UNIT1_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 1: Que es la nube"}' | jq -r .id)

curl -sS -X POST "$API/units/$UNIT1_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"rich_text","title":"Lectura: conceptos basicos","required":true,"content_md":"# Computacion en la nube\n\nInfraestructura, plataforma y software como servicio, entregados bajo demanda por internet."}' \
  | jq -c '{id,type,title}'

# =====================================================================
# 3. Funcion para subir una imagen y devolver el asset_id (mismo patron
#    de carga multipart de smoke_video.sh: sirve igual para los PNG de
#    prueba de 200 bytes que para una foto real de varios MB).
# =====================================================================
upload_image() {
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

  local declared_mime
  declared_mime=$(mime_for_image "$file_name")

  init=$(curl -sS -X POST "$API/uploads" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "{\"original_name\":\"$file_name\",\"size_bytes\":$file_size,\"kind\":\"image\",
         \"declared_mime\":\"$declared_mime\",\"checksum_sha256\":\"$checksum\"}")

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

  # El escaneo (worker) es rapido para imagenes: no hay transcodificacion,
  # pero una foto real de varias decenas de MB tarda mas que el PNG de
  # prueba en leerse completa para el checksum y el sniff de mimetype.
  local elapsed=0 status=""
  while [ "$elapsed" -lt 120 ]; do
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

say "5. Unidad 2: subir e insertar la primera imagen"
UNIT2_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 2: Modelos de servicio"}' | jq -r .id)
ASSET1_ID=$(upload_image "$IMAGE1_PATH")
echo "imagen 1 lista: $ASSET1_ID"
curl -sS -X POST "$API/units/$UNIT2_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"type\":\"image\",\"title\":\"Diagrama IaaS/PaaS/SaaS\",\"required\":true,\"asset_id\":\"$ASSET1_ID\"}" \
  | jq -c '{id,type,title}'

say "6. Unidad 3: subir e insertar la segunda imagen"
UNIT3_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 3: Evaluacion"}' | jq -r .id)
ASSET2_ID=$(upload_image "$IMAGE2_PATH")
echo "imagen 2 lista: $ASSET2_ID"
curl -sS -X POST "$API/units/$UNIT3_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"type\":\"image\",\"title\":\"Mapa de proveedores de nube\",\"required\":true,\"asset_id\":\"$ASSET2_ID\"}" \
  | jq -c '{id,type,title}'

# =====================================================================
# 4. Quiz de 5 preguntas x 4 opciones, en la Unidad 3
# =====================================================================
say "7. Crear el recurso de quiz"
QUIZ_RES=$(curl -sS -X POST "$API/units/$UNIT3_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"quiz","title":"Quiz: fundamentos de la nube","required":true}')
QUIZ_ID=$(echo "$QUIZ_RES" | jq -r .quiz_id)
echo "quiz_id: $QUIZ_ID"

say "8. Agregar 5 preguntas, cada una con 4 opciones"
# Arreglos paralelos: prompt, 4 opciones y posicion (1-4) de la correcta.
PROMPTS=(
  "Que significa la sigla IaaS en computacion en la nube?"
  "Cual de los siguientes es un modelo de despliegue de nube?"
  "Que servicio de AWS se usa tipicamente para almacenamiento de objetos?"
  "En el modelo PaaS, de que se encarga el proveedor ademas de la infraestructura?"
  "Que ventaja ofrece el autoescalado en la nube?"
)
OPT1=("Infraestructura como servicio" "Nube hibrida" "Amazon EC2" "Unicamente del hardware" "Reduce la seguridad del sistema")
OPT2=("Internet como servicio" "Nube compilada" "Amazon S3" "Del sistema operativo y el entorno de ejecucion" "Ajusta automaticamente los recursos segun la demanda")
OPT3=("Identidad como servicio" "Nube estatica" "Amazon RDS" "De nada, todo lo administra el cliente" "Elimina la necesidad de monitoreo")
OPT4=("Integracion como servicio" "Nube residente" "AWS Lambda" "Solo de la capa de red" "Aumenta el costo fijo de forma permanente")
CORRECT_POS=(1 1 2 2 2)

CORRECT_TEXT=()
for i in "${!PROMPTS[@]}"; do
  opts=("${OPT1[$i]}" "${OPT2[$i]}" "${OPT3[$i]}" "${OPT4[$i]}")
  correct_idx=${CORRECT_POS[$i]}
  CORRECT_TEXT+=("${opts[$((correct_idx - 1))]}")

  BODY=$(jq -n --arg p "${PROMPTS[$i]}" \
    --arg o1 "${OPT1[$i]}" --arg o2 "${OPT2[$i]}" --arg o3 "${OPT3[$i]}" --arg o4 "${OPT4[$i]}" \
    --argjson idx "$correct_idx" \
    '{prompt:$p, options:[
        {text:$o1, is_correct:($idx==1)},
        {text:$o2, is_correct:($idx==2)},
        {text:$o3, is_correct:($idx==3)},
        {text:$o4, is_correct:($idx==4)}
      ]}')

  curl -sS -X POST "$API/quizzes/$QUIZ_ID/questions" \
    -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
    -d "$BODY" | jq -c '{id,position,options}'
done

say "9. Vista de autor: 5 preguntas, 4 opciones cada una, con is_correct"
curl -sS "$API/quizzes/$QUIZ_ID" -H "Authorization: Bearer $TEACHER_TOKEN" \
  | jq '{preguntas: (.questions | length), opciones_por_pregunta: [.questions[].options | length]}'

# =====================================================================
# 5. Previsualizacion y publicacion
# =====================================================================
say "10. Previsualizacion: la version deberia ser publicable"
curl -sS "$API/versions/$VERSION_ID" -H "Authorization: Bearer $TEACHER_TOKEN" \
  | jq '{publishable, problems}'

say "11. Publicar"
curl -sS -X POST "$API/versions/$VERSION_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}' | jq -c .

# =====================================================================
# 6. Estudiante: inscripcion, esquema y acceso a las imagenes
# =====================================================================
say "12. Registrar y verificar un estudiante"
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

say "13. Esquema visto por el estudiante: 1 modulo, 3 unidades"
OUTLINE=$(curl -sS "$API/enrollments/$ENROLLMENT_ID/outline" -H "Authorization: Bearer $STUDENT_TOKEN")
echo "$OUTLINE" | jq '{modulos: (.modules | length),
  unidades: [.modules[0].units[] | {title, tipos: [.resources[].type]}]}'

STUDENT_QUIZ_ID=$(echo "$OUTLINE" | jq -r '.modules[0].units[] | .resources[]? | select(.type=="quiz") | .quiz_id')
STUDENT_ASSET1=$(echo "$OUTLINE" | jq -r '.modules[0].units[] | .resources[]? | select(.type=="image") | .asset_id' | head -1)
TEXT_STABLE=$(echo "$OUTLINE" | jq -r '.modules[0].units[] | .resources[]? | select(.type=="rich_text") | .stable_id')
mapfile -t IMAGE_STABLES < <(echo "$OUTLINE" | jq -r '.modules[0].units[] | .resources[]? | select(.type=="image") | .stable_id')

say "14. El estudiante obtiene una URL firmada de la primera imagen"
curl -sS "$API/assets/$STUDENT_ASSET1/url" -H "Authorization: Bearer $STUDENT_TOKEN" \
  | jq '{asset_id, download_url_presente: (.download_url | length > 0)}'

# =====================================================================
# 7. Consumir TODO el material obligatorio (texto + 2 imagenes). Sin
#    esto la inscripcion nunca llega a 'approved', sin importar que el
#    quiz se apruebe: el progreso lo calcula el servidor a partir de
#    senales open/heartbeat/complete, no de que el recurso exista.
# =====================================================================
consume_resource() {
  local stable_id="$1" dwell_needed="$2" label="$3"
  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $STUDENT_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$ENROLLMENT_ID\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"open\"}" >/dev/null
  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $STUDENT_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$ENROLLMENT_ID\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"heartbeat\",\"delta_secs\":$dwell_needed}" >/dev/null
  curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $STUDENT_TOKEN" \
    -H 'Content-Type: application/json' \
    -d "{\"enrollment_id\":\"$ENROLLMENT_ID\",\"resource_stable_id\":\"$stable_id\",\"event_type\":\"complete\"}" \
    | jq -c --arg r "$label" '{recurso: $r, completado: .resource.completed, progress}'
}

say "15. Consumir el texto (dwell minimo: 15s)"
consume_resource "$TEXT_STABLE" 15 "texto"

say "16. Consumir la imagen 1 (dwell minimo: 5s)"
consume_resource "${IMAGE_STABLES[0]}" 5 "imagen 1"

say "17. Consumir la imagen 2 (dwell minimo: 5s)"
consume_resource "${IMAGE_STABLES[1]}" 5 "imagen 2"

# =====================================================================
# 8. Presentar el quiz de 5 preguntas: un intento que reprueba y uno
#    que aprueba, para verificar la calificacion sobre varias preguntas.
# =====================================================================
answer_and_submit() {
  local n_correct="$1" idem_key="$2" show_proof="${3:-false}"
  local attempt attempt_id answers question_stable option_stable target_text
  attempt=$(curl -sS -X POST "$API/quizzes/$STUDENT_QUIZ_ID/attempts" -H "Authorization: Bearer $STUDENT_TOKEN")
  attempt_id=$(echo "$attempt" | jq -r .id)

  if [ "$show_proof" = "true" ]; then
    echo "opciones que ve el estudiante (sin is_correct):"
    echo "$attempt" | jq -c '.questions[0].options'
  fi

  answers='{}'
  for i in "${!PROMPTS[@]}"; do
    question_stable=$(echo "$attempt" | jq -r --argjson i "$i" '.questions[$i].stable_id')
    if [ "$i" -lt "$n_correct" ]; then
      target_text="${CORRECT_TEXT[$i]}"
    else
      target_text=$(echo "$attempt" | jq -r --argjson i "$i" --arg c "${CORRECT_TEXT[$i]}" \
        '.questions[$i].options[] | select(.text != $c) | .text' | head -1)
    fi
    option_stable=$(echo "$attempt" | jq -r --argjson i "$i" --arg t "$target_text" \
      '.questions[$i].options[] | select(.text == $t) | .stable_id')
    answers=$(echo "$answers" | jq --arg q "$question_stable" --arg o "$option_stable" '. + {($q): [$o]}')
  done

  curl -sS -X POST "$API/attempts/$attempt_id/submit" \
    -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
    -H "Idempotency-Key: $idem_key" \
    -d "{\"answers\":$answers}" | jq -c '{score,passed,progress}'
}

say "18. Intento 1: 3 de 5 correctas (60%, se espera passed=false)"
answer_and_submit 3 "quiz-fail-$STAMP" true

say "19. Intento 2: 5 de 5 correctas (100%, se espera passed=true, progress.state=approved)"
answer_and_submit 5 "quiz-pass-$STAMP"

# =====================================================================
# 9. Insignia: con TODO el material consumido y el quiz aprobado, la
#    inscripcion queda 'approved' y el worker emite la insignia.
# =====================================================================
say "20. Insignia (la emite el worker de forma asincrona)"
sleep 3
curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_TOKEN" | jq -c '.data'
CODE=$(curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_TOKEN" | jq -r '.data[0].public_code // empty')
if [ -n "$CODE" ]; then
  echo "Verificacion publica (sin sesion, sin correo expuesto):"
  curl -sS "$API/public/badges/$CODE" | jq -c .
else
  echo "ADVERTENCIA: no aparecio insignia todavia. Revisa 'docker compose logs worker'." >&2
fi

say "Prueba de humo terminada"
