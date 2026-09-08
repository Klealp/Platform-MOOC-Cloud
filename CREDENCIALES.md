# CREDENCIALES.md — Referencia rápida

Todas las credenciales del entorno local. Ninguna de estas es apta para
producción; el archivo `.env` real está en `.gitignore` y no se sube al
repositorio.

---

## 1. Administrador de la plataforma (API)

Lo crea automáticamente la API al arrancar (`auth.EnsureAdmin` en
`cmd/api/main.go`), leyendo `ADMIN_EMAIL` / `ADMIN_PASSWORD` de tu `.env`.

| Campo | Valor |
|---|---|
| Email | `admin@mooc.local` |
| Password | `Admin123!` |
| Rol | `admin` |

Login:
```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@mooc.local","password":"Admin123!"}'
```

> Si cambiaste `ADMIN_EMAIL` / `ADMIN_PASSWORD` en tu `.env` antes del primer
> arranque, esos son los valores reales — la API solo crea el admin la
> primera vez que no existe ninguna fila con ese email.

---

## 2. Usuarios que crea `testdata/smoke.sh`

El script genera usuarios **nuevos en cada ejecución**, usando la hora actual
(`$STAMP = date +%s`) para que el email nunca se repita. Las contraseñas sí
son fijas — están escritas en el script.

| Rol | Patrón de email | Password fija |
|---|---|---|
| Profesor | `profe<STAMP>@mooc.local` | `Profesor2026` |
| Estudiante principal | `estudiante<STAMP>@mooc.local` | `Estudiante2026` |
| Estudiante "intruso" (prueba de aislamiento) | `intruso<STAMP>@mooc.local` | `Intruso2026` |

`<STAMP>` es un timestamp Unix, por ejemplo `1788816015`. Para saber los
emails exactos de la última corrida, revisa la salida del script (los
imprime en el paso 2, 6 y 13) o consulta la base:

```bash
docker compose exec postgres psql -U mooc -d mooc \
  -c "SELECT email, role, status, created_at FROM users ORDER BY created_at DESC LIMIT 10;"
```

Para volver a iniciar sesión como el profesor o el estudiante de la última
corrida sin correr el script de nuevo, usa el email que veas en esa consulta
junto con la password fija de la tabla de arriba.

---

## 3. Servicios de infraestructura (Docker Compose)

| Servicio | URL | Usuario | Password |
|---|---|---|---|
| PostgreSQL | `localhost:5432` (db `mooc`) | `mooc` | `mooc` |
| MinIO — consola web | http://localhost:9001 | `minioadmin` | `minioadmin` |
| MinIO — API S3 | `localhost:9000` | `minioadmin` | `minioadmin` |
| Redis | `localhost:6379` | *(sin password)* | — |
| Mailpit — bandeja web | http://localhost:8025 | *(sin login)* | — |
| Prometheus | http://localhost:9090 | *(sin login)* | — |
| Grafana | http://localhost:3000 | `admin` | `admin` |
| pgAdmin | http://localhost:5050 | `admin@local.com` | `admin` |

**Conexión desde pgAdmin al servicio de Postgres** (host interno de Docker,
no `localhost`):
- Host: `postgres`
- Port: `5432`
- Database: `mooc`
- Username: `mooc`
- Password: `mooc`

---

## 4. Dónde está cada valor en el código, por si lo cambias

| Credencial | Archivo | Variable |
|---|---|---|
| Admin de la API | `.env` | `ADMIN_EMAIL`, `ADMIN_PASSWORD` |
| Postgres | `.env` y `docker-compose.yml` | `DATABASE_URL` / `POSTGRES_USER`, `POSTGRES_PASSWORD` |
| MinIO | `.env` y `docker-compose.yml` | `S3_ACCESS_KEY`, `S3_SECRET_KEY` / `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD` |
| Grafana | `docker-compose.yml` | `GF_SECURITY_ADMIN_USER`, `GF_SECURITY_ADMIN_PASSWORD` |
| pgAdmin | `docker-compose.yml` | `PGADMIN_DEFAULT_EMAIL`, `PGADMIN_DEFAULT_PASSWORD` |
| Usuarios de `smoke.sh` | `testdata/smoke.sh` | strings fijos junto a cada `curl -X POST .../register` |

---

## 5. Nota de seguridad

Todo esto vive en `.env` y en `docker-compose.yml` **en texto plano**, lo cual
es aceptable solo porque es un entorno local de desarrollo detrás de
`localhost`. Nada de esto debe copiarse tal cual a un despliegue real en la
nube (Entrega 2): ahí corresponde usar gestión de secretos externa (por
ejemplo Secret Manager de GCP o Secrets Manager de AWS), como ya se anticipa
en `DOCUMENTACION.md`, sección 8.
