#!/usr/bin/env bash
# =====================================================================
# Escenario 2 - Paso 2: preparar los datos sinteticos (FUERA de la medicion)
#
# Deja la plataforma lista para la prueba de carga:
#
#   a) Contenido HLS "ya disponible" para los estudiantes:
#      un profesor de contenido crea el curso "Escenario 2 - Contenido HLS"
#      con una unidad por perfil (P1, P2, P3, A1), sube los 4 archivos por
#      carga multipart DIRECTA al almacenamiento, espera a que el worker los
#      deje en estado ready, valida las rendiciones y publica el curso.
#   b) PROFESORES profesores de carga, cada uno con su curso borrador y una
#      unidad donde k6 creara los recursos de los archivos que suba.
#   c) ESTUDIANTES estudiantes (creados por el admin, ya verificados)
#      inscritos en el curso de contenido.
#
# Todo queda en capacity-planning/escenario2/datos/seed.json, que lee k6.
# OJO: seed.json contiene tokens de sesion (validos 24 h). No se versiona
# (.gitignore) y si pasan mas de 24 h hay que volver a correr este script.
#
# Uso (desde la raiz del repo, con la plataforma arriba):
#   ./capacity-planning/escenario2/preparar_datos.sh
#   API_URL=https://mi-dominio ./capacity-planning/escenario2/preparar_datos.sh   # en la nube
#
# Variables opcionales: PROFESORES (20), ESTUDIANTES (40), ADMIN_EMAIL,
# ADMIN_PASSWORD, TIMEOUT_READY_S (1800).
# Requiere: curl, jq, dd. Antes: generar_perfiles.sh
# =====================================================================
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MEDIA="$DIR/media"
DATOS="$DIR/datos"
API_URL="${API_URL:-http://localhost:8080}"
API="$API_URL/api/v1"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
PROFESORES="${PROFESORES:-20}"
ESTUDIANTES="${ESTUDIANTES:-40}"
TIMEOUT_READY_S="${TIMEOUT_READY_S:-1800}"
PASS_PROFE="Profesor2026"
PASS_EST="Estudiante2026"
STAMP="$(date +%s)"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }
die() { echo "ERROR: $*" >&2; exit 1; }

[ -f "$MEDIA/perfiles.json" ] || die "No existe $MEDIA/perfiles.json. Corre primero generar_perfiles.sh"
mkdir -p "$DATOS"

# ---------------------------------------------------------------------
# call METODO RUTA TOKEN [JSON]
# Imprime el cuerpo de la respuesta. Si la API responde 429 (limite de
# tasa: 30/min por IP en login y 120/min por usuario en el resto) espera y
# reintenta: en la PREPARACION no queremos medir nada, solo terminar.
# Cualquier otro codigo >= 400 detiene el script mostrando el error.
# ---------------------------------------------------------------------
call() {
  local method="$1" path="$2" token="${3:-}" body="${4:-}" out code tries=0
  local args=(-sS -X "$method" "$API$path" -H 'Content-Type: application/json' -w '\n%{http_code}')
  [ -n "$token" ] && args+=(-H "Authorization: Bearer $token")
  [ -n "$body" ] && args+=(-d "$body")
  while :; do
    out=$(curl "${args[@]}")
    code="${out##*$'\n'}"
    out="${out%$'\n'*}"
    if [ "$code" = "429" ] && [ $tries -lt 20 ]; then
      tries=$((tries + 1))
      printf '   (limite de tasa alcanzado, espero 10 s... intento %d)\n' "$tries" >&2
      sleep 10
      continue
    fi
    if [ "$code" -ge 400 ]; then
      echo "HTTP $code en $method $path" >&2
      echo "$out" >&2
      exit 1
    fi
    printf '%s' "$out"
    return 0
  done
}

login() { call POST /auth/login "" "{\"email\":\"$1\",\"password\":\"$2\"}" | jq -r .access_token; }

