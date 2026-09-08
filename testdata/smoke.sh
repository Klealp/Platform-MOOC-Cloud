#!/usr/bin/env bash
# =====================================================================
# Prueba de humo del flujo completo. Requiere curl y jq.
#
#   chmod +x testdata/smoke.sh
#   ./testdata/smoke.sh
#
# Recorre: admin -> crear profesor -> curso -> modulo -> unidad ->
# recurso de texto -> quiz con una pregunta -> publicar -> registrar
# estudiante -> inscribir -> eventos de progreso -> presentar el quiz ->
# insignia. Sirve para comprobar que el despliegue quedo operativo.
# =====================================================================
set -euo pipefail

API="${API:-http://localhost:8080/api/v1}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
STAMP="$(date +%s)"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$1"; }

say "1. Login del administrador"
ADMIN_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" | jq -r .access_token)
echo "token admin: ${ADMIN_TOKEN:0:12}..."

say "2. El administrador crea un profesor (unica via posible)"
TEACHER_EMAIL="profe$STAMP@mooc.local"
curl -sS -X POST "$API/admin/users" \
  -H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"full_name\":\"Profesora Demo\",\"password\":\"Profesor2026\",\"role\":\"teacher\"}" | jq -c .

TEACHER_TOKEN=$(curl -sS -X POST "$API/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$TEACHER_EMAIL\",\"password\":\"Profesor2026\"}" | jq -r .access_token)

