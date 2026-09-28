# Informe de capacidad - Entrega 2

Analisis de capacidad de la plataforma MOOC sobre el despliegue basico en la
nube publica. Sigue el formato exigido por el enunciado.

## 0. Resumen ejecutivo

- Capacidad sostenida (Escenario 1): la plataforma responde con latencia baja y
  recursos holgados en los niveles bajo y medio (hasta ~50 usuarios/s de
  llegada, con CPU del Web < 15%). En el nivel alto se degrada.
- Nivel donde inicia la degradacion: el **nivel alto** (objetivo 90 usuarios/s
  de llegada). Ahi la latencia p95 sube a ~2,9 s y aparecen errores 5xx.
- Cuello de botella identificado: **CPU del Web Server** (la API en Go), que se
  satura a ~98% en el nivel alto. Factor secundario: el pool de conexiones a
  PostgreSQL se estanca en ~31.
- Propuesta de evolucion de mayor impacto: escalar el Web Server (mas vCPU o mas
  replicas detras de un balanceador) y ampliar el pool de conexiones a la BD.

---

## 1. Escenario 1 - Actividad academica concurrente

### 1.1 Definicion del escenario y niveles de carga

- **Recorrido (usuarios virtuales):** consultar el catalogo (listado y detalle),
  abrir el esquema de la inscripcion, registrar progreso valido sobre un recurso
  no-quiz (open + heartbeat + complete) y presentar un quiz (crear intento +
  enviar, validando la nota). Cada usuario usa una cuenta distinta y un nuevo intento cada vez.
- **Endpoints involucrados:** `GET /catalog`, `GET /catalog/{slug}`,
  `GET /enrollments/{id}/outline`, `POST /progress/events`,
  `POST /quizzes/{id}/attempts`, `POST /attempts/{id}/submit`.
- **Mezcla de operaciones:** lecturas (catalogo, outline) + escrituras (eventos
  de progreso, envio de quiz).
- **Datos sinteticos:** 20 estudiantes activos e inscritos, 1 curso publicado
  (1 modulo, 3 lecturas obligatorias + 1 quiz de 5 preguntas x 4 opciones),
  20 inscripciones.
- **Patron de inyeccion:** tasa de llegada creciente (open model).
- **Niveles (usuarios/s de llegada):** 5 (base) -> 20 -> 50 -> 90 -> 90 (rep) -> 5.
- **Calentamiento, duracion, pausas:** base 1 min; 2 min por nivel; pausas de
  1-3 s entre acciones dentro de cada recorrido.
- **Criterios de exito, saturacion y parada:** exito = latencia acotada y sin
  errores 5xx; saturacion = CPU de una VM cercana al 100%, latencia p95/p99 en
  segundos y aparicion de 5xx; parada = alcanzada la saturacion.
- **Autenticacion:** las sesiones se preparan antes de la corrida (no forman
  parte del recorrido medido).

### 1.2 Herramienta, infraestructura y configuracion efectivas (condiciones fijas)

| Elemento | Valor |
|---|---|
| Herramienta de generacion de carga y version | k6 v1.6.1 |
| Ubicacion y recursos del generador (fuera de las 2 VMs) | maquina externa a las VMs (equipo del estudiante) |
| Proveedor / region | GCP us-central1 |
| Web Server (tipo, vCPU/RAM/disco) | 2 vCPU / 2 GiB / 30 GiB (tipo: __) |
| Worker Server (tipo, vCPU/RAM/disco) | 2 vCPU / 2 GiB / 30 GiB (tipo: __) |
| PostgreSQL administrado (clase, vCPU/RAM/almacenamiento, AZ) | PostgreSQL 16.15 - 1 CPU virtuales, 3.75 GB - 10 GB SSD  |
| Almacenamiento de objetos (servicio) | us-central1 (Iowa) - Cloud Storage |
| Concurrencia de workers | 4 (WORKER_CONCURRENCY) |
| Version de la app (tag/commit) | commit 87455468b5c76f40ac81c9759f239971138263bc |
| Condiciones que se mantuvieron fijas | RATE_LIMIT_PER_MIN=1000000 (subido para medir la app, no el limitador); N_STUDENTS=20; niveles y duraciones fijos; observabilidad: exporters postgres/redis + Prometheus en Web Server, CPU/mem via muestreo en cada VM |

### 1.3 Resultados por nivel