# ---------------------------------------------------------------------
# subir_perfil TOKEN UNIT_ID PERFIL_ID -> imprime el asset_id
# Carga multipart real: POST /uploads, PUT de cada parte a la URL
# prefirmada (directo al almacenamiento) y POST /uploads/{id}/complete.
# Las partes se cortan con dd (lee solo el pedazo necesario).
# ---------------------------------------------------------------------
subir_perfil() {
  local token="$1" unit="$2" pid="$3"
  local p file size kind mime sha init upload asset part_size total url tmp code
  p=$(jq -c --arg id "$pid" '.perfiles[] | select(.id==$id)' "$MEDIA/perfiles.json")
  file="$MEDIA/$(echo "$p" | jq -r .archivo)"
  size=$(echo "$p" | jq -r .tamano_bytes); kind=$(echo "$p" | jq -r .kind)
  mime=$(echo "$p" | jq -r .mime);          sha=$(echo "$p" | jq -r .sha256)
  [ -f "$file" ] || die "No existe $file"

  init=$(call POST /uploads "$token" "{\"original_name\":\"$(basename "$file")\",\"size_bytes\":$size,
          \"kind\":\"$kind\",\"declared_mime\":\"$mime\",\"checksum_sha256\":\"$sha\"}")
  upload=$(echo "$init" | jq -r .upload_id); asset=$(echo "$init" | jq -r .asset_id)
  part_size=$(echo "$init" | jq -r .part_size); total=$(echo "$init" | jq -r .total_parts)
  tmp=$(mktemp)
  for i in $(seq 1 "$total"); do
    url=$(echo "$init" | jq -r ".parts[] | select(.part_number==$i) | .url")
    dd if="$file" of="$tmp" bs="$part_size" skip=$((i - 1)) count=1 iflag=fullblock status=none
    code=$(curl -sS -o /dev/null -w '%{http_code}' -X PUT --data-binary @"$tmp" "$url")
    [ "$code" = "200" ] || die "parte $i/$total de $pid respondio HTTP $code"
  done
  rm -f "$tmp"
  call POST "/uploads/$upload/complete" "$token" >/dev/null
  local rtype="video"; [ "$kind" = "audio" ] && rtype="audio"
  call POST "/units/$unit/resources" "$token" \
    "{\"type\":\"$rtype\",\"title\":\"Contenido $pid\",\"required\":true,\"asset_id\":\"$asset\"}" >/dev/null
  printf '%s' "$asset"
}

# =====================================================================
say "0. Salud de la API ($API_URL)"
curl -fsS "$API_URL/readyz" | jq -c . || die "La API no esta lista en $API_URL"

say "1. Login del administrador"
ADMIN_TOKEN=$(login "$ADMIN_EMAIL" "$ADMIN_PASSWORD")
[ -n "$ADMIN_TOKEN" ] && [ "$ADMIN_TOKEN" != "null" ] || die "login de admin fallido"

crear_usuario() { # email nombre password rol
  call POST /admin/users "$ADMIN_TOKEN" \
    "{\"email\":\"$1\",\"full_name\":\"$2\",\"password\":\"$3\",\"role\":\"$4\"}" >/dev/null
}

# =====================================================================
# a) Contenido HLS ya disponible
# =====================================================================
say "2. Profesor de contenido y curso con los 4 perfiles"
CONTENT_EMAIL="e2contenido$STAMP@mooc.local"
crear_usuario "$CONTENT_EMAIL" "Profesor Contenido E2" "$PASS_PROFE" teacher
CONTENT_TOKEN=$(login "$CONTENT_EMAIL" "$PASS_PROFE")
COURSE=$(call POST /courses "$CONTENT_TOKEN" \
  "{\"title\":\"Escenario 2 - Contenido HLS $STAMP\",\"summary\":\"Contenido multimedia para la prueba de consumo\",\"category\":\"cloud\",\"level\":\"beginner\"}")
COURSE_ID=$(echo "$COURSE" | jq -r .id); SLUG=$(echo "$COURSE" | jq -r .slug)
VERSION_ID=$(echo "$COURSE" | jq -r .draft_version.id)
MODULE_ID=$(call POST "/versions/$VERSION_ID/modules" "$CONTENT_TOKEN" '{"title":"Multimedia"}' | jq -r .id)
echo "curso: $SLUG"

declare -A ASSET
for pid in $(jq -r '.perfiles[].id' "$MEDIA/perfiles.json"); do
  UNIT_ID=$(call POST "/modules/$MODULE_ID/units" "$CONTENT_TOKEN" "{\"title\":\"Perfil $pid\"}" | jq -r .id)
  ASSET[$pid]=$(subir_perfil "$CONTENT_TOKEN" "$UNIT_ID" "$pid")
  echo "  $pid subido -> asset ${ASSET[$pid]}"
done

say "3. Esperando a que el worker deje los 4 assets en ready (escaneo + HLS)"
START=$(date +%s)
while :; do
  PEND=0; LINE=""
  for pid in "${!ASSET[@]}"; do
    ST=$(call GET "/assets/${ASSET[$pid]}" "$CONTENT_TOKEN" | jq -r .status)
    LINE+="$pid=$ST "
    case "$ST" in
      ready) ;;
      failed|infected) die "el asset de $pid termino en estado $ST" ;;
      *) PEND=$((PEND + 1)) ;;
    esac
  done
  echo "  [$(( $(date +%s) - START )) s] $LINE"
  [ "$PEND" -eq 0 ] && break
  [ $(( $(date +%s) - START )) -gt "$TIMEOUT_READY_S" ] && die "timeout esperando ready"
  sleep 10
done

