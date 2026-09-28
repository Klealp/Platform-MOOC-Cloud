#!/usr/bin/env bash
# =====================================================================
# Aprovisionamiento de la Entrega 2 en Google Cloud.
#
#   Web Server (VM, IP publica) ---> Cloud SQL PostgreSQL (IP privada)
#        |                     \---> Cloud Storage (HTTPS, HMAC propio)
#        v  6379 (red privada)
#   Worker Server (VM, sin IP publica, sale por Cloud NAT)
#        worker + Redis/asynq + Mailpit
#
# Uso (desde Cloud Shell o cualquier bash con gcloud autenticado):
#   export PROJECT=dsc-uniandes-20262
#   ./deploy/gcp/provision.sh todo          # todo en orden
#   ./deploy/gcp/provision.sh <paso>        # un paso: apis red firewall
#                                           # sql bucket cuentas vms config
#                                           # presupuesto resumen
#   ./deploy/gcp/provision.sh monitor       # opcional: Prometheus en el Web
#                                           # Server (deploy/gcp/monitor/)
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

AQUI="$(cd "$(dirname "$0")" && pwd)"
SA_API="mooc-api@$PROJECT.iam.gserviceaccount.com"
SA_WORKER="mooc-worker@$PROJECT.iam.gserviceaccount.com"
SA_VM="mooc-vm@$PROJECT.iam.gserviceaccount.com"

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
    monitoring.googleapis.com logging.googleapis.com billingbudgets.googleapis.com
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

  say "Copiando la configuracion a /etc/mooc de cada VM (por IAP)"
  for par in "web-server:web.env" "worker-server:worker.env" "web-server:monitor.env"; do
    vm="${par%%:*}"; f="${par##*:}"
    gcloud compute scp --zone="$ZONE" --tunnel-through-iap "$SECRETS_DIR/$f" "$vm:/tmp/$f"
    gcloud compute ssh "$vm" --zone="$ZONE" --tunnel-through-iap \
      --command="sudo install -d -m 700 /etc/mooc && sudo install -m 600 /tmp/$f /etc/mooc/$f && rm /tmp/$f"
  done
  echo "Dominio publico: https://$dominio"
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
}

case "${1:-}" in
  todo) paso_apis; paso_red; paso_firewall; paso_sql; paso_bucket; paso_cuentas
        paso_vms; echo "Espera ~3 min a que las VMs instalen Docker antes de 'config'." ;;
  apis|red|firewall|sql|bucket|cuentas|vms|monitor|config|presupuesto|resumen) "paso_$1" ;;
  *) sed -n '2,21p' "$0"; exit 1 ;;
esac
