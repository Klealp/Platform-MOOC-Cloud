#!/usr/bin/env bash
# =====================================================================
# Aprovisionamiento en Google Cloud (Entregas 2 y 3).
#
# Entrega 3 - aplicacion elastica:
#   Balanceador HTTPS --/api/v1/*--> MIG mooc-api (1-3, CPU)    --+--> Cloud SQL
#          \----------- resto ----> MIG mooc-web (fijo en 1)      |--> Cloud Storage
#                                    MIG mooc-worker (1-3, cola) --+
#                                    infra-server: Redis/asynq + Mailpit
#
#   export PROJECT=dsc-uniandes-20262
#   ./deploy/gcp/provision.sh elastica      # red, identidades, registro
#                                           # y secretos
#   ./deploy/gcp/build.sh                   # imagenes del commit actual
#   ./deploy/gcp/provision.sh infra grupos balanceador
#   ./deploy/gcp/provision.sh desplegar     # version nueva: plantillas +
#                                           # reemplazo gradual de las VMs
#   Pasos sueltos: red-elastica firewall-elastico identidades registro
#   secretos infra plantillas grupos balanceador desplegar resumen
#   VERSION=<sha> fija la version (por defecto, el commit actual).
#
# Entrega 2 - dos VMs fijas (se conserva para reconstruir la linea base):
#   ./deploy/gcp/provision.sh todo          # todo en orden
#   ./deploy/gcp/provision.sh <paso>        # apis red firewall sql bucket
#                                           # cuentas vms config presupuesto
#   ./deploy/gcp/provision.sh monitor       # opcional: Prometheus en el Web
#                                           # Server (deploy/gcp/monitor/)
#
# Se pueden encadenar pasos: provision.sh grupos balanceador
#
# Los secretos generados (contrasenas, llaves HMAC, .env completos) se
# escriben en $SECRETS_DIR, FUERA del repositorio, con permisos 600.
# =====================================================================
set -euo pipefail

PROJECT="${PROJECT:?exporta PROJECT con el id del proyecto}"
REGION="${REGION:-us-central1}"
ZONE="${ZONE:-us-central1-a}"
NET="mooc-vpc"
SUBNET_WEB="mooc-web";       RANGE_WEB="10.20.1.0/24";    IP_WEB="10.20.1.10"
SUBNET_WORKER="mooc-worker"; RANGE_WORKER="10.20.2.0/24"; IP_WORKER="10.20.2.10"
# Entrega 3: los grupos api y web van en mooc-app (sin IP publica), el grupo
# worker reutiliza mooc-worker e infra-server tiene subred e IP propias.
SUBNET_APP="mooc-app";       RANGE_APP="10.20.3.0/24"
SUBNET_INFRA="mooc-infra";   RANGE_INFRA="10.20.4.0/24";  IP_INFRA="10.20.4.10"
# 2 vCPU / 2 GiB / 30 GiB exigidos por el enunciado: e2-highcpu-2 es
# exactamente esa combinacion. e2-small tambien anuncia 2 vCPU pero son de
# nucleo compartido (0.5 vCPU sostenida).
VM_TYPE="${VM_TYPE:-e2-highcpu-2}"
VM_DISK="30GB"
DB_INSTANCE="mooc-pg"
DB_TIER="${DB_TIER:-db-custom-1-3840}"   # 1 vCPU dedicada, 3.75 GB
BUCKET="${BUCKET:-mooc-$PROJECT}"
SECRETS_DIR="${SECRETS_DIR:-$HOME/.mooc-secrets/$PROJECT}"
BILLING_ACCOUNT="${BILLING_ACCOUNT:-}"   # opcional, para el presupuesto
BUDGET_USD="${BUDGET_USD:-50}"
RATE_LIMIT_PER_MIN="${RATE_LIMIT_PER_MIN:-120}"   # limite de tasa de la API
# Frontend e infra no necesitan 2 vCPU dedicadas y no entran en los limites
# de 1-3 instancias del enunciado.
WEB_TYPE="${WEB_TYPE:-e2-small}"
INFRA_TYPE="${INFRA_TYPE:-e2-small}"
# Trabajos (pendientes + activos) que se espera que atienda cada worker. Con
# WORKER_CONCURRENCY=2, 2 significa "cada VM ocupada y nada esperando".
BACKLOG_POR_WORKER="${BACKLOG_POR_WORKER:-2}"

AQUI="$(cd "$(dirname "$0")" && pwd)"
SA_API="mooc-api@$PROJECT.iam.gserviceaccount.com"
SA_WORKER="mooc-worker@$PROJECT.iam.gserviceaccount.com"
SA_VM="mooc-vm@$PROJECT.iam.gserviceaccount.com"
SA_WEB="mooc-web@$PROJECT.iam.gserviceaccount.com"
SA_INFRA="mooc-infra@$PROJECT.iam.gserviceaccount.com"
MIG_DIR="$AQUI/mig"
# Rangos de Google desde los que llegan el balanceador y las verificaciones
# de salud: https://cloud.google.com/load-balancing/docs/health-checks#firewall_rules
RANGOS_LB="130.211.0.0/22,35.191.0.0/16"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }
# existe <comando describe...>: evita fallar al repetir un paso ya hecho.
existe() { "$@" >/dev/null 2>&1; }
# privado <ruta> [modo]: en Linux/Cloud Shell restringe el acceso al dueno.
# En Git Bash sobre Windows chmod no aplica y la carpeta ya queda protegida
# por los permisos NTFS del perfil de usuario, asi que no se trata como error.
privado() { chmod "${2:-600}" "$1" 2>/dev/null || true; }
secreto() { openssl rand -base64 30 | tr -d '/+=' | cut -c1-32; }

