#!/bin/bash
# =====================================================================
# Script de apagado de las VMs de los grupos administrados (Entrega 3).
#
# La plantilla lo adjunta como metadato "shutdown-script". Google lo
# ejecuta cuando el autoescalador retira la VM, cuando el grupo la
# reemplaza o cuando alguien la detiene. Hay un margen aproximado de 90 s
# antes del apagado forzado, y es "mejor esfuerzo": si la VM se cae de
# golpe no corre, y por eso la correccion no puede depender de el.
#
#   api/web: SIGTERM -> la API deja de aceptar conexiones y termina las
#            peticiones en curso (cmd/api/main.go, hasta 20 s). El
#            balanceador ya dejo de enviarle trafico (drenaje).
#   worker:  SIGTERM -> asynq deja de tomar trabajos nuevos y espera a los
#            activos. Lo que no termine a tiempo vuelve a la cola (al
#            apagarse asynq, o cuando vence su lease si la VM muere) y otro
#            worker lo reintenta; la idempotencia evita efectos duplicados.
# =====================================================================
set -uo pipefail

ROL="$(curl -fsS -H 'Metadata-Flavor: Google' \
  http://metadata.google.internal/computeMetadata/v1/instance/attributes/mooc-rol 2>/dev/null || echo desconocido)"
echo "[mooc-shutdown $(date -u +%H:%M:%S)] rol=$ROL: deteniendo contenedores"

# El tiempo de gracia de cada servicio esta en su stop_grace_period; aqui
# solo se acota por debajo del margen de ~90 s de Compute Engine.
cd /opt/mooc 2>/dev/null && timeout 85 docker compose stop
echo "[mooc-shutdown $(date -u +%H:%M:%S)] rol=$ROL: contenedores detenidos"
