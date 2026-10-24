# CLAUDE.md

Contexto para Claude Code al trabajar sobre este repositorio.

## Que es esto

Plataforma MOOC (cursos masivos abiertos en linea) del curso ISIS4426
"Desarrollo de Soluciones Cloud" (Universidad de los Andes). Backend en Go
(monolito modular + workers asincronos) y frontend Next.js que actua como BFF.
Las Entregas 1 (backend local) y 2 (despliegue en Google Cloud) estan
terminadas; el proyecto se prepara para la **Entrega 3**.

- La arquitectura vigente es la de [`docs/entrega2/README.md`](docs/entrega2/README.md).
  La arquitectura logica (modulos, reglas, modelo de datos) sigue en
  [`docs/entrega1/ARQUITECTURA.md`](docs/entrega1/ARQUITECTURA.md) y la
  documentacion archivo por archivo en `docs/entrega1/DOCUMENTACION.md`.
- La Entrega 2 **no cambio el backend**: pasar de MinIO a Cloud Storage y de
  Postgres local a Cloud SQL fue solo configuracion. Mantenerlo asi: cualquier
  diferencia entre local y nube va en variables de entorno, no en codigo.
- El frontend (`frontend/`) tiene su propio README y su propio stack
  (TypeScript). No modificarlo al trabajar en el backend salvo peticion
  explicita; si cambia el contrato OpenAPI, avisar que el frontend regenera
  sus tipos con `npm run gen:api`.

Un proyecto anterior del mismo autor, `okf-platform` (conversion de documentos
a bundles OKF), sirvio de referencia: se conservan a proposito su estructura de
carpetas, su estilo de comentarios en espanol y sus convenciones de despliegue.

## Idioma

**Todo el codigo, los comentarios, los mensajes de error, los commits y las
respuestas se escriben en espanol.** Los comentarios evitan tildes para no
depender de la codificacion del terminal. Los mensajes de error de la API van
dirigidos a un desarrollador y son concretos.

## Restricciones que NO se pueden romper

Vienen del enunciado del curso y son criterios de calificacion:

1. **La API no guarda estado local.** Nada en memoria entre peticiones, nada
   en el disco del contenedor. Si algo necesita persistir va a Postgres, a
   Redis o al almacenamiento de objetos.
2. **Los archivos no pasan por la API.** Se suben con URLs prefirmadas
   directamente al almacenamiento y se descargan igual.
3. **Postgres es la fuente de verdad.** Redis solo tiene cache, contadores de
   rate limit y la cola. Si Redis se vacia, el sistema debe seguir siendo
   correcto.
4. **Ningun binario en la base relacional.** Solo la llave del objeto.
5. **El progreso lo calcula el servidor.** Un porcentaje enviado por el
   cliente se rechaza con 422 y se audita.
6. **La clave de un quiz nunca sale hacia el cliente.** Vive en el campo `key`
   del snapshot y solo la lee `gradeAttempt`.
7. **Una version publicada es inmutable.** Para editar se crea una version
   nueva clonando la anterior y conservando los `stable_id`.
8. **El acceso a un recurso ajeno responde 404, no 403.**
9. **Los quizzes son de seleccion multiple SOLO DE TEXTO.** Sin imagenes,
   video ni archivos en enunciados ni en opciones. No agregar ese soporte sin
   que lo pidan.
10. **Los profesores solo los crea un administrador.** El registro publico
    siempre produce un `student`.
11. **Siempre debe quedar al menos un administrador activo.**

## Mapa del repositorio