gcloud config set project "$PROJECT" >/dev/null
mkdir -p "$SECRETS_DIR"; privado "$SECRETS_DIR" 700

paso_apis() {
  say "Habilitando APIs"
  gcloud services enable compute.googleapis.com sqladmin.googleapis.com \
    servicenetworking.googleapis.com storage.googleapis.com iap.googleapis.com \
    monitoring.googleapis.com logging.googleapis.com billingbudgets.googleapis.com \
    artifactregistry.googleapis.com secretmanager.googleapis.com
}

paso_red() {
  say "VPC $NET y subredes"
  existe gcloud compute networks describe "$NET" ||
    gcloud compute networks create "$NET" --subnet-mode=custom
  # Private Google Access: el worker, sin IP publica, llega a
  # storage.googleapis.com por la red de Google y no por el NAT.
  existe gcloud compute networks subnets describe "$SUBNET_WEB" --region="$REGION" ||
    gcloud compute networks subnets create "$SUBNET_WEB" --network="$NET" \
      --region="$REGION" --range="$RANGE_WEB" --enable-private-ip-google-access
  existe gcloud compute networks subnets describe "$SUBNET_WORKER" --region="$REGION" ||
    gcloud compute networks subnets create "$SUBNET_WORKER" --network="$NET" \
      --region="$REGION" --range="$RANGE_WORKER" --enable-private-ip-google-access

  say "Cloud NAT solo para la subred del worker (instalar paquetes, docker pull)"
  existe gcloud compute routers describe mooc-router --region="$REGION" ||
    gcloud compute routers create mooc-router --network="$NET" --region="$REGION"
  existe gcloud compute routers nats describe mooc-nat --router=mooc-router --region="$REGION" ||
    gcloud compute routers nats create mooc-nat --router=mooc-router --region="$REGION" \
      --nat-custom-subnet-ip-ranges="$SUBNET_WORKER" --auto-allocate-nat-external-ips

  say "Rango para Private Service Access (IP privada de Cloud SQL)"
  existe gcloud compute addresses describe mooc-psa --global ||
    gcloud compute addresses create mooc-psa --global --purpose=VPC_PEERING \
      --prefix-length=20 --network="$NET"
  gcloud services vpc-peerings connect --service=servicenetworking.googleapis.com \
    --ranges=mooc-psa --network="$NET" 2>/dev/null ||
  gcloud services vpc-peerings update --service=servicenetworking.googleapis.com \
    --ranges=mooc-psa --network="$NET" --force
}

paso_firewall() {
  say "Reglas de firewall (todo lo no listado queda bloqueado por defecto)"
  regla() { local n="$1"; shift; existe gcloud compute firewall-rules describe "$n" ||
    gcloud compute firewall-rules create "$n" --network="$NET" --direction=INGRESS "$@"; }
  # Unico punto publico: HTTP (redirige) y HTTPS hacia Caddy en el Web Server.
  regla mooc-allow-web-https --action=ALLOW --rules=tcp:80,tcp:443 \
    --source-ranges=0.0.0.0/0 --target-tags=web-server
  # Administracion por IAP: SSH sin exponer el puerto 22 a Internet.
  regla mooc-allow-iap-ssh --action=ALLOW --rules=tcp:22 \
    --source-ranges=35.235.240.0/20 --target-tags=web-server,worker-server
  # Redis/asynq: solo el Web Server (la API encola) llega al Worker Server.
  regla mooc-allow-web-to-redis --action=ALLOW --rules=tcp:6379 \
    --source-tags=web-server --target-tags=worker-server
  # Metricas de la app: solo desde la VM con el tag monitor (el Web Server,
  # donde corre Prometheus; ver paso "monitor").
  regla mooc-allow-monitor --action=ALLOW --rules=tcp:8080,tcp:9100 \
    --source-tags=monitor --target-tags=web-server,worker-server
}

paso_abrir_prometheus() {
  say "Publicando Prometheus (tcp 9090) en la IP del Web Server"
  # Decision del equipo para consultar las metricas sin tunel durante las
  # pruebas de carga. Prometheus no tiene autenticacion: cerrar al terminar
  # con   gcloud compute firewall-rules delete mooc-allow-prometheus
  regla() { local n="$1"; shift; existe gcloud compute firewall-rules describe "$n" ||
    gcloud compute firewall-rules create "$n" --network="$NET" --direction=INGRESS "$@"; }
  regla mooc-allow-prometheus --action=ALLOW --rules=tcp:9090 \
    --source-ranges="${PROMETHEUS_SOURCE:-0.0.0.0/0}" --target-tags=web-server
}

paso_sql() {
  say "Cloud SQL $DB_INSTANCE (PostgreSQL 16, una zona, sin IP publica)"
  if ! existe gcloud sql instances describe "$DB_INSTANCE"; then
    gcloud sql instances create "$DB_INSTANCE" --database-version=POSTGRES_16 \
      --edition=ENTERPRISE --tier="$DB_TIER" --zone="$ZONE" --availability-type=ZONAL \
      --storage-type=SSD --storage-size=10 \
      --network="projects/$PROJECT/global/networks/$NET" --no-assign-ip \
      --ssl-mode=ENCRYPTED_ONLY --backup-start-time=07:00
  fi
  existe gcloud sql databases describe mooc --instance="$DB_INSTANCE" ||
    gcloud sql databases create mooc --instance="$DB_INSTANCE"
  if [ ! -f "$SECRETS_DIR/db_password" ]; then
    secreto > "$SECRETS_DIR/db_password"; privado "$SECRETS_DIR/db_password"
    gcloud sql users create mooc --instance="$DB_INSTANCE" \
      --password="$(cat "$SECRETS_DIR/db_password")"
  fi
}