say "3. Crear curso, modulo, unidad y recursos"
COURSE=$(curl -sS -X POST "$API/courses" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Fundamentos de la nube","summary":"Curso de prueba","category":"cloud","level":"beginner"}')
COURSE_ID=$(echo "$COURSE" | jq -r .id)
SLUG=$(echo "$COURSE" | jq -r .slug)
VERSION_ID=$(echo "$COURSE" | jq -r .draft_version.id)
echo "curso $SLUG ($COURSE_ID) version $VERSION_ID"

MODULE_ID=$(curl -sS -X POST "$API/versions/$VERSION_ID/modules" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Modulo 1"}' | jq -r .id)

UNIT_ID=$(curl -sS -X POST "$API/modules/$MODULE_ID/units" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"Unidad 1"}' | jq -r .id)

TEXT_RES=$(curl -sS -X POST "$API/units/$UNIT_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"rich_text","title":"Lectura inicial","content_md":"# Hola\n\nContenido de prueba.","required":true}')
TEXT_ID=$(echo "$TEXT_RES" | jq -r .id)

QUIZ_RES=$(curl -sS -X POST "$API/units/$UNIT_ID/resources" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"type":"quiz","title":"Quiz de cierre","required":true}')
QUIZ_ID=$(echo "$QUIZ_RES" | jq -r .quiz_id)

curl -sS -X POST "$API/quizzes/$QUIZ_ID/questions" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' \
  -d '{"prompt":"Que significa que la API sea sin estado?","options":[
        {"text":"Que no guarda datos en el contenedor entre peticiones","is_correct":true},
        {"text":"Que no usa base de datos","is_correct":false},
        {"text":"Que no responde nada","is_correct":false}]}' | jq -c .

say "4. Previsualizacion: la version deberia ser publicable"
curl -sS "$API/versions/$VERSION_ID" -H "Authorization: Bearer $TEACHER_TOKEN" \
  | jq '{publishable, problems}'

say "5. Publicar"
curl -sS -X POST "$API/versions/$VERSION_ID/publish" \
  -H "Authorization: Bearer $TEACHER_TOKEN" -H 'Content-Type: application/json' -d '{}' | jq -c .

say "6. Registro y verificacion de un estudiante"
STUDENT_EMAIL="estudiante$STAMP@mooc.local"
REG=$(curl -sS -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"full_name\":\"Estudiante Demo\",\"password\":\"Estudiante2026\"}")
VERIFY_TOKEN=$(echo "$REG" | jq -r .dev_verification_token)

curl -sS -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$VERIFY_TOKEN\"}" | jq -c .

STUDENT_TOKEN=$(curl -sS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$STUDENT_EMAIL\",\"password\":\"Estudiante2026\"}" | jq -r .access_token)

say "7. Catalogo e inscripcion"
curl -sS "$API/catalog?q=nube" | jq -c '.data[0] | {slug,title}'
ENROLLMENT_ID=$(curl -sS -X POST "$API/enrollments" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"slug\":\"$SLUG\"}" | jq -r .id)

OUTLINE=$(curl -sS "$API/enrollments/$ENROLLMENT_ID/outline" -H "Authorization: Bearer $STUDENT_TOKEN")
TEXT_STABLE=$(echo "$OUTLINE" | jq -r '.modules[0].units[0].resources[] | select(.type=="rich_text") | .stable_id')
STUDENT_QUIZ_ID=$(echo "$OUTLINE" | jq -r '.modules[0].units[0].resources[] | select(.type=="quiz") | .quiz_id')

say "8. El servidor rechaza que el cliente dicte el porcentaje"
curl -sS -o /dev/null -w 'HTTP %{http_code} (se espera 422)\n' -X POST "$API/progress/events" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -d "{\"enrollment_id\":\"$ENROLLMENT_ID\",\"resource_stable_id\":\"$TEXT_STABLE\",\"event_type\":\"heartbeat\",\"progress_pct\":100}"

say "9. Senales legitimas de permanencia"
curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $STUDENT_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"enrollment_id\":\"$ENROLLMENT_ID\",\"resource_stable_id\":\"$TEXT_STABLE\",\"event_type\":\"open\"}" >/dev/null
curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $STUDENT_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"enrollment_id\":\"$ENROLLMENT_ID\",\"resource_stable_id\":\"$TEXT_STABLE\",\"event_type\":\"heartbeat\",\"delta_secs\":30}" >/dev/null
curl -sS -X POST "$API/progress/events" -H "Authorization: Bearer $STUDENT_TOKEN" \
  -H 'Content-Type: application/json' \
  -d "{\"enrollment_id\":\"$ENROLLMENT_ID\",\"resource_stable_id\":\"$TEXT_STABLE\",\"event_type\":\"complete\"}" \
  | jq -c '.progress'

say "10. Presentar el quiz (la clave correcta nunca llega al cliente)"
ATTEMPT=$(curl -sS -X POST "$API/quizzes/$STUDENT_QUIZ_ID/attempts" -H "Authorization: Bearer $STUDENT_TOKEN")
ATTEMPT_ID=$(echo "$ATTEMPT" | jq -r .id)
echo "El cuestionario recibido NO trae is_correct:"
echo "$ATTEMPT" | jq -c '.questions[0].options'

Q_STABLE=$(echo "$ATTEMPT" | jq -r '.questions[0].stable_id')
O_STABLE=$(echo "$ATTEMPT" | jq -r '.questions[0].options[0].stable_id')

IDEM="smoke-$STAMP"
curl -sS -X POST "$API/attempts/$ATTEMPT_ID/submit" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $IDEM" \
  -d "{\"answers\":{\"$Q_STABLE\":[\"$O_STABLE\"]}}" | jq -c '{score,passed,progress}'

say "11. Reenvio con la MISMA Idempotency-Key (no recalifica)"
curl -sS -D- -o /dev/null -X POST "$API/attempts/$ATTEMPT_ID/submit" \
  -H "Authorization: Bearer $STUDENT_TOKEN" -H 'Content-Type: application/json' \
  -H "Idempotency-Key: $IDEM" \
  -d "{\"answers\":{\"$Q_STABLE\":[\"$O_STABLE\"]}}" 2>/dev/null | grep -i 'idempotent-replay' || echo "(respuesta repetida)"

say "12. Insignia (la emite el worker de forma asincrona)"
sleep 3
curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_TOKEN" | jq -c '.data'
CODE=$(curl -sS "$API/badges" -H "Authorization: Bearer $STUDENT_TOKEN" | jq -r '.data[0].public_code // empty')
if [ -n "$CODE" ]; then
  echo "Verificacion publica (sin sesion, sin correo expuesto):"
  curl -sS "$API/public/badges/$CODE" | jq -c .
fi

say "13. Aislamiento: otro estudiante no puede ver esta inscripcion"
OTHER_EMAIL="intruso$STAMP@mooc.local"
OTHER_REG=$(curl -sS -X POST "$API/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$OTHER_EMAIL\",\"full_name\":\"Intruso\",\"password\":\"Intruso2026\"}")
curl -sS -X POST "$API/auth/verify-email" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$(echo "$OTHER_REG" | jq -r .dev_verification_token)\"}" >/dev/null
OTHER_TOKEN=$(curl -sS -X POST "$API/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$OTHER_EMAIL\",\"password\":\"Intruso2026\"}" | jq -r .access_token)

curl -sS -o /dev/null -w 'HTTP %{http_code} (se espera 404)\n' \
  "$API/enrollments/$ENROLLMENT_ID/outline" -H "Authorization: Bearer $OTHER_TOKEN"

say "Prueba de humo terminada"
