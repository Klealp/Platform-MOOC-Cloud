#!/bin/bash
# =====================================================================
# Script de arranque de Web Server y Worker Server (metadata
# startup-script). GCE lo ejecuta como root en CADA arranque, asi que
# todo lo que hace debe ser idempotente.
# =====================================================================
set -euo pipefail

# Docker + compose plugin. El worker no tiene IP publica: sale por Cloud NAT.
if ! command -v docker >/dev/null 2>&1; then
  curl -fsSL https://get.docker.com | sh
  apt-get install -y git make jq
fi

# Swap de 2 GiB: con 2 GiB de RAM, compilar la imagen de Go dentro de la VM
# puede quedarse sin memoria. El swap solo absorbe ese pico del build; en
# operacion normal no deberia usarse (se vigila en Cloud Monitoring).
if ! swapon --show | grep -q /swapfile; then
  if [ ! -f /swapfile ]; then
    fallocate -l 2G /swapfile
    chmod 600 /swapfile
    mkswap /swapfile
  fi
  swapon /swapfile
  grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

# Ops Agent: CPU, memoria, disco y red de la VM en Cloud Monitoring. Las
# metricas por defecto de GCE no incluyen memoria ni uso de disco, y el
# analisis de capacidad las exige.
if ! systemctl is-active --quiet google-cloud-ops-agent; then
  curl -sSfo /tmp/add-ops-agent.sh https://dl.google.com/cloudagents/add-google-cloud-ops-agent-repo.sh
  bash /tmp/add-ops-agent.sh --also-install
fi

# Configuracion sensible: fuera del repositorio, solo legible por root.
install -d -m 700 /etc/mooc