paso_bucket() {
  say "Bucket gs://$BUCKET (acceso uniforme, sin acceso publico)"
  existe gcloud storage buckets describe "gs://$BUCKET" ||
    gcloud storage buckets create "gs://$BUCKET" --location="$REGION" \
      --uniform-bucket-level-access --public-access-prevention
  gcloud storage buckets update "gs://$BUCKET" --cors-file="$AQUI/cors.json"
}

paso_cuentas() {
  say "Cuentas de servicio: una por componente"
  for sa in mooc-api mooc-worker mooc-vm; do
    existe gcloud iam service-accounts describe "$sa@$PROJECT.iam.gserviceaccount.com" ||
      gcloud iam service-accounts create "$sa" --display-name="MOOC $sa"
  done
  # API: crea objetos (carga multipart directa), los lee y firma URLs. No
  # borra. legacyBucketReader le da storage.buckets.get, que usa
  # EnsureBucket al arrancar.
  for r in roles/storage.objectCreator roles/storage.objectViewer roles/storage.legacyBucketReader; do
    gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" \
      --member="serviceAccount:$SA_API" --role="$r" >/dev/null
  done
  # Worker: lee originales, escribe derivados e insignias y BORRA lo que el
  # escaneo marca como infectado (internal/tasks/scan.go).
  gcloud storage buckets add-iam-policy-binding "gs://$BUCKET" \
    --member="serviceAccount:$SA_WORKER" --role=roles/storage.objectUser >/dev/null
  # Las VMs solo escriben logs y metricas; no tocan el bucket con su identidad.
  for r in roles/logging.logWriter roles/monitoring.metricWriter; do
    gcloud projects add-iam-policy-binding "$PROJECT" \
      --member="serviceAccount:$SA_VM" --role="$r" --condition=None >/dev/null
  done

  say "Llaves HMAC (la app habla S3 con minio-go; el secreto solo se ve una vez)"
  for comp in api worker; do
    f="$SECRETS_DIR/hmac_$comp"
    [ -f "$f" ] && continue
    sa="mooc-$comp@$PROJECT.iam.gserviceaccount.com"
    gcloud storage hmac create "$sa" --format="value(metadata.accessId,secret)" > "$f"
    privado "$f"
  done
}

paso_registro() {
  say "Artifact Registry: repositorio Docker mooc en $REGION"
  # Las imagenes las construye y sube deploy/gcp/build.sh. Las VMs solo las
  # descargan: llegan a $REGION-docker.pkg.dev por Private Google Access, sin
  # IP publica ni NAT.
  existe gcloud artifacts repositories describe mooc --location="$REGION" ||
    gcloud artifacts repositories create mooc --location="$REGION" \
      --repository-format=docker --description="Imagenes de la plataforma MOOC"
  # Las identidades de las VMs de los grupos descargan las imagenes. Solo
  # lectura: subir es trabajo de quien construye, no de quien ejecuta.
  for sa in "$SA_API" "$SA_WORKER" "$SA_WEB" "$SA_INFRA"; do
    gcloud artifacts repositories add-iam-policy-binding mooc --location="$REGION" \
      --member="serviceAccount:$sa" --role=roles/artifactregistry.reader >/dev/null
  done
}

paso_vms() {
  say "IP estatica y VMs ($VM_TYPE, disco $VM_DISK)"
  existe gcloud compute addresses describe mooc-web-ip --region="$REGION" ||
    gcloud compute addresses create mooc-web-ip --region="$REGION"
  comunes=(--zone="$ZONE" --machine-type="$VM_TYPE" --image-family=debian-12
    --image-project=debian-cloud --boot-disk-size="$VM_DISK" --boot-disk-type=pd-balanced
    --service-account="$SA_VM" --scopes=cloud-platform
    --metadata-from-file=startup-script="$AQUI/startup.sh" --shielded-secure-boot)
  existe gcloud compute instances describe web-server --zone="$ZONE" ||
    gcloud compute instances create web-server "${comunes[@]}" \
      --subnet="$SUBNET_WEB" --private-network-ip="$IP_WEB" \
      --address=mooc-web-ip --tags=web-server
  existe gcloud compute instances describe worker-server --zone="$ZONE" ||
    gcloud compute instances create worker-server "${comunes[@]}" \
      --subnet="$SUBNET_WORKER" --private-network-ip="$IP_WORKER" \
      --no-address --tags=worker-server
}

paso_monitor() {
  say "Web Server como nodo de monitoreo (tag monitor)"
  # Prometheus corre en el Web Server (deploy/gcp/monitor/): la cuota del
  # proyecto (12 vCPU) no deja crear una maquina de monitoreo aparte. El tag
  # monitor le permite raspar worker:9100; Redis y Cloud SQL ya los alcanzaba.
  gcloud compute instances add-tags web-server --zone="$ZONE" --tags=monitor
}

