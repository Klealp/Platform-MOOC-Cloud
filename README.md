# Plataforma MOOC

Plataforma de cursos masivos abiertos en linea (MOOC).
Backend en Go (monolito modular + workers asincronos) y frontend Next.js (BFF),
todo en contenedores.

> La documentacion detallada de la arquitectura y de cada archivo esta en
> [`DOCUMENTACION.md`](./DOCUMENTACION.md).
>
> **Despliegue local completo (backend + frontend) con un solo comando:**
> `docker compose up --build` levanta la API, los workers, el frontend y todas
> las dependencias. El **frontend queda en http://localhost:3001**.

## Documentacion por entrega

| Que | Donde |
|---|---|
| Arquitectura logica, documentacion de cada archivo y credenciales locales (Entrega 1) | [`docs/entrega1/`](docs/entrega1/) (`ARQUITECTURA.md`, `DOCUMENTACION.md`, `CREDENCIALES.md`) |
| Arquitectura del despliegue en GCP (Entrega 2) | [`docs/entrega2/README.md`](docs/entrega2/README.md) |
| Informe de capacidad (Entrega 2) | [`capacity-planning/pruebas_de_carga_entrega2.md`](capacity-planning/pruebas_de_carga_entrega2.md) |
| Aprovisionamiento, compose por VM y migracion | [`deploy/gcp/`](deploy/gcp/) |
| Prueba automatica de accesibilidad WCAG 2.2 AA (Entrega 3) | [`docs/entrega3/Prueba Accesibilidad/README.md`](docs/entrega3/Prueba%20Accesibilidad/README.md) |
| Ultimo reporte de accesibilidad | [`docs/entrega3/Prueba Accesibilidad/resultados/REPORTE.md`](docs/entrega3/Prueba%20Accesibilidad/resultados/REPORTE.md) |
| Convenciones de accesibilidad del frontend | [`frontend/README.md`](frontend/README.md), seccion "Accesibilidad (WCAG 2.2 AA)" |
| URL de la aplicacion | https://136-114-53-21.sslip.io |

Las credenciales de la nube se entregan al equipo docente por el canal privado
del curso, nunca en este repositorio.

## Arquitectura actual (despliegue en Google Cloud)

La arquitectura vigente es la descrita en
[`docs/entrega2/`](docs/entrega2/README.md): dos VMs `e2-highcpu-2` en una VPC
propia (`us-central1-a`) mas servicios administrados.

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
| **Frontend (app web)** | http://localhost:3001            | ver admin abajo     |
| API (health)           | http://localhost:8080/healthz    | —                   |
| **Swagger UI (API docs)** | http://localhost:8080/docs    | —                   |
| Spec OpenAPI (crudo)   | http://localhost:8080/openapi.yaml | —                 |
| Bandeja de correo      | http://localhost:8025            | —                   |
| Consola de MinIO       | http://localhost:9001            | minioadmin / minioadmin |
| Prometheus             | http://localhost:9090            | —                   |
| Grafana                | http://localhost:3000            | admin / admin       |
| **Jaeger (trazas)**    | http://localhost:16686           | —                   |
| PostgreSQL             | localhost:5433                   | mooc / mooc         |

> El **frontend** (Next.js como BFF) es la única puerta de entrada para el
> navegador: habla con la API por la red interna de Docker (`api:8080`), nunca
> directo desde el cliente. Se publica en **3001** porque Grafana ya ocupa el
> 3000 en el host.

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

## Prueba de accesibilidad

```bash
"./docs/entrega3/Prueba Accesibilidad/ejecutar.sh"
```

Auditoria automatica WCAG 2.2 AA con Playwright + axe-core. Corre por completo
dentro de la imagen oficial de Playwright, conectada a la red de Docker
Compose: no instala nada en la maquina ni modifica la API. Recorre 19 paginas
de los cuatro roles (anonimo, estudiante, profesor, administrador) en tema
claro y oscuro, y aprueba solo si ninguna tiene incumplimientos `critical` o
`serious`. Requiere el proyecto levantado y los datos que deja la prueba de
humo. El reporte queda en
[`resultados/REPORTE.md`](docs/entrega3/Prueba%20Accesibilidad/resultados/REPORTE.md); la ultima
ejecucion (2026-10-24) aprobo las 38 auditorias. Paso a paso en
[`docs/entrega3/Prueba Accesibilidad/README.md`](docs/entrega3/Prueba%20Accesibilidad/README.md).

## Comandos frecuentes

