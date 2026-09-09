# Plataforma MOOC — Backend (Entrega 1)

Backend en Go de una plataforma de cursos masivos abiertos en linea.
Monolito modular + workers asincronos, todo en contenedores.

> Esta entrega es **solo backend**. No hay frontend.
> La documentacion detallada de la arquitectura y de cada archivo esta en
> [`DOCUMENTACION.md`](./DOCUMENTACION.md).

## Requisitos

- Docker y Docker Compose (probado con Docker 29.x / Docker Desktop 4.76)
- `curl` y `jq` para la prueba de humo

No hace falta tener Go instalado: la compilacion ocurre dentro de Docker.

## Puesta en marcha

```bash
cp .env.example .env
make deps          # genera go.sum (go mod tidy en un contenedor Go)
docker compose up --build
```

La primera vez tarda varios minutos: descarga las imagenes y compila los dos
binarios de Go.

Cuando todo este arriba:

| Servicio               | URL                              | Credenciales        |
|------------------------|----------------------------------|---------------------|
| API (health)           | http://localhost:8080/healthz    | —                   |
| **Swagger UI (API docs)** | http://localhost:8080/docs    | —                   |
| Spec OpenAPI (crudo)   | http://localhost:8080/openapi.yaml | —                 |
| Bandeja de correo      | http://localhost:8025            | —                   |
| Consola de MinIO       | http://localhost:9001            | minioadmin / minioadmin |
| Prometheus             | http://localhost:9090            | —                   |
| Grafana                | http://localhost:3000            | admin / admin       |
| **Jaeger (trazas)**    | http://localhost:16686           | —                   |
| PostgreSQL             | localhost:5432                   | mooc / mooc         |

Administrador inicial: `admin@mooc.local` / `Admin123!`
(lo crea la API al arrancar a partir de `ADMIN_EMAIL` y `ADMIN_PASSWORD`).

## Prueba de humo

```bash
./testdata/smoke.sh
```

Recorre el flujo completo: crear profesor, armar el curso, publicarlo,
registrar un estudiante, inscribirlo, reportar progreso, presentar el quiz y
verificar la insignia. De paso comprueba tres condiciones de la rubrica:
el rechazo del porcentaje enviado por el cliente, la idempotencia del envio
del quiz y el aislamiento entre estudiantes.

## Comandos frecuentes

```bash
make up                     # levantar
make down                   # detener (conserva datos)
make reset                  # detener y BORRAR volumenes
make logs                   # seguir logs de api y worker
make psql                   # consola SQL
docker compose up --scale worker=3   # tres workers
```

> **Importante:** `db/init.sql` solo se ejecuta la primera vez que se crea el
> volumen de Postgres. Si lo modificas, ejecuta `make reset` antes de volver a
> levantar.

## Arquitectura en una pantalla

```
                  ┌──────────────┐
   Cliente ──────▶│  API (Go)    │──── publica trabajos ───▶ Redis / asynq
                  │  sin estado  │                                │
                  └───┬──────┬───┘                                │
                      │      │                                    ▼
     URL prefirmada   │      │  metadatos              ┌──────────────────┐
   ┌──────────────────┘      └──────────▶ PostgreSQL   │  Worker (Go)     │
   │                                        ▲          │  ffmpeg, escaneo │
   ▼                                        └──────────│  correo, badges  │
┌──────────┐                                           └────────┬─────────┘
│  MinIO   │◀──────── sube y baja archivos ─────────────────────┘
└──────────┘
```

- La **API** valida, consulta, firma URLs y encola. Nunca transcodifica.
- Los **workers** hacen el trabajo pesado y se escalan por separado.
- **Postgres** es la fuente de verdad. **Redis** es cache y cola.
- **MinIO** guarda originales, derivados HLS e imagenes de insignias.
- **Mailpit** captura los correos en desarrollo.

## Variables de configuracion

Todas viven en `.env` (plantilla en `.env.example`) y se leen en un unico
lugar del codigo: `internal/config/config.go`.

La documentacion de la API (Swagger UI y el spec) **no necesita configuracion**:
se sirve embebida en el binario, siempre disponible en `/docs` y `/openapi.yaml`.

Las trazas OpenTelemetry se controlan con dos variables:

| Variable                      | Default       | Para que |
|-------------------------------|---------------|----------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `jaeger:4318` | Colector OTLP/HTTP al que la API y el worker envian sus trazas. **Vacio = trazas apagadas**, sin recompilar. |
| `OTEL_SAMPLE_RATIO`           | `1.0`         | Fraccion de trazas muestreadas (0..1). Bajalo si hay mucho trafico. |

## Contrato de la API

El contrato vive en `internal/openapi/openapi.yaml` (OpenAPI 3.1) y se incrusta
en el binario de la API. Con el sistema levantado se navega y se prueba desde el
navegador, sin herramientas externas:

- **Swagger UI:** http://localhost:8080/docs — cada operacion de escritura trae
  un ejemplo cargado, asi que "Try it out" ya viene con datos listos para enviar.
- **Spec crudo:** http://localhost:8080/openapi.yaml — util para Postman/Insomnia
  o para generar clientes.

Flujo tipico en Swagger UI: `POST /auth/login` con el admin
(`admin@mooc.local` / `Admin123!`), copiar el `access_token`, pulsar
**Authorize** y pegar el token; a partir de ahi se pueden recorrer los ejemplos
de crear profesor, curso, modulo, unidad, recurso, quiz e inscripcion.

Tambien se puede abrir el `.yaml` en [editor.swagger.io](https://editor.swagger.io).

## Observabilidad

Cada servicio expone metricas Prometheus (`/metrics` en la API y en `:9100` del
worker). Prometheus las raspa y Grafana las pinta, ambos provisionados por
archivo (sin pasos manuales):

- **Grafana:** http://localhost:3000 (admin/admin, tambien anonimo). El tablero
  **"Plataforma MOOC - Vision general"** queda cargado al arrancar
  (`deploy/grafana/dashboards/mooc-overview.json`) con p95/p99 de latencia, tasa
  de error 5xx, throughput por codigo, rate limiting, trabajos del worker por
  estado, **conteo en DLQ**, duracion de jobs y metricas de dominio
  (inscripciones, quizzes, insignias).
- **Prometheus:** http://localhost:9090 para consultas ad-hoc, p. ej.
  `histogram_quantile(0.95, sum by (le) (rate(mooc_http_request_duration_seconds_bucket[5m])))`.

El provisioning vive en `deploy/grafana/provisioning/` (fuente de datos con uid
fijo `prometheus` + proveedor de dashboards).

### Trazas (OpenTelemetry)

La API y el worker exportan trazas por OTLP a **Jaeger**:

- **Jaeger UI:** http://localhost:16686 — elige el servicio `mooc-api` o
  `mooc-worker` y pulsa *Find Traces*. Cada peticion HTTP muestra su span y, por
  debajo, las consultas SQL (instrumentadas con otelsql); cada trabajo del
  worker aparece como su propia traza.
- Se configura con `OTEL_EXPORTER_OTLP_ENDPOINT` (por defecto `jaeger:4318`).
  Dejala vacia y las trazas se apagan sin tocar el codigo.

La instrumentacion vive en `internal/telemetry` (arranque del exportador),
`otelgin` en el router y `otelsql` en la conexion a Postgres.

## Uso de agentes de inteligencia artificial

El diseno y la implementacion de este backend se realizaron con asistencia de
un agente de IA, segun lo permite el enunciado del curso. Debe declararse en
la sustentacion.