paso_config() {
  say "Generando web.env y worker.env en $SECRETS_DIR"
  local ip_pub ip_db dominio
  # tr -d '\r': gcloud en Windows termina las lineas en CRLF y el \r
  # terminaria dentro de los .env.
  ip_pub=$(gcloud compute addresses describe mooc-web-ip --region="$REGION" --format="value(address)" | tr -d '\r')
  ip_db=$(gcloud sql instances describe "$DB_INSTANCE" --format="value(ipAddresses[0].ipAddress)" | tr -d '\r')
  dominio="${ip_pub//./-}.sslip.io"
  [ -f "$SECRETS_DIR/redis_password" ] || { secreto > "$SECRETS_DIR/redis_password"; privado "$SECRETS_DIR/redis_password"; }
  [ -f "$SECRETS_DIR/admin_password" ] || { echo "Adm$(secreto | cut -c1-16)1!" > "$SECRETS_DIR/admin_password"; privado "$SECRETS_DIR/admin_password"; }

  rellenar() {  # rellenar <plantilla> <componente> <destino>
    local hk="" hs=""
    # El monitoreo no tiene llave HMAC: no toca el bucket.
    [ -f "$SECRETS_DIR/hmac_$2" ] && read -r hk hs < <(tr -d '\r' < "$SECRETS_DIR/hmac_$2")
    # La plantilla tambien pasa por tr: si un editor de Windows la guardo con
    # CRLF, Docker leeria "sslmode=require\r" y la conexion fallaria.
    tr -d '\r' < "$1" | sed -e "s#<DOMINIO>#$dominio#g" -e "s#<IP_CLOUDSQL>#$ip_db#g" \
        -e "s#<IP_WORKER>#$IP_WORKER#g" -e "s#<BUCKET>#$BUCKET#g" \
        -e "s#<DB_PASSWORD>#$(cat "$SECRETS_DIR/db_password")#g" \
        -e "s#<REDIS_PASSWORD>#$(cat "$SECRETS_DIR/redis_password")#g" \
        -e "s#<RATE_LIMIT_PER_MIN>#$RATE_LIMIT_PER_MIN#g" \
        -e "s#<ADMIN_PASSWORD>#$(cat "$SECRETS_DIR/admin_password")#g" \
        -e "s#<HMAC_ACCESS_ID>#$hk#g" -e "s#<HMAC_SECRET>#$hs#g" > "$3"
    privado "$3"
  }
  rellenar "$AQUI/web/web.env.example" api "$SECRETS_DIR/web.env"
  rellenar "$AQUI/worker/worker.env.example" worker "$SECRETS_DIR/worker.env"
  rellenar "$AQUI/monitor/monitor.env.example" monitor "$SECRETS_DIR/monitor.env"
  # El frontend (BFF) no tiene secretos ni marcadores: habla con la API por la
  # red interna de Docker (api:8080). Pasa por rellenar() solo para normalizar
  # finales de linea.
  rellenar "$AQUI/web/web-frontend.env.example" monitor "$SECRETS_DIR/web-frontend.env"

  say "Copiando la configuracion a /etc/mooc de cada VM (por IAP)"
  for par in "web-server:web.env" "worker-server:worker.env" "web-server:monitor.env" "web-server:web-frontend.env"; do
    vm="${par%%:*}"; f="${par##*:}"
    gcloud compute scp --zone="$ZONE" --tunnel-through-iap "$SECRETS_DIR/$f" "$vm:/tmp/$f"
    gcloud compute ssh "$vm" --zone="$ZONE" --tunnel-through-iap \
      --command="sudo install -d -m 700 /etc/mooc && sudo install -m 600 /tmp/$f /etc/mooc/$f && rm /tmp/$f"
  done
  echo "Dominio publico: https://$dominio"
}

# =====================================================================
# Entrega 3: aplicacion elastica
# =====================================================================

# version: etiqueta de imagen y sufijo de plantilla. Por defecto el commit
# actual, el mismo que usa build.sh.
version() { echo "${VERSION:-$(git -C "$AQUI" rev-parse --short=7 HEAD)}"; }

# regla_fw <nombre> <args...>: crea una regla de firewall si no existe.
regla_fw() {
  local n="$1"; shift
  existe gcloud compute firewall-rules describe "$n" ||
    gcloud compute firewall-rules create "$n" --network="$NET" --direction=INGRESS "$@"
}

# ip_lb: IP global del balanceador (la reserva si falta). El dominio publico
# se deriva de ella con sslip.io, asi que debe existir antes de los secretos.
ip_lb() {
  existe gcloud compute addresses describe mooc-lb-ip --global ||
    gcloud compute addresses create mooc-lb-ip --global --ip-version=IPV4 >&2
  gcloud compute addresses describe mooc-lb-ip --global --format="value(address)" | tr -d '\r'
}
dominio_lb() { local ip; ip="$(ip_lb)"; echo "${ip//./-}.sslip.io"; }

paso_red_elastica() {
  say "Subredes de la Entrega 3 ($SUBNET_APP, $SUBNET_INFRA), sin IPs publicas"
  for par in "$SUBNET_APP:$RANGE_APP" "$SUBNET_INFRA:$RANGE_INFRA"; do
    existe gcloud compute networks subnets describe "${par%%:*}" --region="$REGION" ||
      gcloud compute networks subnets create "${par%%:*}" --network="$NET" \
        --region="$REGION" --range="${par##*:}" --enable-private-ip-google-access
  done
  # Ninguna VM de la Entrega 3 tiene IP publica. Cloud NAT les da salida
  # para instalar Docker y el Ops Agent, y al BFF para llamar a la API por
  # el dominio publico. Artifact Registry, Secret Manager y Cloud Storage
  # van por Private Google Access, sin pasar por el NAT.
  say "Cloud NAT para $SUBNET_WORKER, $SUBNET_APP y $SUBNET_INFRA"
  gcloud compute routers nats update mooc-nat --router=mooc-router --region="$REGION" \
    --nat-custom-subnet-ip-ranges="$SUBNET_WORKER,$SUBNET_APP,$SUBNET_INFRA"
}