```bash
make up                     # levantar
make down                   # detener (conserva datos)
make reset                  # detener y BORRAR volumenes
make logs                   # seguir logs de api y worker
make psql                   # consola SQL
docker compose up --scale worker=3   # tres workers
docker compose up -d --build --no-deps web   # reconstruir solo el frontend sin tocar lo demas
```

> **Importante:** `db/init.sql` solo se ejecuta la primera vez que se crea el
> volumen de Postgres. Si lo modificas, ejecuta `make reset` antes de volver a
> levantar.

## Arquitectura en una pantalla

```
                         Internet (HTTPS, Let's Encrypt)
                                      │
  ┌─ Web Server (subred publica) ─────▼────────────────────────────┐
  │  Caddy ── /api/v1/*, /docs, /openapi.yaml, /healthz, /readyz ─▶ API (Go)
  │    └──── todo lo demas ─▶ Frontend Next.js (BFF) ─ red interna ─▶ API
  │  Prometheus + redis-exporter + postgres-exporter (solo localhost)
  └──────────────┬──────────────────┬──────────────────┬───────────┘
                 │ SQL + TLS        │ encola (6379)    │ firma URLs
                 ▼                  ▼                  ▼
          Cloud SQL (PG 16)   ┌─ Worker Server ─┐   Cloud Storage
          IP privada          │ Redis / asynq   │   (bucket privado,
                ▲             │ Worker (Go)     │──▶ API XML S3 + HMAC)
                └─────────────│ Mailpit         │        ▲
                              └─ sin IP publica ┘        │
         Navegador ── PUT/GET directo con URL firmada ───┘
```

| Componente | Donde corre | Responsabilidad |
|---|---|---|
| **Caddy** | Web Server | Unico servicio expuesto a Internet. Termina TLS y enruta por ruta; oculta `/metrics`. |
| **API** (`cmd/api`) | Web Server | Autenticacion con tokens opacos, autoria versionada, inscripciones, progreso calculado en servidor, calificacion de quizzes y emision de URLs firmadas. Sin estado; nunca transcodifica. |
| **Frontend / BFF** (`frontend/`) | Web Server | Interfaz web. Guarda el token en una cookie HttpOnly y llama a la API por la red interna con `Authorization: Bearer`. |
| **Worker** (`cmd/worker`) | Worker Server | Escaneo antimalware (stub EICAR), transcodificacion HLS con ffmpeg, insignias, correo y limpieza de cargas vencidas. |
| **Redis + asynq** | Worker Server | Cola con tres prioridades, cache de sesion y rate limit. Solo alcanzable desde el Web Server. |
| **Cloud SQL** | Administrado | PostgreSQL 16, fuente de verdad. IP privada y TLS obligatorio. |
| **Cloud Storage** | Administrado | Originales, derivados HLS e insignias. El cliente sube y descarga directamente con URLs firmadas. |
| **Mailpit** | Worker Server | Captura del correo saliente. |

Principios que se mantienen desde la Entrega 1: la API no guarda estado local,
los archivos no pasan por la API, Postgres es la fuente de verdad (si Redis se
vacia el sistema sigue siendo correcto) y la idempotencia de las tareas tiene
tres capas (`asynq.TaskID`, tabla `processed_jobs` y restricciones de
integridad).

**Observabilidad en la nube:** Cloud Monitoring (Ops Agent en las VMs y
metricas nativas de Cloud SQL) para infraestructura; Prometheus y exportadores
en el Web Server para metricas de la aplicacion y de la cola; Grafana corre en
el PC de quien analiza, conectado por un tunel IAP. Las trazas OpenTelemetry
estan apagadas en la nube por memoria.

Para red, firewall, cuentas de servicio, costos, puntos unicos de falla y
operacion (despliegue, migracion, respaldo) ver
[`docs/entrega2/README.md`](docs/entrega2/README.md).

## Mapa del repositorio

