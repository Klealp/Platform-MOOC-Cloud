# DOCUMENTACION — Plataforma MOOC (Entrega 1, solo backend)

Este documento explica **por que** el sistema esta construido asi y **que hace
cada archivo**. Esta pensado para que puedas defender cada decision en la
sustentacion y para que puedas modificar el codigo sabiendo que tocas.

Si vienes del proyecto `okf-platform` (conversion a bundles OKF), vas a
reconocer casi todo: la estructura de carpetas, el `db/init.sql` autoejecutado,
el `Makefile` con `make deps` y el estilo de comentarios son deliberadamente
los mismos. Lo que cambia es el dominio y tres piezas de infraestructura, que
se explican en la seccion 2.4.

---

## Indice

1. [El problema y la forma de la solucion](#1-el-problema-y-la-forma-de-la-solucion)
2. [Arquitectura](#2-arquitectura)
3. [Modelo de datos](#3-modelo-de-datos)
4. [Los siete mecanismos que hay que saber explicar](#4-los-siete-mecanismos-que-hay-que-saber-explicar)
5. [Archivo por archivo](#5-archivo-por-archivo)
6. [Recorrido completo del sistema](#6-recorrido-completo-del-sistema)
7. [Como se demuestra cada condicion verificable](#7-como-se-demuestra-cada-condicion-verificable)
8. [Que falta para la Entrega 2](#8-que-falta-para-la-entrega-2)

---

## 1. El problema y la forma de la solucion

El enunciado pide una plataforma donde profesores publican cursos con
contenido multimedia y evaluaciones, y estudiantes se inscriben, aprenden,
presentan quizzes y obtienen insignias verificables. Hasta 50.000 usuarios
registrados y 2.000 concurrentes.

Tres exigencias del enunciado condicionan toda la arquitectura:

**(a) Hay trabajo que no cabe en una peticion HTTP.** Transcodificar un video
de una hora puede tardar veinte minutos. Nadie va a dejar el navegador
esperando. Por eso existe una cola y un proceso worker aparte.

**(b) La API tiene que poder replicarse.** Con 2.000 usuarios concurrentes una
sola instancia no basta. Para poder levantar tres copias detras de un
balanceador, ninguna puede guardar nada propio: ni sesiones en memoria, ni
archivos temporales en su disco. Si la instancia 2 no puede atender una
peticion que empezo en la instancia 1, no se puede escalar.

**(c) El material es privado.** Un video de pago no puede quedar en una URL
publica adivinable. Pero tampoco puede pasar por la API, porque eso convertiria
al servidor en un cuello de botella. La respuesta a esa tension son las **URLs
prefirmadas**: la API verifica el derecho de acceso y luego emite un permiso
temporal para que el cliente hable directamente con el almacenamiento.

Casi todo lo demas se deriva de esos tres puntos.

---

## 2. Arquitectura

### 2.1 Los dos procesos

```
┌───────────────────────────────────────────────────────────────┐
│                         cmd/api                               │
│  Valida, consulta, escribe metadatos, firma URLs, ENCOLA.     │
│  Nunca transcodifica, nunca recibe los bytes de un archivo.   │
│  Escalable a N replicas sin coordinacion.                     │
└───────────────────────────────────────────────────────────────┘
                              │ publica trabajos
                              ▼
                    ┌───────────────────┐
                    │  Redis (asynq)    │
                    └───────────────────┘
                              │ consume
                              ▼
┌───────────────────────────────────────────────────────────────┐
│                        cmd/worker                             │
│  ffmpeg, escaneo antimalware, correo, insignias, limpieza.    │
│  Escalable por separado: docker compose up --scale worker=3   │
└───────────────────────────────────────────────────────────────┘
```

Que sean **dos binarios y no uno con hilos** es una decision, no un accidente.
Si fueran el mismo proceso, escalar para atender mas peticiones HTTP obligaria
a escalar tambien la capacidad de transcodificacion, y viceversa. Separandolos,
cada uno crece segun su propia demanda. Ademas un fallo de ffmpeg no puede
tumbar la API.

### 2.2 Monolito modular, no microservicios

El backend es **un solo binario de API** con el codigo separado por dominio
(`handlers_auth.go`, `handlers_courses.go`, `handlers_media.go`...). No son
microservicios y esto es intencional:

- Una sola transaccion de base de datos cubre operaciones que tocan varias
  tablas. Con microservicios habria que inventar transacciones distribuidas
  para algo que Postgres resuelve gratis.
- Un solo despliegue, un solo conjunto de logs, una sola version.
- Los limites entre modulos ya existen en el codigo, asi que si algun dia un
  modulo necesita escalar solo, se puede extraer.

Adoptar microservicios desde el dia uno significaria pagar su costo operativo
sin tener todavia el problema que resuelven.

### 2.3 Donde vive cada cosa

| Dato | Donde | Por que |
|---|---|---|
| Usuarios, cursos, progreso, notas | PostgreSQL | Necesita transacciones y restricciones de integridad |
| Sesiones (verdad) | PostgreSQL | Deben poder revocarse y sobrevivir a un reinicio |
| Sesiones (cache) | Redis | Evita ir a Postgres en cada peticion |
| Contadores de rate limit | Redis | Datos efimeros; perderlos no rompe nada |
| Cola de trabajos | Redis (asynq) | Lo exige el enunciado |
| Videos, PDF, HLS, insignias | MinIO (API S3) | Los binarios no van en una base relacional |
| Series de tiempo | Prometheus | Postgres guarda hechos; Prometheus guarda agregados |

**Regla que atraviesa todo el sistema:** Postgres es la fuente de verdad.
Si Redis se borra entero, el sistema sigue siendo correcto: las sesiones se
releen de Postgres y los trabajos perdidos se reencolan (`tasks/reap.go`).
Esto responde directamente a la recomendacion 3 del enunciado.

### 2.4 Que cambia respecto a `okf-platform`

| Pieza | okf-platform | mooc-platform | Por que |
|---|---|---|---|
| Cola | RabbitMQ | Redis + asynq | Lo exige el enunciado del MOOC |
| Autenticacion | JWT | Token opaco | Se exige revocacion inmediata; un JWT no se puede invalidar antes de expirar |
| Carga de archivos | multipart a la API | URLs prefirmadas | Videos de cientos de MB no pueden pasar por la API |
| Correo | no habia | Mailpit + worker | Verificacion de correo y recuperacion de contrasena |
| Frontend | nginx con HTML estatico | no hay | Esta entrega es solo backend |

Todo lo demas (Gin, `database/sql`, MinIO, la estructura `cmd/` + `internal/`,
Prometheus y Grafana, el `Makefile`) se reutiliza tal cual.

---

## 3. Modelo de datos

### 3.1 La jerarquia academica

```
Curso  ──▶  Version  ──▶  Modulo  ──▶  Unidad  ──▶  Recurso
                                                       │
                                            (si es quiz) ▼
                                                    Quiz ──▶ Pregunta ──▶ Opcion
```

El **curso** es la entidad estable: tiene slug, dueno y estado. La **version**
es lo que se publica; un curso puede tener varias, pero solo una publicada a
la vez (`courses.current_version_id`).

### 3.2 `stable_id`: la idea central del modelo

Cada modulo, unidad, recurso y pregunta tiene dos identificadores:

- `id` — la fila fisica. Cambia al clonar la version.
- `stable_id` — la identidad **logica**. Se copia tal cual al clonar.

El progreso del estudiante (`resource_progress`) apunta a `stable_id`, no a
`id`. Gracias a eso:

> El profesor corrige una falta de ortografia y publica la version 2. Se crean
> filas nuevas en `resources`, con `id` nuevos. Pero como los `stable_id` se
> conservaron, el estudiante que llevaba 7 de 10 recursos vistos sigue
> llevando 7 de 10.

Sin `stable_id`, publicar una correccion menor borraria el avance de todos los
estudiantes inscritos. Esta es la respuesta concreta al requisito
"identificadores estables para conservar progreso".

### 3.3 Inmutabilidad de lo publicado

`course_versions.status` recorre `draft → published → superseded`. Una version
publicada **nunca** se modifica: `s.guard()` rechaza con 409 cualquier
escritura sobre ella. Para cambiar algo se crea un borrador nuevo
(`POST /courses/{id}/versions`) que clona el arbol completo preservando
`stable_id`. Al publicarlo, la anterior pasa a `superseded` pero sigue
existiendo intacta, que es justamente lo que significa ser inmutable.

### 3.4 Tablas

| Tabla | Contenido |
|---|---|
| `users` | Identidad, rol (`admin`/`teacher`/`student`) y estado |
| `sessions` | Sesiones revocables. Guarda el **SHA-256** del token, nunca el token |
| `auth_tokens` | Tokens de un solo uso: verificar correo, restablecer contrasena |
| `audit_log` | Bitacora append-only |
| `assets` | Metadatos de cada binario y su llave en el almacenamiento |
| `uploads` | Estado de una carga multipart, para poder reanudarla 24 h |
| `courses`, `course_versions` | Curso y sus versiones |
| `modules`, `units`, `resources` | El arbol de contenido |
| `quizzes`, `quiz_questions`, `quiz_options` | Evaluaciones (solo texto) |
| `quiz_attempts` | Intentos, con snapshot y respuestas |
| `enrollments` | Inscripcion, estado y porcentaje calculado |
| `progress_events` | Evidencia cruda de avance (una fila por senal) |
| `resource_progress` | Estado agregado por recurso |
| `badges` | Insignias, con `public_code` aleatorio |
| `processed_jobs` | Capa 2 de idempotencia de los workers |

Dos detalles del esquema que vale la pena poder explicar:

- Las restricciones `UNIQUE (padre, position)` son **DEFERRABLE INITIALLY
  DEFERRED**. Al reordenar hay un instante en que dos elementos comparten
  posicion; diferir la verificacion al `COMMIT` permite renumerar sin trucos.
- `CREATE UNIQUE INDEX uniq_badge_alive ON badges(course_id, user_id) WHERE
  revoked_at IS NULL` es un **indice unico parcial**: impide dos insignias
  vivas para el mismo par, pero permite emitir una nueva si la anterior fue
  revocada.

---

## 4. Los siete mecanismos que hay que saber explicar

### 4.1 Aislamiento por propietario

**La regla va en el `WHERE` de la consulta, no en un `if` posterior.**

```sql
SELECT id, course_id FROM enrollments WHERE id = $1 AND user_id = $2
```

Si la condicion estuviera en Go, bastaria olvidarla en un handler para abrir un
agujero. En el `WHERE` no hay forma de esquivarla: la fila simplemente no
existe para quien no es su dueno.

Y cuando el recurso existe pero es ajeno, la respuesta es **404, no 403**.
Un 403 confirmaria que el identificador es real, y eso ya es informacion.

### 4.2 Idempotencia en tres capas

| Capa | Mecanismo | Que ataja |
|---|---|---|
| 1 | `asynq.TaskID("badge:<id>")` | Encolar dos veces el mismo efecto |
| 2 | Tabla `processed_jobs` | Reentrega despues de un reinicio de Redis |
| 3 | `uniq_badge_alive` en Postgres | Dos workers ejecutando a la vez |

La tercera es la unica infalible: las otras dos dependen de estado que puede
perderse. Por eso el codigo de `tasks/badge.go` trata la violacion de unicidad
(`23505`) como **exito**, no como error: significa que el efecto deseado ya
estaba logrado.

En la API, el mismo problema del lado HTTP se resuelve con la cabecera
`Idempotency-Key` (`internal/api/idempotency.go`): la primera peticion guarda
su respuesta en Redis y la segunda la repite sin volver a ejecutar nada.

### 4.3 Carga multipart directa y reanudable

```
1. POST /uploads                  → la API abre la carga y devuelve una URL
                                    prefirmada por cada parte
2. PUT <url-de-la-parte>          → el CLIENTE sube cada parte a MinIO
3. GET /uploads/{id}              → dice que partes faltan y firma URLs nuevas
4. POST /uploads/{id}/complete    → la API ensambla y encola el escaneo
```

El detalle importante del paso 3: la API **le pregunta al almacenamiento**
(`ListObjectParts`) que partes llegaron. No confia en lo que el cliente diga
haber subido. La fuente autoritativa es quien recibio los bytes.

### 4.4 Verificacion del archivo

El worker (`tasks/scan.go`) lee el objeto **una sola vez** y en esa pasada:

1. Calcula el SHA-256 real y lo compara con el declarado (**integridad**).
2. Guarda los primeros 512 bytes y aplica `http.DetectContentType`
   (**MIME real**). El cliente puede declarar `application/pdf` y subir un
   ejecutable; lo que vale son los bytes.
3. Busca la firma EICAR (**antimalware**). Se usa una ventana solapada porque
   la firma podria quedar partida entre dos bloques de lectura.

Si hay deteccion, **el objeto se borra** del almacenamiento y el asset queda
en `infected`. EICAR no es un virus: es un texto inofensivo que todos los
antivirus reconocen, creado justamente para probar la cadena de deteccion.

### 4.5 HLS y la entrega firmada

HLS parte el video en segmentos de 6 segundos y genera listas `.m3u8`. El
reproductor pide segmentos sueltos y cambia de calidad segun el ancho de banda.
Eso es la "reproduccion adaptativa" del enunciado.

Aparece entonces un problema real: **una URL prefirmada ampara un solo
objeto**. Si firmaramos solo el `.m3u8`, el reproductor pediria despues los
`.ts` y recibiria 403.

La solucion esta en `handleHLSPlaylist`: la API sirve la lista **reescrita al
vuelo**, sustituyendo cada nombre de segmento por su propia URL firmada. Asi el
bucket sigue siendo privado y el reproductor funciona.

Ademas, el transcodificador **no hace upscaling**: si el original es de 480p no
se genera una variante de 720p. Subir la resolucion no agrega informacion, solo
gasta CPU y almacenamiento.

### 4.6 Integridad del quiz

Al iniciar un intento se guarda un **snapshot** en `quiz_attempts.snapshot` con
dos partes:

```json
{
  "questions": [ ... ],          // lo que ve el estudiante
  "key": { "<pregunta>": ["<opcion-correcta>"] }   // NUNCA sale de la base
}
```

- `publicQuestions()` es la unica funcion que serializa hacia el cliente, y
  solo lee `questions`. `key` no tiene ningun camino de salida.
- El snapshot ademas **congela el examen**: si el profesor edita el quiz a
  mitad del intento, la calificacion usa lo que el estudiante vio.
- `gradeAttempt()` corre en el servidor y acredita una pregunta solo si el
  conjunto marcado coincide **exactamente** con el correcto. No hay puntaje
  parcial.
- Reenviar no recalifica: el `UPDATE` lleva `WHERE status = 'in_progress'`, asi
  que si otra peticion gano la carrera, afecta cero filas y se devuelve el
  resultado ya existente.

### 4.7 Progreso verificado en servidor

El cliente **no puede enviar porcentajes**. Solo manda senales:

| Senal | Significa |
|---|---|
| `open` | Abri el recurso |
| `heartbeat` | Sigo aqui (con `delta_secs`, tope 60) |
| `complete` | Creo que termine |

Y el servidor decide:

- Si el cuerpo trae `progress_pct` o `completed` → **422 y linea de auditoria**
  (`progress.rejected`).
- `delta_secs` se acota a 60 por evento: sin ese tope, un cliente enviaria
  999999 y "completaria" un video al instante.
- Un `complete` solo se acepta si el tiempo acumulado supera el umbral de
  `progress.MinDwellSeconds`: 80% de la duracion real para video y audio,
  15 segundos para lectura. La duracion viene de `ffprobe`, es decir **del
  archivo**, no del cliente.
- El porcentaje se recalcula siempre con `progress.Recompute()`, contando
  recursos obligatorios completados sobre el total.

`completed` significa "consumio el material exigido". `approved` anade "y
aprobo todos los quizzes obligatorios". Solo la transicion a `approved` dispara
la insignia, y solo la primera vez (`NewlyApproved`).

---

## 5. Archivo por archivo

### 5.1 Raiz

| Archivo | Que hace |
|---|---|
| `go.mod` | Modulo `mooc-platform`, Go 1.24 y las ocho dependencias. `go.sum` lo genera `make deps`. |
| `docker-compose.yml` | Los ocho servicios: api, worker, postgres, redis, minio, mailpit, prometheus, grafana. Con `healthcheck` y `depends_on: condition: service_healthy` para que la API no arranque antes que sus dependencias. |
| `Dockerfile.api` | Build en dos etapas. La imagen final es Alpine con solo el binario, sin compilador ni codigo fuente. Corre como usuario sin privilegios (uid 10001). |
| `Dockerfile.worker` | Igual, pero **con ffmpeg instalado**. Es la unica diferencia real entre las dos imagenes y explica por que son dos y no una. |
| `.env.example` | Plantilla de configuracion, comentada variable por variable. |
| `Makefile` | Atajos. `make deps` corre `go mod tidy` en un contenedor efimero para no depender del Go instalado en la maquina. |
| `README.md` | Instrucciones de despliegue y prueba. |
| `CLAUDE.md` | Contexto para Claude Code: restricciones que no se pueden romper, convenciones y deuda tecnica conocida. |

### 5.2 `db/init.sql`

El esquema completo: enums, tablas, indices y restricciones.

PostgreSQL lo ejecuta **solo la primera vez que se crea el volumen**
(`docker-entrypoint-initdb.d`). Es el error mas comun del proyecto: cambias el
SQL, levantas y no pasa nada. Solucion: `make reset` (`docker compose down -v`).

No incluye la semilla del administrador. Ponerla ahi habria exigido dejar un
hash de bcrypt fijo en el repositorio; en su lugar la crea `auth.EnsureAdmin`
al arrancar la API, cifrando la contrasena de `ADMIN_PASSWORD`.

### 5.3 `cmd/`

| Archivo | Que hace |
|---|---|
| `cmd/api/main.go` | Carga configuracion, abre Postgres y Redis, prepara el bucket, crea el administrador inicial, construye el router y sirve HTTP. Tiene **apagado ordenado**: al recibir SIGTERM deja terminar las peticiones en curso antes de cerrar, lo que evita respuestas cortadas durante un redespliegue. Los timeouts del `http.Server` son explicitos: sin ellos una conexion lenta puede retener recursos indefinidamente. |
| `cmd/worker/main.go` | Construye `tasks.Deps`, registra los handlers en el `ServeMux` de asynq, levanta el servidor de la cola y un **Scheduler** que encola el mantenimiento cada 5 minutos. Expone `/metrics` en el puerto 9100. Los pesos de las colas (critical 6, default 3, low 1) hacen que un correo de verificacion no espere detras de una transcodificacion de dos horas. |

### 5.4 `internal/config/config.go`

Un unico `struct Config` y una funcion `Load()`. **Ningun otro archivo del
proyecto llama a `os.Getenv`.** Esa disciplina es lo que permite que la misma
imagen funcione en local y en la nube cambiando solo el entorno.

`Describe()` se imprime al arrancar y nunca incluye secretos.

### 5.5 `internal/database/`

| Archivo | Que hace |
|---|---|
| `postgres.go` | Abre la conexion con **30 reintentos**. Docker Compose arranca los contenedores casi a la vez y Postgres tarda en inicializarse. Fija limites del pool: sin `SetMaxOpenConns` cada peticion concurrente puede abrir una conexion y agotar `max_connections`. |
| `redis.go` | Lo mismo para Redis. El comentario de cabecera enumera los tres papeles de Redis y aclara que ninguno es fuente de verdad. |

### 5.6 `internal/storage/storage.go`

Envuelve el cliente de MinIO. **Ningun handler ni worker habla directamente con
MinIO**: todos pasan por aqui. Eso es lo que hace que migrar a Google Cloud
Storage en la Entrega 2 sea escribir un archivo nuevo con los mismos metodos,
en vez de tocar el dominio entero.

Metodos: `EnsureBucket`, `NewMultipartUpload`, `PresignPartURL`, `ListParts`,
`CompleteMultipartUpload`, `AbortMultipartUpload`, `PutBytes`, `GetRange`,
`OpenStream`, `DownloadFile`, `UploadFile`, `StatSize`, `Remove`, `PresignGet`.

Detalle util: `rewriteHost()`. Dentro de Docker el endpoint es `minio:9000`,
nombre que solo resuelve entre contenedores; tu navegador necesita
`localhost:9000`. Como la firma S3 se calcula sobre la ruta y la query y no
sobre el host, se puede reescribir sin invalidarla.

### 5.7 `internal/queue/queue.go`

El **contrato** entre API y workers: los cinco tipos de tarea, las tres colas
de prioridad y las estructuras de payload.

`Enqueue()` trata `ErrTaskIDConflict` como exito. Que la llave ya exista no es
un fallo: significa que el trabajo ya estaba pedido, que es exactamente lo que
se buscaba.

### 5.8 `internal/auth/auth.go`

Contrasenas con bcrypt, tokens opacos y sesiones.

Lo importante para la sustentacion es la decision de **no usar JWT**. Un JWT es
autocontenido: el servidor no consulta nada para validarlo, y por eso no se
puede invalidar antes de que expire sin montar una lista negra, que es
precisamente el estado que el JWT pretendia evitar. El enunciado exige
"revocacion inmediata de sesiones", asi que un token opaco respaldado por una
fila en Postgres es la opcion correcta.

La cache en Redis dura 5 minutos, pero `RevokeSession` **borra la llave de la
cache** ademas de marcar la fila. Por eso la revocacion sigue siendo inmediata.

`ConsumeAuthToken` valida y marca como usado **en una sola sentencia**
(`UPDATE ... WHERE used_at IS NULL RETURNING`), de modo que dos peticiones
simultaneas no puedan canjear el mismo token.

### 5.9 `internal/audit/audit.go`

Bitacora append-only con constantes para cada accion.

Regla explicita: **la auditoria nunca hace fallar la operacion principal**. Si
no se puede escribir, se registra en el log y se sigue. Perder una linea de
auditoria es malo; perder la operacion del usuario es peor.

### 5.10 `internal/metrics/metrics.go`

Las series de Prometheus, agrupadas en tres bloques: HTTP, dominio y workers.

Incluye `mooc_transcoded_media_minutes_total`, que es el insumo directo de la
recomendacion 2 del enunciado (medir el costo por minuto de transcodificacion
para decidir entre FFmpeg propio y un servicio gestionado).

### 5.11 `internal/api/` — capa HTTP

| Archivo | Que hace |
|---|---|
| `router.go` | El `Server` con sus dependencias y **la tabla completa de rutas**. Es el mejor punto de partida para leer el proyecto: de un vistazo se ve que es publico, que exige sesion y que exige rol. Incluye `/healthz` (el proceso vive) y `/readyz` (ademas sus dependencias responden). |
| `errors.go` | El formato uniforme de error y los ayudantes (`badRequest`, `notFound`, `conflict`, `unprocessable`, `internalError`). `internalError` registra el error real en el servidor y devuelve al cliente un mensaje generico: los detalles internos no salen nunca. |
| `middleware.go` | `X-Request-ID`, log y metricas, recuperacion de panics, cabeceras de seguridad, `requireAuth` y `requireRole`. Las metricas usan `c.FullPath()` (el patron de ruta) y no la URL concreta, porque si no cada identificador crearia una serie de tiempo nueva y Prometheus reventaria en cardinalidad. |
| `ratelimit.go` | Ventana fija por minuto en Redis (`INCR` + `EXPIRE`). Si Redis no responde **no bloquea el trafico**: el limite de tasa es una proteccion, no una funcion critica. |
| `idempotency.go` | La cabecera `Idempotency-Key`. Guarda la respuesta de la primera peticion y repite esa misma respuesta ante un reintento. Verifica que el cuerpo sea identico, y no memoriza respuestas 5xx (esas si deben poder reintentarse). |
| `handlers_auth.go` | Registro, verificacion, login, logout, sesiones, recuperacion. La respuesta del registro es la misma exista o no el correo, para impedir enumerar cuentas. En `APP_ENV=dev` incluye `dev_verification_token` para poder probar con Postman sin abrir Mailpit; en produccion nunca aparece. |
| `handlers_admin.go` | Usuarios y auditoria. Contiene la **proteccion del ultimo administrador**: la comprobacion va dentro de una transaccion con `SELECT ... FOR UPDATE`, para que dos administradores simultaneos no puedan degradarse mutuamente y dejar el sistema sin nadie que lo administre. |
| `handlers_courses.go` | Cursos, versiones, clonacion y publicacion. Aqui viven `resolveAuthoring()` y `guard()`, que resuelven propiedad e inmutabilidad para cualquier nodo del arbol, y `validateForPublication()`, que devuelve la **lista exhaustiva** de problemas y no solo el primero. |
| `handlers_content.go` | Modulos, unidades, recursos, autosave y `buildOutline()`. El autosave admite `expected_revision` para detectar ediciones concurrentes. `buildOutline` arma el arbol con **tres consultas** en vez de una por nodo: evitar el N+1 importa porque este endpoint lo llama cada estudiante al abrir un curso. |
| `handlers_media.go` | Carga multipart, consulta de assets, entrega firmada y la lista HLS reescrita. `sanitizeFilename` evita que un nombre como `../../etc/x` se convierta en una ruta arbitraria dentro del bucket. |
| `handlers_quiz_authoring.go` | Autoria de preguntas. `validate()` rechaza preguntas con menos de dos opciones, sin respuesta correcta, o con varias correctas sin `multiple`. Actualizar una pregunta la **reemplaza completa**: es mas simple y menos propenso a errores que parchear una lista ordenada. |
| `handlers_quiz_attempts.go` | Intentos: snapshot, guardado parcial, calificacion. La pieza mas sensible del proyecto; ver 4.6. |
| `handlers_learning.go` | Catalogo con cursor, inscripcion, retiro y eventos de progreso. Usa cursor en vez de `OFFSET` porque con `OFFSET`, si alguien publica un curso mientras el usuario pagina, las filas se desplazan y se repiten o se pierden. |
| `handlers_badges.go` | Insignias propias, verificacion publica y revocacion. Revocar **no borra**: alguien puede tener el enlace y necesita saber que ya no es valido. |

### 5.12 `internal/progress/progress.go`

Las reglas de avance en un paquete propio porque las usan **dos procesos**: la
API (al recibir un evento o calificar un quiz) y el worker (al reevaluar una
inscripcion). Una sola implementacion evita que cada uno calcule un porcentaje
distinto.

`Recompute()` devuelve `NewlyApproved`, que es lo que decide si se encola la
insignia. Sin ese campo habria que consultar antes y despues para saber si la
aprobacion acaba de ocurrir.

### 5.13 `internal/badge/render.go`

Genera el SVG de la insignia. SVG y no PNG porque es texto (se versiona e
inspecciona facilmente), no necesita fuentes instaladas en el contenedor y no
agrega dependencias.

### 5.14 `internal/tasks/` — los workers

| Archivo | Que hace |
|---|---|
| `tasks.go` | `Deps`, el registro de handlers, los ayudantes de idempotencia (`alreadyProcessed`, `markProcessed`), el envoltorio `instrument()` que mide duracion y resultado, y el `ErrorHandler`. Cuando se agotan los reintentos, asynq mueve la tarea a la cola **archived**: esa es la DLQ. El `ErrorHandler` deja ademas una linea `job.dead_lettered` en la auditoria, que es la alerta que pide el enunciado. |
| `email.go` | Envio por SMTP. Un payload corrupto devuelve `asynq.SkipRetry` porque no va a mejorar reintentando. |
| `scan.go` | Integridad, MIME real y antimalware en una sola lectura del objeto. Ver 4.4. |
| `transcode.go` | `ffprobe` para duracion y altura, escalera de calidades **sin upscaling**, generacion de HLS, lista maestra y subida de los derivados. El original nunca se toca. |
| `badge.go` | Emision idempotente. Trata la violacion de unicidad como exito. `newPublicCode()` genera un codigo aleatorio: si fuera el id del usuario o un consecutivo, cualquiera podria enumerar las insignias de la plataforma. |
| `reap.go` | El mantenimiento periodico: aborta cargas vencidas, reencola escaneos y transcodificaciones huerfanas, emite insignias faltantes, expira intentos vencidos y limpia sesiones caducadas. Responde directamente a la recomendacion del enunciado de "reencolar trabajos sin heartbeat para mitigar perdidas en Redis". |

### 5.15 `deploy/` y `testdata/`

- `deploy/prometheus.yml` — usa `dns_sd_configs` para los workers, no una lista
  fija. Al escalar con `--scale worker=3`, Docker crea tres contenedores y el
  DNS interno devuelve las tres IPs bajo el mismo nombre; con una lista
  estatica solo se veria uno.
- `deploy/grafana/datasource.yml` — provisionamiento por archivo: Grafana
  arranca con la fuente de datos lista, sin pasos manuales.
- `testdata/smoke.sh` — recorrido completo con `curl`.
- `testdata/eicar.txt` — el archivo de prueba del antivirus.

---

## 6. Recorrido completo del sistema

**El profesor sube un video y lo publica**

1. `POST /uploads` → la API crea el `asset` en `awaiting_upload`, abre la carga
   multipart y devuelve una URL prefirmada por parte.
2. El cliente hace `PUT` de cada parte **directo a MinIO**.
3. Si se corta, `GET /uploads/{id}` pregunta al almacenamiento que llego y
   firma URLs nuevas solo para lo que falta.
4. `POST /uploads/{id}/complete` → ensambla, pasa a `uploaded` y **encola**
   `media:scan`. La respuesta es inmediata (202).
5. El worker verifica checksum, MIME y antimalware → `processing` y encola
   `media:transcode`.
6. El worker descarga el original, corre `ffprobe` y `ffmpeg`, sube los
   segmentos HLS, escribe `duration_secs` y pasa el asset a `ready`.
7. El profesor crea el recurso apuntando al asset, y publica la version.
   `validateForPublication` comprueba, entre otras cosas, que el asset este en
   `ready`: no se puede publicar un curso con un video a medio procesar.

**El estudiante aprende y aprueba**

1. `GET /catalog` → `POST /enrollments`.
2. `GET /enrollments/{id}/outline` → el arbol visible mas su progreso.
3. `GET /assets/{id}/url` → la API verifica la inscripcion activa y firma.
   Para video devuelve `playlist_url`, que apunta al endpoint que reescribe la
   lista HLS con un segmento firmado por linea.
4. `POST /progress/events` con `open`, luego `heartbeat` cada 30 s, luego
   `complete`. El servidor valida el tiempo minimo antes de aceptarlo.
5. `POST /quizzes/{id}/attempts` → snapshot sin claves.
6. `POST /attempts/{id}/submit` con `Idempotency-Key` → calificacion en
   servidor.
7. Si con eso queda `approved`, se encola `badge:issue`. El worker inserta la
   insignia (protegida por el indice unico), genera el SVG y envia el correo.
8. Cualquiera puede verificar la insignia en `/public/badges/{code}` sin
   sesion y sin ver el correo del estudiante.

---

## 7. Como se demuestra cada condicion verificable

| Condicion del enunciado | Donde esta | Como se demuestra |
|---|---|---|
| Publicacion valida | `validateForPublication` | Publicar un curso incompleto → 422 con la lista de problemas |
| Procesamiento asincrono | `handleCompleteUpload` + `tasks/transcode.go` | Subir un video: respuesta 202 inmediata; el asset pasa de `uploaded` a `ready` en segundo plano |
| Tolerancia a fallos | `tasks.go` + `ErrorHandler` | Apagar MinIO y subir: reintentos con backoff, luego cola `archived` y linea `job.dead_lettered` |
| Sin duplicados | `processed_jobs` + `uniq_badge_alive` | Reencolar `badge:issue` a mano: en el log aparece "ya fue procesado" y la tabla `badges` no cambia |
| Integridad del quiz | `attemptSnapshot.Key` | El JSON del intento no contiene `is_correct` por ninguna parte |
| Envio idempotente | `idempotency.go` | Reenviar con la misma `Idempotency-Key` → cabecera `Idempotent-Replay: true` y misma nota |
| Progreso verificable | `handleProgressEvent` | Enviar `progress_pct: 100` → 422 y linea `progress.rejected` en la auditoria |
| Control de acceso | `WHERE ... user_id = $` | El estudiante B pide la inscripcion de A → 404 |
| Emision de insignia | `tasks/badge.go` | Una sola fila en `badges`; `/public/badges/{code}` no muestra el correo |
| Ultimo administrador | `handleAdminUpdateUser` | Degradar al unico admin → 409 |

Los puntos 5, 6, 7 y 8 los ejecuta automaticamente `./testdata/smoke.sh`.

---

## 8. Que falta para la Entrega 2

Cosas conscientemente fuera de alcance, utiles para el segmento de
"limitaciones conocidas" del video:

- **Frontend.** No existe. Esta entrega es solo backend.
- **Despliegue en la nube.** El cambio principal sera sustituir MinIO por
  almacenamiento gestionado (GCS o S3) y activar `CDN_BASE_URL`. Como todo el
  acceso pasa por `internal/storage`, el cambio queda contenido en un archivo.
- **ClamAV real.** Hoy el escaneo detecta EICAR; la interfaz ya esta lista para
  enchufar un demonio real.
- **Trazas distribuidas (OpenTelemetry).** Hay `X-Request-ID` y metricas de
  Prometheus, pero no trazas correlacionadas entre API y worker.
- **Pruebas automaticas.** Solo esta la prueba de humo. Faltan pruebas
  unitarias de `gradeAttempt`, `Recompute` y `validateForPublication`, que son
  las tres funciones con logica no trivial.
- **Pruebas de carga y objetivos de latencia.** El histograma
  `mooc_http_request_duration_seconds` ya permite medir el p95; falta la
  campana de carga que lo ponga a prueba.
- **Alcance opcional del enunciado**: conversion de PPTX/ODP, iframes con lista
  blanca, editor con tablas y formulas, coautoria, subtitulos, foros y
  Open Badges 3.0.
