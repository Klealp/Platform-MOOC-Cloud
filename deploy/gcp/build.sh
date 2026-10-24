#!/usr/bin/env bash
# =====================================================================
# Construye las imagenes de la plataforma y las sube a Artifact Registry
# (Entrega 3).
#
# Las VMs de los grupos autoescalados NO compilan: descargan una imagen ya
# construida. Asi cada instancia nueva arranca en segundos y todas las
# replicas de un grupo corren exactamente el mismo codigo. La etiqueta de la
# imagen es el SHA corto del commit, de modo que la version desplegada es
# trazable al repositorio.
#
# Uso (desde la raiz del repo o desde cualquier carpeta):
#   export PROJECT=dsc-uniandes-20262
#   ./deploy/gcp/build.sh                 # api, worker, web y queue-metrics
#   ./deploy/gcp/build.sh api worker      # solo algunas
#
# Variables opcionales:
#   REGION=us-central1   region del repositorio (la misma de las VMs)
#   NO_PUSH=1            construye y etiqueta, pero no sube (prueba local)
#   ALLOW_DIRTY=1        permite construir con cambios sin commit; la
#                        etiqueta lleva el sufijo -dirty para no confundirla
#                        con una version reproducible
#
# Requisito previo: ./deploy/gcp/provision.sh registro (crea el repositorio).
# =====================================================================
set -euo pipefail

PROJECT="${PROJECT:?exporta PROJECT con el id del proyecto}"
REGION="${REGION:-us-central1}"
REPO="mooc"
REGISTRY="$REGION-docker.pkg.dev/$PROJECT/$REPO"

RAIZ="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$RAIZ"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }

# La etiqueta tiene que identificar el codigo exacto. Con cambios sin commit
# el SHA mentiria, asi que se rechaza salvo que se pida explicitamente.
VERSION="$(git rev-parse --short=7 HEAD)"
if [ -n "$(git status --porcelain)" ]; then
  if [ "${ALLOW_DIRTY:-}" != "1" ]; then
    echo "Hay cambios sin commit: la imagen no corresponderia a $VERSION." >&2
    echo "Haz commit, o usa ALLOW_DIRTY=1 para una prueba (etiqueta $VERSION-dirty)." >&2
    exit 1
  fi
  VERSION="$VERSION-dirty"
fi

# componente -> Dockerfile. El contexto siempre es la raiz del repo.
# queue-metrics corre en infra-server y publica la metrica del autoescalador
# de workers.
declare -A DOCKERFILE=([api]=Dockerfile.api [worker]=Dockerfile.worker [web]=Dockerfile.web
  [queue-metrics]=Dockerfile.queue-metrics)

if [ "$#" -eq 0 ]; then
  set -- api worker web queue-metrics
fi
for comp in "$@"; do
  [ -n "${DOCKERFILE[$comp]:-}" ] || { echo "Componente desconocido: $comp (api|worker|web|queue-metrics)" >&2; exit 1; }
done

if [ "${NO_PUSH:-}" != "1" ]; then
  say "Autenticando Docker contra $REGION-docker.pkg.dev"
  gcloud auth configure-docker "$REGION-docker.pkg.dev" --quiet >/dev/null
fi

for comp in "$@"; do
  img="$REGISTRY/$comp:$VERSION"
  say "Construyendo $img"
  # linux/amd64: las VMs de GCE son x86 aunque se construya desde un Mac ARM.
  # La etiqueta OCI deja la revision dentro de la imagen (docker inspect).
  docker build --platform linux/amd64 -f "${DOCKERFILE[$comp]}" \
    --label "org.opencontainers.image.revision=$(git rev-parse HEAD)" \
    --label "org.opencontainers.image.source=$(git remote get-url origin 2>/dev/null || echo local)" \
    -t "$img" .
  if [ "${NO_PUSH:-}" != "1" ]; then
    docker push "$img"
  fi
done

say "Listo"
echo "Version: $VERSION"
for comp in "$@"; do echo "  $REGISTRY/$comp:$VERSION"; done
[ "${NO_PUSH:-}" = "1" ] && echo "(NO_PUSH=1: las imagenes quedaron solo en este Docker local)"
exit 0