```
cmd/api/          proceso HTTP (no hace trabajo pesado)
cmd/worker/       proceso que consume la cola (ffmpeg, escaneo, correo, insignias)
cmd/queue-metrics/ publica pendientes + activos de la cola en Cloud Monitoring
                  (autoescalado de workers, Entrega 3). Una sola instancia, en
                  infra-server
internal/
  api/            handlers, middlewares, rutas. Un archivo handlers_*.go por dominio
  auth/           contrasenas, tokens opacos, sesiones revocables
  audit/          bitacora append-only
  badge/          generacion del SVG de la insignia
  config/         TODAS las variables de entorno se leen aqui y en ningun otro lugar
  database/       conexiones a Postgres y Redis con reintentos
  mailer/         SMTP (Mailpit en local)
  metrics/        series de Prometheus
  progress/       reglas de avance, finalizacion y aprobacion
  queue/          contrato de tareas entre API y workers (asynq)
  queuemetrics/   medicion de la cola y escritura en Cloud Monitoring (REST +
                  token de metadatos, sin el SDK de Google)
  storage/        cliente del almacenamiento de objetos (API S3: MinIO en local, Cloud Storage en la nube)
  tasks/          handlers de los workers
  telemetry/      arranque del exportador OpenTelemetry
db/init.sql       esquema completo. Solo se ejecuta al CREAR el volumen
internal/openapi/ contrato de la API (openapi.yaml embebido; se sirve en /docs y /openapi.yaml)
frontend/         Next.js 15 (BFF). Ver frontend/README.md
deploy/           Prometheus, Grafana y exportadores para el entorno local
deploy/gcp/       provision.sh, startup.sh, migrate.sh, cors.json y un compose
                  por VM: web/ (api, web, caddy), worker/ (worker, redis,
                  mailpit), monitor/ (prometheus, exportadores, k6.sh)
deploy/gcp/mig/   Entrega 3: startup.sh y shutdown.sh de las VMs de los
                  grupos y un compose + .env.example por rol (api, worker,
                  web, infra). build.sh sube las imagenes a Artifact
                  Registry; provision.sh (elastica, grupos, balanceador,
                  desplegar) crea la infraestructura. Los archivos de la
                  Entrega 2 se conservan para reconstruir la linea base
capacity-planning/ pruebas de carga k6 e informe de capacidad (escenarios 1 y 2)
postman/          coleccion de la API y entornos local y nube
docs/entrega1/    ARQUITECTURA.md, DOCUMENTACION.md, CREDENCIALES.md (locales)
docs/entrega2/    documento de arquitectura del despliegue en GCP y diagramas
docs/entrega1/testdata/
                  smoke.sh, otros smoke_*.sh (cada uno demuestra un
                  comportamiento concreto: un formato de archivo, un
                  camino de falla, una regla de negocio). smoke_eicar.sh
                  genera el archivo EICAR al ejecutarse.
                  Agregar un smoke_*.sh nuevo esta bien cuando cubre un
                  comportamiento que los demas no cubren; no crear
                  variantes redundantes de uno ya existente ni agregar
                  binarios de prueba sueltos sin un script que los use.
                  Mantener docs/entrega1/testdata/README.md al dia con la tabla de
                  que hace cada script.
```

## Convenciones de codigo

- Los handlers son metodos sobre `*api.Server`, que tiene las dependencias.
- Errores de API: usar los ayudantes de `internal/api/errors.go`
  (`badRequest`, `notFound`, `conflict`, `unprocessable`, `internalError`).
  Nunca escribir un JSON de error a mano.
- **El aislamiento por propietario va en el `WHERE` de la consulta**, no en un
  `if` posterior. Es la regla mas importante del proyecto: si esta en el SQL,
  no hay camino que la omita.
- Para autoria, usar `s.guard(c, nivel, id)`: resuelve propiedad e
  inmutabilidad de una vez. Los niveles son `course`, `version`, `module`,
  `unit`, `resource`, `quiz`, `question`.
- Los comentarios explican **por que**, no **que**. El codigo ya dice que hace.
- Consultas SQL en linea con `database/sql`. No hay ORM y no se va a agregar.
- Evitar el problema N+1: ver `buildOutline`, que arma el arbol con tres
  consultas.

## Idempotencia (tres capas, a proposito)

1. `asynq.TaskID(llave)` impide encolar dos veces el mismo efecto.
2. La tabla `processed_jobs` registra el trabajo terminado en Postgres.
3. Restricciones de integridad, como `uniq_badge_alive`. Esta es la unica que
   no se puede esquivar en una carrera entre dos workers.

Al agregar una tarea nueva, implementar las tres.

## Cambios que exigen recrear el volumen

`db/init.sql` solo se ejecuta la **primera** vez que se crea el volumen de
Postgres. Despues de tocarlo hay que correr `make reset` (`docker compose
down -v`). Avisarlo siempre que se modifique ese archivo.

## Despliegue en Google Cloud (Entrega 2)