| Carpeta / archivo | Contenido | Mas documentacion |
|---|---|---|
| [`cmd/api/`](cmd/api/) | Punto de entrada del proceso HTTP (`main.go`). | `docs/entrega1/DOCUMENTACION.md` |
| [`cmd/worker/`](cmd/worker/) | Punto de entrada del proceso que consume la cola (`main.go`). | `docs/entrega1/DOCUMENTACION.md` |
| [`internal/`](internal/) | Paquetes del backend: `api` (handlers, middlewares, rutas), `auth`, `audit`, `badge`, `config` (unico lugar donde se leen variables de entorno), `database`, `mailer`, `metrics`, `openapi` (contrato embebido), `progress`, `queue`, `storage`, `tasks` (handlers del worker) y `telemetry`. | `docs/entrega1/DOCUMENTACION.md` |
| [`db/`](db/) | `init.sql`, esquema completo de la base. Solo se ejecuta al crear el volumen. | `docs/entrega1/ARQUITECTURA.md` |
| [`frontend/`](frontend/) | Frontend Next.js 15 + TypeScript + Tailwind (BFF). | [`frontend/README.md`](frontend/README.md) |
| [`deploy/gcp/`](deploy/gcp/) | `provision.sh`, `startup.sh`, `migrate.sh`, `cors.json` y un compose por VM: `web/` (API, frontend, Caddy), `worker/` (worker, Redis, Mailpit) y `monitor/` (Prometheus, exportadores, Grafana local, `k6.sh`). | [`docs/entrega2/README.md`](docs/entrega2/README.md) |
| [`deploy/`](deploy/) | Configuracion local de Prometheus y Grafana (dashboard provisionado) y `docker-compose.metrics.yml` con exportadores. | — |
| [`capacity-planning/`](capacity-planning/) | Scripts de k6, resultados y el informe de capacidad de los escenarios 1 y 2. | [`capacity-planning/README.md`](capacity-planning/README.md), [`capacity-planning/escenario2/README.md`](capacity-planning/escenario2/README.md) |
| [`postman/`](postman/) | Coleccion de la API y entornos local y nube. | [`postman/README.md`](postman/README.md) |
| [`.postman/`](.postman/) | Metadatos del workspace de Postman. | — |
| [`docs/`](docs/) | Documentacion de cada entrega (`entrega1/`, `entrega2/`, `entrega3/`). | — |
| [`docs/entrega3/Prueba Accesibilidad/`](docs/entrega3/Prueba%20Accesibilidad/) | Auditoria automatica WCAG 2.2 AA (`ejecutar.sh`, script de Playwright + axe en `prueba/`) y su ultimo reporte en `resultados/`. | [`README.md`](docs/entrega3/Prueba%20Accesibilidad/README.md) |
| [`docs/entrega1/testdata/`](docs/entrega1/testdata/) | Pruebas de humo de extremo a extremo (`smoke.sh` y `smoke_*.sh`, cada una demuestra un comportamiento concreto). | [`docs/entrega1/testdata/README.md`](docs/entrega1/testdata/README.md) |
| `Dockerfile.api`, `Dockerfile.worker`, `Dockerfile.web` | Imagenes de la API, el worker y el frontend. | — |
| `docker-compose.yml`, `Makefile` | Entorno local completo y atajos. | Esta pagina |

## Contrato de la API

El contrato vive en `internal/openapi/openapi.yaml` (OpenAPI 3.1), va embebido
en el binario y no necesita configuracion:

- **Swagger UI:** `/docs` (con ejemplos listos para "Try it out").
- **Spec crudo:** `/openapi.yaml` (util para Postman o para generar clientes;
  el frontend genera sus tipos desde aqui).

Flujo tipico: `POST /api/v1/auth/login`, copiar el `access_token`, pulsar
**Authorize** y recorrer los ejemplos. Para un flujo encadenado completo usar
la coleccion de [`postman/`](postman/README.md).

## Entorno local

Requisitos: Docker y Docker Compose. No hace falta Go ni Node: todo se compila
dentro de Docker.

```bash
make deps                  # genera go.sum en un contenedor Go
docker compose up --build  # requiere un archivo .env en la raiz (lo leen api y worker)
```

En local MinIO reemplaza a Cloud Storage y Postgres corre en un contenedor; el
codigo es el mismo y solo cambia la configuracion (`internal/config`).

| Servicio | URL |
|---|---|
| Frontend web | http://localhost:3001 |
| API (health / Swagger) | http://localhost:8080/healthz · http://localhost:8080/docs |
| Bandeja de correo (Mailpit) | http://localhost:8025 |
| Consola de MinIO | http://localhost:9001 |
| Prometheus / Grafana | http://localhost:9090 · http://localhost:3000 |
| Jaeger (trazas) | http://localhost:16686 |
| PostgreSQL | localhost:5433 |

Las credenciales locales estan en
[`docs/entrega1/CREDENCIALES.md`](docs/entrega1/CREDENCIALES.md).

```bash
make up | make down | make logs | make psql
make reset                           # BORRA los volumenes
docker compose up --scale worker=3   # varios workers
```

> `db/init.sql` solo se ejecuta la primera vez que se crea el volumen de
> Postgres. Si se modifica, hay que correr `make reset` antes de volver a
> levantar.

## Uso de agentes de inteligencia artificial

El diseno y la implementacion se realizaron con asistencia de un agente de IA,
segun lo permite el enunciado del curso. Debe declararse en la sustentacion.