paso_firewall_elastico() {
  say "Firewall de la Entrega 3 (todo lo no listado sigue bloqueado)"
  # El balanceador y sus verificaciones de salud llegan desde rangos fijos
  # de Google: es lo unico que alcanza la API y el frontend.
  regla_fw mooc-allow-lb-api --action=ALLOW --rules=tcp:8080 \
    --source-ranges="$RANGOS_LB" --target-tags=mooc-api
  regla_fw mooc-allow-lb-web --action=ALLOW --rules=tcp:3000 \
    --source-ranges="$RANGOS_LB" --target-tags=mooc-web
  # El worker no recibe trafico, pero la autorreparacion del grupo verifica
  # su /healthz desde los mismos rangos.
  regla_fw mooc-allow-hc-worker --action=ALLOW --rules=tcp:9100 \
    --source-ranges="$RANGOS_LB" --target-tags=mooc-worker
  # Redis (cola, sesiones, limite de tasa) y SMTP: solo desde API y workers.
  regla_fw mooc-allow-app-to-infra --action=ALLOW --rules=tcp:6379,tcp:1025 \
    --source-tags=mooc-api,mooc-worker --target-tags=mooc-infra
  # Prometheus en infra-server raspa las metricas de la API y del worker.
  regla_fw mooc-allow-infra-scrape --action=ALLOW --rules=tcp:8080,tcp:9100 \
    --source-tags=mooc-infra --target-tags=mooc-api,mooc-worker
  # Administracion por IAP: solo para mirar, nunca para configurar a mano.
  regla_fw mooc-allow-iap-ssh-e3 --action=ALLOW --rules=tcp:22 \
    --source-ranges=35.235.240.0/20 --target-tags=mooc-api,mooc-worker,mooc-web,mooc-infra
}

paso_identidades() {
  say "Identidades de las VMs: una cuenta de servicio por grupo"
  # mooc-api y mooc-worker ya existen (paso cuentas) y tienen sus permisos
  # sobre el bucket; en la Entrega 3 tambien son la identidad de sus VMs.
  # La aplicacion sigue usando la llave HMAC para el bucket y usuario y
  # contrasena para Postgres: la identidad de la VM solo autoriza el acceso a
  # servicios de Google (imagenes, secretos, logs y metricas).
  for sa in mooc-api mooc-worker mooc-web mooc-infra; do
    existe gcloud iam service-accounts describe "$sa@$PROJECT.iam.gserviceaccount.com" ||
      gcloud iam service-accounts create "$sa" --display-name="MOOC $sa"
  done
  # Logs (driver gcplogs y startup.sh), metricas (Ops Agent) y, en infra, la
  # metrica de la cola que usa el autoescalador de workers.
  for sa in "$SA_API" "$SA_WORKER" "$SA_WEB" "$SA_INFRA"; do
    for r in roles/logging.logWriter roles/monitoring.metricWriter; do
      gcloud projects add-iam-policy-binding "$PROJECT" \
        --member="serviceAccount:$sa" --role="$r" --condition=None >/dev/null
    done
  done
}

# generar_env <plantilla> <componente_hmac> <destino> <dominio> <ip_db>
generar_env() {
  local hk="" hs=""
  [ -f "$SECRETS_DIR/hmac_$2" ] && read -r hk hs < <(tr -d '\r' < "$SECRETS_DIR/hmac_$2")
  tr -d '\r' < "$1" | sed -e "s#<DOMINIO>#$4#g" -e "s#<IP_CLOUDSQL>#$5#g" \
      -e "s#<IP_INFRA>#$IP_INFRA#g" -e "s#<BUCKET>#$BUCKET#g" \
      -e "s#<DB_PASSWORD>#$(cat "$SECRETS_DIR/db_password")#g" \
      -e "s#<REDIS_PASSWORD>#$(cat "$SECRETS_DIR/redis_password")#g" \
      -e "s#<RATE_LIMIT_PER_MIN>#$RATE_LIMIT_PER_MIN#g" \
      -e "s#<ADMIN_PASSWORD>#$(cat "$SECRETS_DIR/admin_password")#g" \
      -e "s#<HMAC_ACCESS_ID>#$hk#g" -e "s#<HMAC_SECRET>#$hs#g" > "$3"
  privado "$3"
  if grep -q '<[A-Z_]*>' "$3"; then
    echo "Quedaron marcadores sin reemplazar en $3:" >&2; grep -o '<[A-Z_]*>' "$3" | sort -u >&2
    return 1
  fi
}

paso_secretos() {
  say "Configuracion de cada rol en Secret Manager"
  local dominio ip_db
  dominio="$(dominio_lb)"
  ip_db=$(gcloud sql instances describe "$DB_INSTANCE" --format="value(ipAddresses[0].ipAddress)" | tr -d '\r')
  [ -f "$SECRETS_DIR/redis_password" ] || { secreto > "$SECRETS_DIR/redis_password"; privado "$SECRETS_DIR/redis_password"; }
  [ -f "$SECRETS_DIR/admin_password" ] || { echo "Adm$(secreto | cut -c1-16)1!" > "$SECRETS_DIR/admin_password"; privado "$SECRETS_DIR/admin_password"; }

  for rol in api worker web infra; do
    local f="$SECRETS_DIR/e3-$rol.env" sec="mooc-$rol-env"
    generar_env "$MIG_DIR/$rol/$rol.env.example" "$rol" "$f" "$dominio" "$ip_db"
    # Cada ejecucion publica una version nueva; las VMs leen "latest" al
    # arrancar, asi que el cambio aplica al recrearlas (paso desplegar).
    if existe gcloud secrets describe "$sec"; then
      gcloud secrets versions add "$sec" --data-file="$f" >/dev/null
    else
      gcloud secrets create "$sec" --replication-policy=automatic --data-file="$f" >/dev/null
    fi
    # Minimo privilegio: cada identidad lee SOLO su propio secreto.
    gcloud secrets add-iam-policy-binding "$sec" \
      --member="serviceAccount:mooc-$rol@$PROJECT.iam.gserviceaccount.com" \
      --role=roles/secretmanager.secretAccessor >/dev/null
    echo "  $sec <- $f"
  done
  echo "Dominio publico: https://$dominio"
}