Metricas de aplicacion (agregado de la corrida; esta corrida fue un unico
`k6 run` con todos los niveles encadenados, por eso las latencias son del
conjunto. Para latencia por nivel se puede correr un `k6 run` por nivel):

| Metrica | Valor |
|---|---|
| Peticiones totales | 82.125 |
| Throughput medio | 129,7 req/s |
| Latencia p50 | 441 ms |
| Latencia p90 | 2.438 ms |
| Latencia p95 | 2.859 ms |
| Latencia p99 | 3.822 ms |
| Latencia max | 6.784 ms |
| Errores funcionales (5xx/conexion) | 1.086 (~1,3%) |
| Rechazos de negocio (4xx) | 2.485 (~3,0%) |
| Timeouts | 0 |
| Rate-limited (429) | 0% (limite subido para la prueba) |
| Quiz calificado correcto | 100% |
| Integridad: duplicado sin doble calificacion | 100% |

Latencia por endpoint (agregado, de mayor a menor p95):

| Endpoint | avg (ms) | p95 (ms) | p99 (ms) |
|---|---|---|---|
| outline | 1.358 | 3.466 | 4.350 |
| quiz_submit | 868 | 3.223 | 4.163 |
| progress_event | 1.141 | 2.971 | 3.811 |
| quiz_start | 874 | 2.412 | 3.259 |
| catalog_detail | 621 | 1.715 | 2.499 |
| catalog_list | 283 | 732 | 1.425 |

Metricas de infraestructura por nivel (pico observado en el tramo de cada nivel,
del muestreo de CPU/mem en cada VM y de Prometheus para BD/cola):

| Nivel | Carga (usuarios/s) | CPU Web % | Mem Web % | CPU Worker % | Conex. PG | Prof. cola |
|---|---|---|---|---|---|---|
| Base | 5 | ~5 | ~35 | ~10 | 16 | 0 |
| 1 | 20 | ~8 | ~36 | ~20 | ~30 | 0 |
| 2 | 50 | ~11 (pico transitorio 88) | ~37 | ~56 | ~31 | 0 |
| 3 | 90 | **98,7** (satura) | ~46 | ~30 | ~31-34 | 0 |
| 3 (rep) | 90 | **~85-98** (satura) | ~41 | ~28 | ~31 | 0 |



Variacion entre repeticiones cercanas al limite: el nivel alto y su repeticion
mostraron el mismo comportamiento (CPU del Web saturada ~85-98% en ambos), lo
que confirma que la saturacion es estable y no un pico puntual.


### 1.4 Analisis (preguntas del enunciado)

- **¿Que volumen sostiene la plataforma dentro de los umbrales y en que nivel
  comienza la degradacion?** En los niveles bajo (5) y medio (20-50 usuarios/s)
  la plataforma responde con recursos holgados: la CPU del Web se mantuvo por
  debajo del 15%, la del Worker por debajo del 56% y las conexiones a la BD
  estables (~30). La degradacion comienza en el **nivel alto (90 usuarios/s)**,
  donde la CPU del Web Server se satura (~98%), la latencia agregada sube a
  p95 ~2,9 s / p99 ~3,8 s y aparecen errores 5xx (~1,3% del total).

- **¿Que operaciones concentran la latencia o los errores, y como se relacionan
  con la API, Redis / la cola, el pool de conexiones y PostgreSQL?** Las
  operaciones mas lentas fueron `outline` (p95 3,47 s), `quiz_submit`
  (p95 3,22 s) y `progress_event` (p95 2,97 s) -- todas requieren trabajo de la
  API y de PostgreSQL. La causa raiz no es Redis ni la cola (cola en 0 durante
  todo el escenario, es sincrono), sino que la **CPU de la API en el Web Server
  se agota**; ademas el numero de **conexiones a PostgreSQL se estanca en ~31**
  (techo del pool), lo que sugiere que las escrituras esperan por una conexion
  libre. La latencia se dispara de forma pareja en los endpoints que tocan la BD.

- **¿Se conservan la integridad de intentos, la calificacion y el progreso bajo
  concurrencia?** Si. La calificacion fue correcta en el 100% de los envios
  (`quiz_calificado_correcto`=100%) y la **comprobacion de envio duplicado no
  produjo doble calificacion** (misma `Idempotency-Key` -> respuesta identica,
  metrica de idempotencia = 100%). Los 2.485 rechazos de negocio (HTTP 409)
  corresponden a intentos ya enviados/expirados, no a fallos de integridad.

