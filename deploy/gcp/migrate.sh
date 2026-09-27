set -euo pipefail

RAIZ="$(cd "$(dirname "$0")/../.." && pwd)"
PG_IMG="postgres:16-alpine"
RCLONE_IMG="rclone/rclone:1.75"
ENV_WEB="/etc/mooc/web.env"

say() { printf '\n\033[1;36m== %s\033[0m\n' "$*"; }
# valor <archivo> <VAR>: lee una variable de un .env sin ejecutarlo.
valor() { sudo grep -m1 "^$2=" "$1" | cut -d= -f2-; }

psql_cloud() {  # psql contra Cloud SQL usando DATABASE_URL de /etc/mooc/web.env
  sudo docker run --rm -i "$PG_IMG" psql "$(valor "$ENV_WEB" DATABASE_URL)" \
    -v ON_ERROR_STOP=1 "$@"
}


rclone_gcs() {  # rclone_gcs <access_id> <secret> <args...>
  local id="$1" sec="$2"; shift 2
  sudo docker run --rm --network host \
    -e RCLONE_CONFIG_GCS_TYPE=s3 -e RCLONE_CONFIG_GCS_PROVIDER=GCS \
    -e RCLONE_CONFIG_GCS_ENDPOINT=https://storage.googleapis.com \
    -e RCLONE_CONFIG_GCS_ACCESS_KEY_ID="$id" -e RCLONE_CONFIG_GCS_SECRET_ACCESS_KEY="$sec" \
    -e RCLONE_CONFIG_SRC_TYPE=s3 -e RCLONE_CONFIG_SRC_PROVIDER=Minio \
    -e RCLONE_CONFIG_SRC_ENDPOINT="${SRC_ENDPOINT:-http://localhost:9000}" \
    -e RCLONE_CONFIG_SRC_ACCESS_KEY_ID="${SRC_ACCESS_KEY:-minioadmin}" \
    -e RCLONE_CONFIG_SRC_SECRET_ACCESS_KEY="${SRC_SECRET_KEY:-minioadmin}" \
    "$RCLONE_IMG" "$@"
}

case "${1:-}" in
  dump-db)
    say "pg_dump de la BD local de la Entrega 1 -> mooc.dump"
    (cd "$RAIZ" && docker compose exec -T postgres \
      pg_dump -U mooc -d mooc --format=custom --no-owner --no-privileges) > mooc.dump
    ls -lh mooc.dump
    echo "Copialo al Web Server: gcloud compute scp mooc.dump web-server:~ --zone=<zona> --tunnel-through-iap"
    ;;

  init-db)
    say "Cargando db/init.sql en Cloud SQL"
    psql_cloud < "$RAIZ/db/init.sql"
    ;;

  restore-db)
    dump="${2:?uso: migrate.sh restore-db <archivo.dump>}"
    say "pg_restore de $dump en Cloud SQL"
    sudo docker run --rm -i "$PG_IMG" pg_restore --no-owner --no-privileges \
      --exit-on-error -d "$(valor "$ENV_WEB" DATABASE_URL)" < "$dump"
    ;;

  objects)
    : "${BUCKET:?}" "${HMAC_ID:?llave HMAC con escritura (la del worker)}" "${HMAC_SECRET:?}"
    src="${SRC_BUCKET:-mooc}"
    say "Copiando src:$src -> gcs:$BUCKET"
    rclone_gcs "$HMAC_ID" "$HMAC_SECRET" copy "src:$src" "gcs:$BUCKET" --checksum --stats-one-line -P
    # --download compara el CONTENIDO byte a byte: las cargas multipart no
    # tienen un MD5 comparable en el ETag, asi que el hash no basta.
    say "Verificando contenido (rclone check --download)"
    rclone_gcs "$HMAC_ID" "$HMAC_SECRET" check "src:$src" "gcs:$BUCKET" --download --one-way
    ;;

  verify)
    bucket=$(valor "$ENV_WEB" S3_BUCKET)
    id=$(valor "$ENV_WEB" S3_ACCESS_KEY); sec=$(valor "$ENV_WEB" S3_SECRET_KEY)
    tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
    say "Llaves referenciadas por la BD"
    psql_cloud -At <<'SQL' > "$tmp/esperadas"
SELECT object_key FROM assets WHERE status NOT IN ('awaiting_upload', 'failed', 'infected')
UNION ALL
SELECT hls_prefix || '/' FROM assets WHERE hls_prefix <> ''
UNION ALL
SELECT image_key FROM badges WHERE image_key <> '';
SQL
    say "Objetos presentes en gs://$bucket"
    rclone_gcs "$id" "$sec" lsf -R --files-only "gcs:$bucket" > "$tmp/presentes"
    faltan=0
    while IFS= read -r k; do
      [ -z "$k" ] && continue
      if [[ "$k" == */ ]]; then grep -q "^$k" "$tmp/presentes" || { echo "FALTA prefijo $k"; faltan=$((faltan+1)); }
      else grep -qxF "$k" "$tmp/presentes" || { echo "FALTA $k"; faltan=$((faltan+1)); }
      fi
    done < "$tmp/esperadas"
    echo "referencias: $(grep -c . "$tmp/esperadas" || true)  objetos: $(wc -l < "$tmp/presentes")  faltantes: $faltan"
    [ "$faltan" -eq 0 ]
    ;;

  *) sed -n '2,19p' "$0"; exit 1 ;;
esac