paso_infra() {
  local v; v="$(version)"
  say "infra-server: Redis, Mailpit y queue-metrics $v ($INFRA_TYPE, $IP_INFRA)"
  imagen_existe queue-metrics "$v" || return 1
  local archivos="startup-script=$MIG_DIR/startup.sh,shutdown-script=$MIG_DIR/shutdown.sh,mooc-compose=$MIG_DIR/infra/docker-compose.yml"
  # VM fija: el autoescalador nunca la retira, asi que la cola no depende de
  # una maquina desechable. Se configura igual que las de los grupos.
  if ! existe gcloud compute instances describe infra-server --zone="$ZONE"; then
    gcloud compute instances create infra-server --zone="$ZONE" \
      --machine-type="$INFRA_TYPE" --image-family=debian-12 --image-project=debian-cloud \
      --boot-disk-size=20GB --boot-disk-type=pd-balanced \
      --subnet="$SUBNET_INFRA" --private-network-ip="$IP_INFRA" --no-address \
      --tags=mooc-infra --service-account="$SA_INFRA" --scopes=cloud-platform \
      --metadata=mooc-rol=infra,mooc-version="$v" \
      --metadata-from-file="$archivos" --shielded-secure-boot
    return
  fi
  # Ya existe: se actualizan los metadatos, pero NO se reinicia sola. Un
  # reinicio corta Redis unos segundos (la cola sobrevive por el AOF), asi
  # que se hace cuando se decide, fuera de una prueba de carga.
  gcloud compute instances add-metadata infra-server --zone="$ZONE" \
    --metadata=mooc-version="$v" --metadata-from-file="$archivos"
  echo "Metadatos actualizados a $v. Para aplicarlos (corta Redis unos segundos):"
  echo "  gcloud compute instances stop infra-server --zone=$ZONE && gcloud compute instances start infra-server --zone=$ZONE"
}

# imagen_existe <componente> <version>: sin imagen no hay arranque; mejor
# fallar aqui que con VMs reiniciando en bucle.
imagen_existe() {
  existe gcloud artifacts docker images describe "$REGION-docker.pkg.dev/$PROJECT/mooc/$1:$2" && return 0
  echo "No existe la imagen $1:$2 en Artifact Registry. Ejecuta antes: ./deploy/gcp/build.sh" >&2
  return 1
}

# plantilla_de <rol> <version>: nombre de la plantilla. Es inmutable: una
# version nueva siempre es una plantilla nueva.
plantilla_de() { echo "mooc-$1-tpl-$2"; }

paso_plantillas() {
  local v; v="$(version)"
  say "Plantillas de instancia para la version $v"
  for rol in api worker web; do
    local tpl tipo subred
    tpl="$(plantilla_de "$rol" "$v")"
    existe gcloud compute instance-templates describe "$tpl" && { echo "  $tpl ya existe"; continue; }
    imagen_existe "$rol" "$v" || return 1
    case "$rol" in
      api)    tipo="$VM_TYPE";  subred="$SUBNET_APP" ;;
      worker) tipo="$VM_TYPE";  subred="$SUBNET_WORKER" ;;
      web)    tipo="$WEB_TYPE"; subred="$SUBNET_APP" ;;
    esac
    gcloud compute instance-templates create "$tpl" \
      --machine-type="$tipo" --image-family=debian-12 --image-project=debian-cloud \
      --boot-disk-size="$VM_DISK" --boot-disk-type=pd-balanced \
      --network="$NET" --subnet="projects/$PROJECT/regions/$REGION/subnetworks/$subred" --no-address \
      --tags="mooc-$rol" --service-account="mooc-$rol@$PROJECT.iam.gserviceaccount.com" --scopes=cloud-platform \
      --metadata=mooc-rol="$rol",mooc-version="$v" \
      --metadata-from-file=startup-script="$MIG_DIR/startup.sh",shutdown-script="$MIG_DIR/shutdown.sh",mooc-compose="$MIG_DIR/$rol/docker-compose.yml" \
      --shielded-secure-boot
  done
}

# Dos tipos de verificacion, a proposito:
#   *-lb    la usa el BALANCEADOR para decidir si envia trafico (/readyz:
#           la instancia y sus dependencias responden).
#   *-vida  la usa el GRUPO para decidir si RECREA la VM (/healthz: el
#           proceso vive). Si fuera /readyz, una caida de Redis haria que el
#           grupo recreara todas las instancias a la vez sin arreglar nada.
crear_health_checks() {
  hc() {  # hc <nombre> <puerto> <ruta> <intervalo> <umbral_fallo>
    existe gcloud compute health-checks describe "$1" --global ||
      gcloud compute health-checks create http "$1" --global --port="$2" --request-path="$3" \
        --check-interval="$4" --timeout=5s --healthy-threshold=2 --unhealthy-threshold="$5"
  }
  hc mooc-hc-api-lb      8080 /readyz  10s 3
  hc mooc-hc-api-vida    8080 /healthz 15s 4
  hc mooc-hc-web         3000 /login   15s 3
  hc mooc-hc-worker-vida 9100 /healthz 15s 4
}

