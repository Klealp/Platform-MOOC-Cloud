# Analisis de capacidad - Entrega 2

Scripts y evidencias de las pruebas de carga y estres sobre la plataforma MOOC.

- Motor de carga: **k6** (recorridos multi-paso, thresholds, salida reproducible).
- Observabilidad: **Prometheus + Grafana** (ya en `deploy/`) mas tres exportadores
  (`node`, `postgres`, `redis`) que anade `deploy/docker-compose.metrics.yml`.
- Validacion funcional y demo: **Postman** (coleccion en `postman/`).

```
capacity-planning/
  k6/
    escenario1_actividad_academica.js   Escenario 1 (actividad academica concurrente)
  resultados/                           Salidas crudas de cada corrida (JSON/CSV)
  pruebas_de_carga_entrega2.md          Informe (formato exigido por el enunciado)
```

## Requisitos

```bash
brew install k6          # macOS
# o: https://grafana.com/docs/k6/latest/set-up/install-k6/
```

## 1. Probar en LOCAL (depuracion del recorrido)

> Los numeros locales **NO** son capacidad: k6 y las "dos VMs" comparten la
> misma maquina y compiten por CPU/RAM, justo lo que el enunciado prohibe.
> Local sirve para validar que el script recorre bien el flujo y que las
> metricas se recogen. Los numeros del informe salen de la corrida en la nube.

```bash
# 1. Levantar la app + observabilidad (incluye los exportadores):
docker compose -f docker-compose.yml -f deploy/docker-compose.metrics.yml up -d --build

# 2. (opcional) quitar el limitador para medir la app y no el rate limit.
#    RATE_LIMIT_PER_MIN esta en .env; para carga real subirlo mucho:
#      RATE_LIMIT_PER_MIN=1000000
#    y reiniciar la api:  docker compose up -d api
#    (El login sigue topado a 30/min por IP; por eso las sesiones se
#     preparan en setup y N_STUDENTS por defecto es 20.)

# 3. Corrida de humo (pocos usuarios, rapida):
RATE_SCALE=0.2 k6 run capacity-planning/k6/escenario1_actividad_academica.js

# 4. Corrida completa local:
mkdir -p capacity-planning/resultados
k6 run --out json=capacity-planning/resultados/e1_local.json \
  capacity-planning/k6/escenario1_actividad_academica.js
```

Ver metricas mientras corre:
- Grafana: http://localhost:3000  (dashboard "MOOC overview")
- Prometheus: http://localhost:9090  (targets en /targets deben estar UP)

Consultas PromQL utiles para el informe:
- Latencia API p95: `histogram_quantile(0.95, sum(rate(mooc_http_request_duration_seconds_bucket[1m])) by (le))`
- Throughput API: `sum(rate(mooc_http_requests_total[1m]))`
- Rechazos por rate limit: `sum(rate(mooc_rate_limited_total[1m]))`
- Profundidad de la cola asynq: `redis_key_size{key=~"asynq:.*pending"}`
- Conexiones Postgres: `pg_stat_activity_count` (postgres-exporter)
- CPU/mem por VM: `node_*` (node-exporter)

## 2. Correr en la NUBE (numeros validos del informe)

El monitoreo ya esta desplegado en el Web Server ([`deploy/gcp/monitor/`](../deploy/gcp/monitor/)):
Prometheus + `redis-exporter` (cola asynq) + `postgres-exporter` (Cloud SQL), raspando la
API y el worker por la red privada. CPU/RAM/disco de las VMs y de Cloud SQL: Cloud Monitoring.

**1. Tunel a Prometheus** (dejar la terminal abierta):

```bash
gcloud compute ssh web-server --zone=us-central1-a --tunnel-through-iap -- -N -L 9090:localhost:9090
```

**2. Grafana local** (Docker Desktop abierto) -> http://localhost:3000 (admin / admin):

```bash
docker compose -f deploy/gcp/monitor/grafana-local.yml up -d
```

**3. Corrida de k6** desde una maquina FUERA de Web Server y Worker Server. `k6.sh` usa k6
nativo o la imagen de Docker, y guarda `raw.json.gz`, `summary.json` y `condiciones.txt` en
`capacity-planning/resultados/<corrida>/`:

```bash
export BASE_URL=https://136-114-53-21.sslip.io/api/v1
export ADMIN_PASSWORD="$(cat ~/.mooc-secrets/dsc-uniandes-20262/admin_password)"
# opcional, para ver la latencia de k6 en Grafana junto a la de la API:
export PROM_RW_URL=http://host.docker.internal:9090/api/v1/write   # (k6 nativo: http://localhost:9090/api/v1/write)
N_STUDENTS=20 ./deploy/gcp/monitor/k6.sh capacity-planning/k6/escenario1_actividad_academica.js e1-nivel1
```

Estado del limitador durante las pruebas: `RATE_LIMIT_PER_MIN=10000000000` (desactivado para
rutas autenticadas y catalogo); las rutas publicas (login, registro) siguen en 30/min por IP.

## Parametros (variables de entorno)

| Variable | Default | Uso |
|---|---|---|
| `BASE_URL` | `http://localhost:8080/api/v1` | unico cambio local -> nube |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | `admin@mooc.local` / `Admin123!` | crea profesor y estudiantes |
| `N_STUDENTS` | `20` | tamano del pool de cuentas (subirlo requiere sembrar en BD, ver nota de login 30/min) |
| `RATE_SCALE` | `1` | escala todos los niveles de tasa de llegada (0.2 = humo, 2 = doble) |
| `PRE_VUS` / `MAX_VUS` | `50` / `300` | VUs preasignados y tope para el modelo de tasa de llegada |

## Niveles de carga (Escenario 1)

Modelo de **tasa de llegada** (open model): linea base + 3 niveles crecientes +
repeticion cerca del limite + enfriamiento. Con `RATE_SCALE=1`: 5 -> 20 -> 50
-> 90 -> 90 -> 5 req/s. Editar `stages` en el script para tu hardware.

## Notas de fidelidad (para no invalidar la medicion)

- El generador debe ir **fuera** de las dos VMs y no ser el cuello de botella:
  vigilar CPU del generador durante la corrida.
- El **login** es publico y topado a 30/min por IP (no configurable): las
  sesiones se preparan en `setup`; una rafaga de login se mide como
  **variante separada**, no dentro de la actividad academica.
- El script **distingue**: `errores_funcionales` (fallo real) vs
  `rechazos_negocio` (4xx esperado) vs `tasa_rate_limited` (429).
- Un HTTP 200 no basta: el script valida el resultado (nota del quiz) y
  comprueba **envio duplicado sin doble calificacion**.
- Fijar la infraestructura y registrar por corrida: version, `RATE_LIMIT_PER_MIN`,
  numero de workers, `N_STUDENTS` y los `stages`.
