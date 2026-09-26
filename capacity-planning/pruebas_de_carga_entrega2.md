# Pruebas de carga - Entrega 2

Informe de capacidad de la configuracion basica desplegada en Google Cloud (ver [`docs/entrega2/`](../docs/entrega2/README.md)).

> **Estado: plantilla.** Las secciones marcadas **PENDIENTE** se completan con las corridas reales. Los disenos de escenario de este documento son una propuesta que el equipo debe fijar **antes** de ejecutar, como exige el enunciado.

## 1. Herramienta y entorno de generacion

| Campo | Valor |
|---|---|
| Herramienta | k6 (PENDIENTE: version exacta, `k6 version`) |
| Justificacion | Escenarios con recorridos de varios pasos en JavaScript, `check()` para validar el resultado funcional de cada respuesta, ejecutores de tasa de llegada constante e incremental, y exportacion de resultados en JSON/CSV reproducibles |
| Ubicacion del generador | VM `loadgen` en `us-central1-a`, fuera de las dos VMs de la app, con tag `monitor` (PENDIENTE: tipo) |
| Verificacion de que no limita | CPU del generador < 70% y sin errores de red locales en cada corrida (PENDIENTE: evidencia) |
| Scripts | `capacity-planning/scripts/` (PENDIENTE) |
| Resultados originales | `capacity-planning/resultados/<escenario>/<corrida>/` (PENDIENTE) |

## 2. Infraestructura y condiciones fijas

| Condicion | Valor |
|---|---|
| Web Server | `e2-highcpu-2`, 30 GB pd-balanced |
| Worker Server | `e2-highcpu-2`, 30 GB pd-balanced |
| Cloud SQL | `db-custom-1-3840`, 10 GB SSD, zonal |
| Pool de conexiones | API 25 max / 10 idle; worker 25 max |
| Concurrencia del worker | 2 (fija en todas las corridas) |
| Cache | Sesiones en Redis, TTL 5 min |
| Limite de tasa | `RATE_LIMIT_PER_MIN` = PENDIENTE (con 120 por usuario se mide el limitador, no la plataforma: fijar y declarar el valor) |
| Commit evaluado | PENDIENTE |

Metricas recogidas en cada corrida: p50, p95, p99, throughput, errores y timeouts (k6); CPU, memoria, red y disco de las VMs (Ops Agent); conexiones y CPU de Cloud SQL; profundidad, antiguedad y tasa de procesamiento de la cola (asynq, PENDIENTE: definir fuente); metricas de la app `mooc_*` (Prometheus).

## 3. Datos sinteticos

PENDIENTE: script de siembra y cantidades declaradas (usuarios, cursos, recursos, inscripciones, intentos) y perfiles de archivos multimedia.

## 4. Escenario 1 - Actividad academica concurrente

### 4.1 Diseno (propuesta a fijar)

- **Sesiones:** se preparan antes de la corrida (un token por usuario virtual), para no atribuir el costo del login a la actividad academica. La rafaga de logins se mide como variante separada.
- **Recorrido por usuario virtual**, con cuentas distintas:
  1. `GET /api/v1/catalog`
  2. `GET /api/v1/catalog/:slug`
  3. `POST /api/v1/enrollments`
  4. `GET /api/v1/enrollments/:id/outline`
  5. `POST /api/v1/progress/events` (permanencia valida)
  6. `POST /api/v1/quizzes/:id/attempts`, `PATCH /api/v1/attempts/:id`, `POST /api/v1/attempts/:id/submit` con `Idempotency-Key`
  7. Reenvio del submit con la misma llave: se valida que no se recalifica
- **Validaciones funcionales:** estado de la inscripcion, `progress_pct` coherente, `score` esperado, `idempotent-replay: true` en el reenvio. Una respuesta 2xx sin el estado esperado cuenta como error.
- **Niveles:** linea base y al menos tres niveles crecientes, mas repeticion cerca del limite. PENDIENTE: tasas, calentamiento, duracion, pausas.
- **Exito y saturacion:** PENDIENTE (p. ej. p95 < X ms y errores < 1%).

### 4.2 Resultados

PENDIENTE.

### 4.3 Preguntas del enunciado

PENDIENTE: volumen sostenido y nivel de degradacion; operaciones que concentran latencia; integridad bajo concurrencia; cambio propuesto y medicion que lo respalda.

## 5. Escenario 2 - Carga, procesamiento y consumo multimedia

### 5.1 Diseno (propuesta a fijar)

- **Profesores:** `POST /uploads` -> `PUT` de cada parte directo a Cloud Storage -> `POST /uploads/:id/complete` -> sondeo de `GET /assets/:id` hasta `ready` o `failed`.
- **Estudiantes:** `GET /assets/:id/playlist` y segmentos HLS a la cadencia de reproduccion declarada (no descarga lo mas rapido posible).
- **Perfiles de archivo:** al menos tres (duracion, tamano, resolucion), sin escalar artificialmente la resolucion del original. PENDIENTE.
- **Mediciones separadas:** latencia de la API al emitir URLs; tiempo de transferencia directa; confirmacion; espera en cola; duracion de procesamiento; carga completa -> `ready`; trabajos por minuto, reintentos y fallos; drenaje de la cola al final.

### 5.2 Resultados

PENDIENTE.

## 6. Punto de degradacion, cuello de botella y limitaciones

PENDIENTE.

## 7. Propuesta de evolucion basada en evidencia

PENDIENTE.