paso_grupos() {
  local v zona_args=(--zone="$ZONE")
  v="$(version)"
  paso_plantillas
  crear_health_checks
  say "Grupos administrados con la version $v"

  # grupo <rol> <health-check de vida>: crea el MIG si no existe. El retraso
  # inicial cubre el arranque completo (Docker + pull + app) para que la
  # autorreparacion no mate una VM que todavia esta arrancando.
  grupo() {
    existe gcloud compute instance-groups managed describe "mooc-$1" "${zona_args[@]}" ||
      gcloud compute instance-groups managed create "mooc-$1" "${zona_args[@]}" \
        --template="$(plantilla_de "$1" "$v")" --size=1 --base-instance-name="mooc-$1" \
        --health-check="$2" --initial-delay=300
  }
  grupo api    mooc-hc-api-vida
  grupo worker mooc-hc-worker-vida
  grupo web    mooc-hc-web
  gcloud compute instance-groups managed set-named-ports mooc-api "${zona_args[@]}" --named-ports=http-api:8080
  gcloud compute instance-groups managed set-named-ports mooc-web "${zona_args[@]}" --named-ports=http-web:3000

  # API: CPU promedio del grupo. 60 % deja margen para absorber el pico
  # mientras arranca la instancia nueva (en la Entrega 2 la API se degrado
  # con la CPU cerca del 100 %). Se reduce de a una VM cada 10 min.
  say "Autoescalado de mooc-api: 1-3 instancias, CPU objetivo 60 %"
  gcloud compute instance-groups managed set-autoscaling mooc-api "${zona_args[@]}" \
    --mode=on --min-num-replicas=1 --max-num-replicas=3 \
    --target-cpu-utilization=0.60 --cool-down-period=180 \
    --scale-in-control=max-scaled-in-replicas=1,time-window=600

  # Workers: trabajos por instancia, con una metrica de grupo (una sola
  # serie) que publica infra-server: pendientes + activos de asynq. La CPU
  # no sirve: ffmpeg la satura igual con 2 trabajos que con 200 en cola.
  # Mientras la metrica no exista, el autoescalador no tiene datos y deja el
  # grupo en su tamano actual.
  say "Autoescalado de mooc-worker: 1-3 instancias, $BACKLOG_POR_WORKER trabajos por instancia"
  gcloud compute instance-groups managed set-autoscaling mooc-worker "${zona_args[@]}" \
    --mode=on --min-num-replicas=1 --max-num-replicas=3 --cool-down-period=180 \
    --update-stackdriver-metric=custom.googleapis.com/mooc/queue_backlog \
    --stackdriver-metric-single-instance-assignment="$BACKLOG_POR_WORKER" \
    --stackdriver-metric-filter='resource.type = "global"' \
    --scale-in-control=max-scaled-in-replicas=1,time-window=600
  # mooc-web no autoescala: grupo fijo de 1 solo para tener plantilla y
  # autorreparacion.
}

paso_balanceador() {
  local dominio
  dominio="$(dominio_lb)"
  crear_health_checks
  say "Balanceador HTTPS externo para $dominio"

  # backend <nombre> <grupo> <puerto con nombre> <health-check>
  backend() {
    existe gcloud compute backend-services describe "$1" --global ||
      gcloud compute backend-services create "$1" --global \
        --load-balancing-scheme=EXTERNAL_MANAGED --protocol=HTTP --port-name="$3" \
        --health-checks="$4" --global-health-checks \
        --connection-draining-timeout=30 --timeout=120s \
        --enable-logging --logging-sample-rate=1.0
    # Drenaje de 30 s: al retirar una VM, el balanceador deja de enviarle
    # peticiones nuevas y espera las que estan en curso. Logs al 100 %: cada
    # linea dice que instancia atendio la peticion (evidencia del reparto).
    gcloud compute backend-services describe "$1" --global --format="value(backends[].group)" | grep -q "/$2\$" ||
      gcloud compute backend-services add-backend "$1" --global \
        --instance-group="$2" --instance-group-zone="$ZONE" \
        --balancing-mode=UTILIZATION --max-utilization=0.8
  }
  backend mooc-api-be mooc-api http-api mooc-hc-api-lb
  backend mooc-web-be mooc-web http-web mooc-hc-web

  # Mismo dominio para todo: la cookie HttpOnly del BFF funciona sin CORS.
  # /metrics no esta en la lista: va al frontend y responde 404.
  existe gcloud compute url-maps describe mooc-lb --global ||
    gcloud compute url-maps create mooc-lb --global --default-service=mooc-web-be
  gcloud compute url-maps describe mooc-lb --global --format="value(pathMatchers[].name)" | grep -qw rutas ||
    gcloud compute url-maps add-path-matcher mooc-lb --global --path-matcher-name=rutas \
      --default-service=mooc-web-be --new-hosts="$dominio" \
      --path-rules="/api/v1=mooc-api-be,/api/v1/*=mooc-api-be,/healthz=mooc-api-be,/readyz=mooc-api-be,/docs=mooc-api-be,/docs/*=mooc-api-be,/openapi.yaml=mooc-api-be,/openapi.yml=mooc-api-be"

  # Certificado administrado por Google: se emite solo cuando el dominio
  # resuelve a la IP del balanceador (sslip.io lo hace de inmediato).
  existe gcloud compute ssl-certificates describe mooc-cert --global ||
    gcloud compute ssl-certificates create mooc-cert --global --domains="$dominio"
  existe gcloud compute target-https-proxies describe mooc-https-proxy --global ||
    gcloud compute target-https-proxies create mooc-https-proxy --global \
      --url-map=mooc-lb --ssl-certificates=mooc-cert
  existe gcloud compute forwarding-rules describe mooc-https --global ||
    gcloud compute forwarding-rules create mooc-https --global \
      --load-balancing-scheme=EXTERNAL_MANAGED --address=mooc-lb-ip \
      --target-https-proxy=mooc-https-proxy --ports=443

  # El puerto 80 solo redirige a HTTPS.
  existe gcloud compute url-maps describe mooc-http-redirect --global ||
    gcloud compute url-maps import mooc-http-redirect --global --quiet --source=/dev/stdin <<YAML
name: mooc-http-redirect
defaultUrlRedirect:
  httpsRedirect: true
  redirectResponseCode: MOVED_PERMANENTLY_DEFAULT
YAML
  existe gcloud compute target-http-proxies describe mooc-http-proxy --global ||
    gcloud compute target-http-proxies create mooc-http-proxy --global --url-map=mooc-http-redirect
  existe gcloud compute forwarding-rules describe mooc-http --global ||
    gcloud compute forwarding-rules create mooc-http --global \
      --load-balancing-scheme=EXTERNAL_MANAGED --address=mooc-lb-ip \
      --target-http-proxy=mooc-http-proxy --ports=80

  echo "https://$dominio"
  echo "El certificado tarda de 15 a 60 min en quedar ACTIVE:"
  echo "  gcloud compute ssl-certificates describe mooc-cert --global --format='value(managed.status)'"
}