say "4. Publicar el curso de contenido"
call POST "/versions/$VERSION_ID/publish" "$CONTENT_TOKEN" '{}' >/dev/null
echo "publicado: $SLUG"

# =====================================================================
# b) Profesores de carga
# =====================================================================
say "5. Crear $PROFESORES profesores de carga (curso borrador + unidad cada uno)"
PROFES_JSON="[]"
for i in $(seq 1 "$PROFESORES"); do
  EMAIL="e2profe${i}_$STAMP@mooc.local"
  crear_usuario "$EMAIL" "Profesor Carga $i" "$PASS_PROFE" teacher
  TK=$(login "$EMAIL" "$PASS_PROFE")
  C=$(call POST /courses "$TK" "{\"title\":\"E2 carga profesor $i $STAMP\",\"summary\":\"Destino de cargas de la prueba\",\"category\":\"cloud\",\"level\":\"beginner\"}")
  V=$(echo "$C" | jq -r .draft_version.id)
  M=$(call POST "/versions/$V/modules" "$TK" '{"title":"Cargas"}' | jq -r .id)
  U=$(call POST "/modules/$M/units" "$TK" '{"title":"Recursos subidos por k6"}' | jq -r .id)
  PROFES_JSON=$(echo "$PROFES_JSON" | jq --arg e "$EMAIL" --arg t "$TK" --arg u "$U" --arg c "$(echo "$C" | jq -r .id)" \
    '. + [{email:$e, token:$t, course_id:$c, unit_id:$u}]')
  printf '  profesor %d/%d listo\n' "$i" "$PROFESORES"
done

# =====================================================================
# c) Estudiantes inscritos en el curso de contenido
# =====================================================================
say "6. Crear $ESTUDIANTES estudiantes e inscribirlos en $SLUG"
EST_JSON="[]"
for i in $(seq 1 "$ESTUDIANTES"); do
  EMAIL="e2est${i}_$STAMP@mooc.local"
  crear_usuario "$EMAIL" "Estudiante Carga $i" "$PASS_EST" student
  TK=$(login "$EMAIL" "$PASS_EST")
  ENR=$(call POST /enrollments "$TK" "{\"slug\":\"$SLUG\"}" | jq -r .id)
  EST_JSON=$(echo "$EST_JSON" | jq --arg e "$EMAIL" --arg t "$TK" --arg en "$ENR" \
    '. + [{email:$e, token:$t, enrollment_id:$en}]')
  printf '  estudiante %d/%d listo\n' "$i" "$ESTUDIANTES"
done

# =====================================================================
say "7. Validar el contenido HLS desde el punto de vista de un estudiante"
TK_EST=$(echo "$EST_JSON" | jq -r '.[0].token')
CONTENIDO="[]"
for pid in $(jq -r '.perfiles[].id' "$MEDIA/perfiles.json"); do
  A="${ASSET[$pid]}"
  INFO=$(call GET "/assets/$A" "$TK_EST")
  MASTER=$(curl -sS "$API/assets/$A/playlist" -H "Authorization: Bearer $TK_EST")
  VARIANTES=$(echo "$MASTER" | grep -c 'playlist?file=' || true)
  ESPERADAS=$(jq --arg id "$pid" '.perfiles[] | select(.id==$id) | .rendiciones_esperadas | length' "$MEDIA/perfiles.json")
  [ "$VARIANTES" = "$ESPERADAS" ] || die "$pid: el master tiene $VARIANTES variantes y se esperaban $ESPERADAS"
  echo "  $pid ok: $VARIANTES variantes, duracion $(echo "$INFO" | jq -r .duration_secs)s"
  CONTENIDO=$(echo "$CONTENIDO" | jq --arg id "$pid" --arg a "$A" --argjson d "$(echo "$INFO" | jq .duration_secs)" \
    --argjson n "$VARIANTES" '. + [{perfil:$id, asset_id:$a, duracion_s:$d, variantes:$n}]')
done

jq -n --arg api "$API_URL" --arg fecha "$(date -Iseconds)" --arg slug "$SLUG" --arg cid "$COURSE_ID" \
  --argjson contenido "$CONTENIDO" --argjson profes "$PROFES_JSON" --argjson est "$EST_JSON" \
  '{api_url:$api, generado:$fecha, advertencia:"Contiene tokens de sesion (24 h). NO versionar.",
    curso_contenido:{id:$cid, slug:$slug, assets:$contenido},
    profesores:$profes, estudiantes:$est}' > "$DATOS/seed.json"
chmod 600 "$DATOS/seed.json"

echo
echo "Listo. Datos de la prueba en: $DATOS/seed.json"
echo "  curso de contenido: $SLUG (4 assets ready)"
echo "  profesores de carga: $PROFESORES   estudiantes inscritos: $ESTUDIANTES"