- **¿Que cambio permitiria aumentar la capacidad y que medicion respalda esa
  propuesta?** Como el limite fue la **CPU del Web Server** (98%), el cambio de
  mayor impacto es escalar ese componente: mas vCPU en el Web Server o varias
  replicas de la API detras de un balanceador (la API no guarda estado, escala
  horizontalmente). En paralelo, **ampliar el pool de conexiones a PostgreSQL**
  (hoy techo ~31) para que las escrituras no esperen. Medicion que lo respalda:
  la saturacion de CPU al 98% coincide con el salto de latencia y la meseta de
  conexiones; al agregar capacidad de CPU/replicas, repetir la corrida deberia
  mostrar la CPU por VM por debajo del 100% y p95 bajando.

### 1.5 Punto de degradacion / maximo probado, cuello de botella y limitaciones

- **Punto de degradacion:** nivel alto (90 usuarios/s de llegada), donde la CPU
  del Web Server llega a ~98% de forma sostenida.
- **Cuello de botella (sustentado por evidencia):** **CPU del Web Server (la API
  en Go).** Evidencia: `web_infra.csv` muestra CPU ~85-98% (max 98,7%) en el
  nivel alto, mientras el Worker se queda en ~28-56% y la cola en 0; la latencia
  agregada de k6 sube a p95 ~2,9 s. Factor secundario: meseta de conexiones a
  PostgreSQL en ~31 (`database_connections.png`).
- **Limitaciones del experimento:**
  - Corrida unica con todos los niveles encadenados -> las latencias de k6 son
    agregadas; para latencia por nivel habria que correr un `k6 run` por nivel.
  - Prometheus corrio en el Web Server, lo que suma algo de CPU a esa VM.
  - No se midio CPU/mem del PostgreSQL administrado (solo conexiones); conviene
    complementarlo con la consola del proveedor.
  - Parte de los 5xx bajo saturacion pueden incluir el defecto conocido de
    creacion de intentos bajo concurrencia (ver deuda tecnica).

---

## 2. Escenario 2 - Carga, procesamiento y consumo multimedia

### 2.1 Definicion del escenario y niveles de carga
__

### 2.2 Herramienta, infraestructura y configuracion efectivas
__

### 2.3 Resultados por nivel

Latencias separadas por etapa (segun el enunciado):

- Autorizacion de carga / emision de URLs firmadas: __
- Transferencia directa al almacenamiento de objetos: __
- Confirmacion de la carga completa: __
- Espera en cola, duracion de procesamiento y tiempo hasta `available`: __
- Trabajos completados por unidad de tiempo, reintentos, fallos: __
- Evolucion de profundidad y antiguedad de la cola: __
- Operaciones sobre el almacenamiento de objetos (latencia, transferencia, errores): __
- Manifiestos y segmentos HLS (latencia, errores): __

### 2.4 Analisis y drenaje de la cola
Verificacion de que los trabajos aceptados terminan o quedan en fallo
diagnosticable; componente que limita el flujo: __

### 2.5 Punto de degradacion / maximo probado, cuello de botella y limitaciones
__

---

## 3. Propuesta de evolucion

Con base en el Escenario 1, el limite es la **CPU del Web Server**. Propuestas,
ordenadas por impacto esperado y con la medicion que las respalda:

1. **Escalar el Web Server** (mas vCPU) o desplegar **varias replicas de la API
   detras de un balanceador**. La API no guarda estado, asi que escala
   horizontalmente. Respaldo: la CPU llego al 98% justo cuando la latencia se
   disparo; repartir la carga entre mas CPU deberia bajar la CPU por VM y el p95.
2. **Ampliar el pool de conexiones a PostgreSQL** (hoy meseta ~31). Respaldo: el
   estancamiento de conexiones coincide con la latencia alta en los endpoints que
   escriben (progress, submit). Verificar tambien el limite de conexiones del
   servicio administrado.
3. **Corregir la creacion de intentos bajo concurrencia** (defecto conocido: el
   `attempt_number` no es seguro ante carrera). Reduce los 5xx bajo carga.

Medicion de control: repetir esta misma corrida tras cada cambio y comparar CPU
por VM, p95/p99 y errores 5xx; una mejora real debe mostrar CPU por debajo del
100% y p95 en cientos de ms en el nivel alto.
