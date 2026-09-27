#!/usr/bin/env bash
# =====================================================================
# Escenario 2 - Compila las dos herramientas en Go:
#   bin/monitorcola  (observa la cola asynq en Redis)
#   bin/analizador   (genera tablas y graficas a partir de los resultados)
#
# Produce binarios ESTATICOS para Linux amd64, asi el mismo archivo sirve en
# WSL y en las VMs de GCP (se copian con scp / gcloud compute scp).
#
# Usa Go si esta instalado en WSL; si no, compila dentro de un contenedor
# golang (igual que los Dockerfile del proyecto), sin instalar nada.
#
# Uso (desde cualquier carpeta):
#   ./capacity-planning/escenario2/construir_herramientas.sh
# =====================================================================
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$DIR/../.." && pwd)"
REL="capacity-planning/escenario2"
mkdir -p "$DIR/bin"

if command -v go >/dev/null 2>&1; then
  echo "Compilando con $(go version)"
  cd "$REPO"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$REL/bin/monitorcola" "./$REL/herramientas/monitorcola"
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$REL/bin/analizador" "./$REL/herramientas/analizador"
else
  echo "Go no esta instalado en esta terminal: compilo dentro de un contenedor golang:1.25-alpine"
  docker run --rm -u "$(id -u):$(id -g)" -e HOME=/tmp -e GOCACHE=/tmp/gocache -e GOMODCACHE=/tmp/gomod \
    -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=amd64 -v "$REPO":/src -w /src golang:1.25-alpine \
    sh -c "go build -o $REL/bin/monitorcola ./$REL/herramientas/monitorcola && go build -o $REL/bin/analizador ./$REL/herramientas/analizador"
fi
ls -la "$DIR/bin"
echo "Listo: $DIR/bin/monitorcola y $DIR/bin/analizador"
