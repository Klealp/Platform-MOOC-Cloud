# CLAUDE.md

Contexto para Claude Code al trabajar sobre este repositorio.

## Que es esto

Backend de una plataforma MOOC (cursos masivos abiertos en linea) escrita en
Go. Es la **Entrega 1** de un proyecto academico del curso ISIS4426
"Desarrollo de Soluciones Cloud" (Universidad de los Andes). En esta entrega
**solo hay backend**: no existe frontend y no se debe crear uno salvo peticion
explicita.

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
  storage/        cliente del almacenamiento de objetos (API S3 / MinIO)
  tasks/          handlers de los workers
db/init.sql       esquema completo. Solo se ejecuta al CREAR el volumen
internal/openapi/ contrato de la API (openapi.yaml embebido; se sirve en /docs y /openapi.yaml)
deploy/           configuracion de Prometheus y Grafana
testdata/         smoke.sh y el archivo EICAR. No agregar mas archivos de prueba
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

## Comandos

```bash
make deps    # go mod tidy en un contenedor efimero (genera go.sum)
make up      # levanta todo
make reset   # BORRA los volumenes; obligatorio tras cambiar db/init.sql
make logs    # sigue los logs de api y worker
make psql    # consola SQL
./testdata/smoke.sh   # prueba de humo de extremo a extremo
```

## Deuda tecnica conocida

Cosas que estan asi a proposito y que no hay que "arreglar" sin hablarlo:

- El antimalware es un stub que detecta EICAR. La interfaz permite enchufar
  ClamAV mas adelante.
- Hay trazas OpenTelemetry (OTLP -> Jaeger) en la API (otelgin), el worker
  (un span por trabajo) y Postgres (otelsql), ademas de `X-Request-ID` y
  metricas. Pendiente: propagar el `traceparent` de la API dentro del trabajo
  encolado para unir en una sola traza la peticion HTTP y su procesamiento en
  el worker (hoy el span del worker es la raiz de su propia traza).
- El `Markdown extendido canonico` se normaliza (finales de linea y espacios).
  La biyeccion completa con el AST del editor es trabajo del frontend.
- El CDN no existe en local: `CDN_BASE_URL` vacio significa "firma contra
  MinIO". En la nube se activa cambiando esa variable.
- No hay pruebas unitarias todavia, solo la prueba de humo.
