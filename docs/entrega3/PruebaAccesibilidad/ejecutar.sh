#!/usr/bin/env bash
# Auditoria automatica de accesibilidad (WCAG 2.2 AA) con Playwright + axe-core.
#
# Todo corre dentro de la imagen oficial de Playwright, conectada a la red de
# Docker Compose: no se instala nada en la maquina ni se toca la API. Los
# resultados quedan en resultados/ junto a este script.
#
# Variables opcionales (si no se dan, se buscan solas):
#   ESTUDIANTE_EMAIL / ESTUDIANTE_PASSWORD   por defecto, el ultimo estudiante
#                                            de smoke.sh con una inscripcion activa
#   PROFESOR_EMAIL / PROFESOR_PASSWORD       por defecto, el ultimo profesor de
#                                            smoke.sh que tenga cursos
#   ADMIN_EMAIL / ADMIN_PASSWORD             por defecto, los del .env de la raiz
#   RED                                      red de Compose (mooc-platform_default)
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RAIZ="$(cd "$DIR/../../.." && pwd)"
IMAGEN="mcr.microsoft.com/playwright:v1.49.1-noble"
RED="${RED:-mooc-platform_default}"

consulta() {
  docker compose -f "$RAIZ/docker-compose.yml" exec -T postgres \
    psql -U mooc -d mooc -tA -c "$1" 2>/dev/null | head -n1 || true
}

# Valor de una variable del .env de la raiz (sin ejecutarlo como script).
del_env() {
  [ -f "$RAIZ/.env" ] && grep -E "^$1=" "$RAIZ/.env" | tail -n1 | cut -d= -f2- | tr -d '"'"'" || true
}

if ! docker network inspect "$RED" >/dev/null 2>&1; then
  echo "No existe la red '$RED'. Levanta el proyecto con 'make up' antes de auditar." >&2
  exit 2
fi

# smoke.sh crea estos usuarios con contrasenas fijas (docs/entrega1/CREDENCIALES.md).
ESTUDIANTE_EMAIL="${ESTUDIANTE_EMAIL:-$(consulta "
  SELECT u.email FROM users u JOIN enrollments e ON e.user_id = u.id AND e.status = 'active'
  WHERE u.email LIKE 'estudiante%@mooc.local' AND u.status = 'active'
  ORDER BY e.enrolled_at DESC LIMIT 1")}"
ESTUDIANTE_PASSWORD="${ESTUDIANTE_PASSWORD:-Estudiante2026}"
PROFESOR_EMAIL="${PROFESOR_EMAIL:-$(consulta "
  SELECT u.email FROM users u
  WHERE u.email LIKE 'profe%@mooc.local' AND u.status = 'active'
    AND EXISTS (SELECT 1 FROM courses c WHERE c.owner_id = u.id)
  ORDER BY u.created_at DESC LIMIT 1")}"
PROFESOR_PASSWORD="${PROFESOR_PASSWORD:-Profesor2026}"
ADMIN_EMAIL="${ADMIN_EMAIL:-$(del_env ADMIN_EMAIL)}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@mooc.local}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-$(del_env ADMIN_PASSWORD)}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Admin123!}"
# `docker run -e VAR` solo copia variables exportadas: sin esto el contenedor
# las recibe vacias y omite todos los roles con sesion.
export ESTUDIANTE_EMAIL ESTUDIANTE_PASSWORD PROFESOR_EMAIL PROFESOR_PASSWORD ADMIN_EMAIL ADMIN_PASSWORD

echo "Estudiante: ${ESTUDIANTE_EMAIL:-(ninguno, se omiten sus paginas)}"
echo "Profesor:   ${PROFESOR_EMAIL:-(ninguno, se omiten sus paginas)}"
echo "Admin:      $ADMIN_EMAIL"
echo

mkdir -p "$DIR/resultados"

# Las dependencias se instalan en /tmp del contenedor (no en el repo) y el
# contenedor corre con el usuario del host para que los resultados no queden
# a nombre de root.
docker run --rm \
  --network "$RED" \
  --user "$(id -u):$(id -g)" \
  -e HOME=/tmp \
  -e ESTUDIANTE_EMAIL -e ESTUDIANTE_PASSWORD \
  -e PROFESOR_EMAIL -e PROFESOR_PASSWORD \
  -e ADMIN_EMAIL -e ADMIN_PASSWORD \
  -e DESTINO="${DESTINO:-web:3000}" \
  -e SALIDA=/resultados \
  -v "$DIR/prueba:/prueba:ro" \
  -v "$DIR/resultados:/resultados" \
  "$IMAGEN" \
  bash -c 'set -e
    cp /prueba/package.json /prueba/auditoria.mjs /tmp/
    cd /tmp
    npm install --no-audit --no-fund --loglevel=error
    node auditoria.mjs'
