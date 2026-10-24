#!/bin/bash
# =====================================================================
# Script de arranque de las VMs de los grupos administrados (Entrega 3).
#
# NADIE lo ejecuta a mano. La plantilla de instancia lo adjunta como
# metadato "startup-script" y Google lo ejecuta como root en CADA arranque
# de cada VM que crea el grupo. Al terminar, la VM queda atendiendo
# trafico (api, web) o consumiendo la cola (worker) sin acceso manual.
#
# Todo lo que cambia entre grupos llega por metadatos de la plantilla:
#   mooc-rol       api | worker | web
#   mooc-version   etiqueta de la imagen (SHA del commit, ver build.sh)
#   mooc-compose   contenido de deploy/gcp/mig/<rol>/docker-compose.yml
#   mooc-registry  opcional; por defecto <region>-docker.pkg.dev/<proyecto>/mooc
#
# Y la configuracion sensible, de Secret Manager: el secreto mooc-<rol>-env
# se escribe en /etc/mooc/<rol>.env (root, 600). Nunca viaja en la plantilla.
#
# Es idempotente: en un reinicio vuelve a leer secretos y metadatos y
# "compose up -d" no duplica contenedores. La salida queda en Cloud Logging
# (google_metadata_script_runner) y en /var/log/mooc-startup.log.
# =====================================================================
set -euo pipefail

exec > >(tee -a /var/log/mooc-startup.log) 2>&1
# Un solo arranque a la vez, por si el agente de metadatos lo relanza.
exec 9>/run/mooc-startup.lock
flock -n 9 || { echo "otro startup.sh en curso; se omite"; exit 0; }

log() { echo "[mooc-startup $(date -u +%H:%M:%S) t=$(cut -d' ' -f1 /proc/uptime)s] $*"; }

MD=http://metadata.google.internal/computeMetadata/v1
md()  { curl -fsS -H 'Metadata-Flavor: Google' "$MD/$1"; }
# atributo opcional: vacio si no existe
mdo() { curl -fsS -H 'Metadata-Flavor: Google' "$MD/instance/attributes/$1" 2>/dev/null || true; }

# reintentar <intentos> <comando...>: la red y los servicios de Google pueden
# tardar unos segundos en responder justo despues del arranque.
reintentar() {
  local n="$1" i; shift
  for ((i = 1; i <= n; i++)); do
    "$@" && return 0
    log "fallo ($i/$n): $*"; sleep $((i * 3))
  done
  return 1
}

ROL="$(md instance/attributes/mooc-rol)"
VERSION="$(md instance/attributes/mooc-version)"
PROYECTO="$(md project/project-id)"
ZONA="$(md instance/zone)"; ZONA="${ZONA##*/}"
REGION="${ZONA%-*}"
REGISTRY="$(mdo mooc-registry)"; REGISTRY="${REGISTRY:-$REGION-docker.pkg.dev/$PROYECTO/mooc}"
INSTANCIA="$(md instance/name)"

case "$ROL" in
  api)    SALUD=http://127.0.0.1:8080/healthz ;;
  worker) SALUD=http://127.0.0.1:9100/healthz ;;
  web)    SALUD=http://127.0.0.1:3000/ ;;
  *) log "mooc-rol invalido: '$ROL'"; exit 1 ;;
esac
log "instancia=$INSTANCIA rol=$ROL version=$VERSION registro=$REGISTRY"

# --- 1. Docker y Ops Agent (solo la primera vez) ---------------------
# Las VMs no tienen IP publica: descargan esto por Cloud NAT. Una imagen de
# disco personalizada con ambos instalados recortaria este paso del arranque.
if ! command -v docker >/dev/null 2>&1; then
  log "instalando Docker"
  reintentar 5 sh -c 'curl -fsSL https://get.docker.com | sh'
fi
# Ops Agent: CPU, memoria y disco por instancia en Cloud Monitoring. El
# analisis de capacidad los exige y las metricas por defecto no traen memoria.
if ! systemctl is-active --quiet google-cloud-ops-agent; then
  log "instalando Ops Agent"
  reintentar 5 curl -sSfo /tmp/add-ops-agent.sh https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh
  bash /tmp/add-ops-agent.sh --also-install
fi
command -v gcloud >/dev/null 2>&1 || { log "falta gcloud: usar una imagen de debian-cloud"; exit 1; }
log "dependencias listas"

# --- 2. Configuracion sensible desde Secret Manager -------------------
# La identidad de la VM necesita roles/secretmanager.secretAccessor SOLO
# sobre mooc-<rol>-env. Se relee en cada arranque: rotar un secreto es
# publicar una version nueva y reiniciar (o recrear) las instancias.
install -d -m 700 /etc/mooc /opt/mooc
umask 077
reintentar 5 gcloud secrets versions access latest --secret="mooc-$ROL-env" \
  --project="$PROYECTO" > "/etc/mooc/$ROL.env.tmp"
mv "/etc/mooc/$ROL.env.tmp" "/etc/mooc/$ROL.env"
umask 022
log "configuracion de mooc-$ROL-env escrita"

# --- 3. Definicion de contenedores (viene de la plantilla) ------------
mdo mooc-compose > /opt/mooc/docker-compose.yml
[ -s /opt/mooc/docker-compose.yml ] || { log "falta el metadato mooc-compose"; exit 1; }
cat > /opt/mooc/.env <<EOF
MOOC_IMAGE=$REGISTRY/$ROL:$VERSION
MOOC_ROL=$ROL
EOF
echo "$VERSION" > /etc/mooc/version

# --- 4. Descargar la imagen exacta y arrancar -------------------------
# El ayudante de credenciales de gcloud autentica a Docker con la identidad
# de la VM (roles/artifactregistry.reader). No hay llaves en disco.
gcloud auth configure-docker "$REGION-docker.pkg.dev" --quiet >/dev/null 2>&1
cd /opt/mooc
reintentar 5 docker compose pull
docker compose up -d --remove-orphans
log "contenedores arriba con $REGISTRY/$ROL:$VERSION"

# --- 5. Esperar a que la aplicacion responda ---------------------------
# El tiempo t= de esta linea es el arranque completo de la instancia (desde
# que el kernel inicia) y es el que se reporta en el informe de capacidad.
# El balanceador y el autoescalador tienen sus propias verificaciones; esto
# solo deja la marca en el log.
for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null "$SALUD"; then
    log "LISTA: $ROL responde en $SALUD"
    exit 0
  fi
  sleep 3
done
log "ADVERTENCIA: $ROL no respondio en 180 s; ver 'docker compose logs' en /opt/mooc"
exit 1