paso_desplegar() {
  local v; v="$(version)"
  paso_plantillas
  say "Reemplazo gradual de las VMs con la version $v"
  # max-surge=1 y max-unavailable=0: primero arranca la VM nueva y solo
  # cuando esta sana se retira una vieja; nunca baja la capacidad.
  for rol in api worker web; do
    gcloud compute instance-groups managed rolling-action start-update "mooc-$rol" \
      --zone="$ZONE" --version=template="$(plantilla_de "$rol" "$v")" \
      --type=proactive --max-surge=1 --max-unavailable=0
  done
  echo "Seguimiento: gcloud compute instance-groups managed list-instances mooc-api --zone=$ZONE"
}

paso_presupuesto() {
  if [ -z "$BILLING_ACCOUNT" ]; then
    echo "BILLING_ACCOUNT vacio: se omite el presupuesto (crealo en Billing > Budgets & alerts)."
    return
  fi
  say "Presupuesto de $BUDGET_USD USD con alertas al 50/90/100%"
  gcloud billing budgets create --billing-account="$BILLING_ACCOUNT" \
    --display-name="mooc-entrega2" --budget-amount="${BUDGET_USD}USD" \
    --filter-projects="projects/$PROJECT" \
    --threshold-rule=percent=0.5 --threshold-rule=percent=0.9 --threshold-rule=percent=1.0
}

paso_resumen() {
  say "Resumen"
  gcloud compute instances list --filter="name~'-server$'" \
    --format="table(name,machineType.basename(),networkInterfaces[0].networkIP,networkInterfaces[0].accessConfigs[0].natIP,status)"
  gcloud sql instances describe "$DB_INSTANCE" \
    --format="table(name,settings.tier,ipAddresses[0].ipAddress,ipAddresses[0].type,state)"
  echo "Bucket: gs://$BUCKET    Secretos: $SECRETS_DIR"
  # Entrega 3, si ya existe: grupos, tamano y plantilla vigente.
  gcloud compute instance-groups managed list --filter="name~'^mooc-'" \
    --format="table(name,zone.basename(),targetSize,instanceTemplate.basename())" 2>/dev/null || true
  existe gcloud compute addresses describe mooc-lb-ip --global && echo "Balanceador: https://$(dominio_lb)"
  return 0
}

[ "$#" -gt 0 ] || { sed -n '2,32p' "$0"; exit 1; }
for paso in "$@"; do
  case "$paso" in
    todo) paso_apis; paso_red; paso_firewall; paso_sql; paso_bucket; paso_cuentas
          paso_vms; echo "Espera ~3 min a que las VMs instalen Docker antes de 'config'." ;;
    # Base de la Entrega 3. Todos los pasos son idempotentes, asi que sirve
    # tanto para reconstruir desde cero como sobre lo que dejo la Entrega 2.
    # infra-server queda fuera: necesita la imagen de queue-metrics, que se
    # construye despues de crear el registro.
    elastica) paso_apis; paso_red; paso_red_elastica; paso_firewall_elastico
              paso_sql; paso_bucket; paso_cuentas; paso_identidades; paso_registro
              paso_secretos
              echo "Siguiente: ./deploy/gcp/build.sh && ./deploy/gcp/provision.sh infra grupos balanceador" ;;
    apis|red|firewall|sql|bucket|cuentas|registro|vms|monitor|config|presupuesto|resumen|\
    red-elastica|firewall-elastico|identidades|secretos|infra|plantillas|grupos|balanceador|desplegar)
      "paso_${paso//-/_}" ;;
    *) echo "Paso desconocido: $paso" >&2; sed -n '2,32p' "$0"; exit 1 ;;
  esac
done
