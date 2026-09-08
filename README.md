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

| Servicio            | URL                              | Credenciales        |
|---------------------|----------------------------------|---------------------|
| API                 | http://localhost:8080/healthz    | —                   |
| Bandeja de correo   | http://localhost:8025            | —                   |
| Consola de MinIO    | http://localhost:9001            | minioadmin / minioadmin |
| Prometheus          | http://localhost:9090            | —                   |
| Grafana             | http://localhost:3000            | admin / admin       |
| PostgreSQL          | localhost:5432                   | mooc / mooc         |

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

## Contrato de la API

`api/openapi.yaml` (OpenAPI 3.1). Se puede abrir en
[editor.swagger.io](https://editor.swagger.io) para navegarlo.

## Uso de agentes de inteligencia artificial

El diseno y la implementacion de este backend se realizaron con asistencia de
un agente de IA, segun lo permite el enunciado del curso. Debe declararse en
la sustentacion.