- **Web Server** (`e2-highcpu-2`, subred publica): Caddy (unico servicio
  expuesto, TLS con Let's Encrypt sobre `sslip.io`), la API, el frontend y el
  monitoreo (Prometheus + exportadores, solo localhost). Caddy envia
  `/api/v1/*`, `/docs`, `/openapi.yaml`, `/healthz` y `/readyz` a la API y el
  resto al frontend.
- **Worker Server** (`e2-highcpu-2`, subred privada, sin IP publica): worker,
  Redis (solo alcanzable desde el Web Server) y Mailpit.
- **Cloud SQL** PostgreSQL 16 con IP privada y TLS obligatorio, y **Cloud
  Storage** con bucket privado accedido por la API XML compatible con S3.
- Cada componente tiene su propia llave HMAC: `mooc-api` no puede borrar
  objetos; `mooc-worker` si (archivos infectados). No usar una sola llave.
- La API Go no usa cookies (sesion en `Authorization: Bearer`, inmune a CSRF
  por construccion). La cookie HttpOnly es solo del BFF.
- Configuracion: en local `.env` en la raiz (no versionado, lo leen `api` y
  `worker` con `env_file`); en la nube `/etc/mooc/*.env` generados por
  `provision.sh config` a partir de las plantillas `deploy/gcp/*/*.env.example`.
  Nunca versionar secretos.
- `WORKER_CONCURRENCY=2` y `MAX_RETRIES=3` se mantuvieron fijos en las pruebas
  de capacidad; cambiarlos invalida la comparacion con el informe.

## Comandos

```bash
make deps    # go mod tidy en un contenedor efimero (genera go.sum)
make up      # levanta todo
make reset   # BORRA los volumenes; obligatorio tras cambiar db/init.sql
make logs    # sigue los logs de api y worker
make psql    # consola SQL
./docs/entrega1/testdata/smoke.sh   # prueba de humo de extremo a extremo

# Nube (ver docs/entrega2/README.md seccion 6)
./deploy/gcp/provision.sh todo|config|resumen
sudo docker compose -f deploy/gcp/<web|worker>/docker-compose.yml up -d --build
./deploy/gcp/migrate.sh init-db|dump-db|restore-db|objects|verify
```

En la nube se levanta primero el Worker Server: la API necesita Redis al
arrancar.

## Deuda tecnica conocida

Cosas que estan asi a proposito y que no hay que "arreglar" sin hablarlo:

- El antimalware es un stub que detecta EICAR. La interfaz permite enchufar
  ClamAV mas adelante.
- Hay trazas OpenTelemetry (OTLP -> Jaeger) en la API (otelgin), el worker
  (un span por trabajo) y Postgres (otelsql), ademas de `X-Request-ID` y
  metricas. Pendiente: propagar el `traceparent` de la API dentro del trabajo
  encolado para unir en una sola traza la peticion HTTP y su procesamiento en
  el worker (hoy el span del worker es la raiz de su propia traza). En la
  nube las trazas estan apagadas (`OTEL_EXPORTER_OTLP_ENDPOINT` vacio) porque
  Jaeger no cabe en 2 GiB junto a la aplicacion.
- El `Markdown extendido canonico` se normaliza (finales de linea y espacios).
  La biyeccion completa con el AST del editor es trabajo del frontend.
- No hay CDN ni en local ni en la nube: `CDN_BASE_URL` vacio significa
  "firma contra el almacenamiento" (MinIO o Cloud Storage). Se activa
  cambiando esa variable.
- Fuera del alcance de la Entrega 2 por enunciado: balanceador y replicas de
  la API, autoescalado de workers, BD con alta disponibilidad y Redis
  administrado. Son las candidatas a la evolucion elastica (seccion 7.4 de
  `docs/entrega2/README.md`); el informe de capacidad decide cual aporta mas.
- `GET /resources/:id/content` esta restringido a profesor/admin, asi que un
  estudiante no puede leer el Markdown de un recurso `rich_text` (ver
  `frontend/README.md`, "Gap conocido del backend").
- Los smoke tests no leen el token de verificacion de correo desde Mailpit;
  en produccion (`APP_ENV` distinto de `dev`) hay que leerlo a mano.
- Casi no hay pruebas unitarias: solo `internal/queuemetrics` (`go test
  ./internal/queuemetrics/`). El resto se cubre con la prueba de humo.
