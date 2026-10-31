# Análisis de capacidad - Entrega 3 (arquitectura elástica)

Repetición de los escenarios de la Entrega 2 sobre la arquitectura elástica
(balanceador HTTPS, grupo `mooc-api` de 1 a 3 instancias por CPU, grupo
`mooc-worker` de 1 a 3 por cola, Cloud SQL y Cloud Storage) y escenario de
autoescalado. Línea base: [`pruebas_de_carga_entrega2.md`](pruebas_de_carga_entrega2.md).

Contenido:

1. [Escenario 1 - Actividad académica concurrente](#1-escenario-1---actividad-académica-concurrente)
2. [Escenario 2 - Carga, procesamiento y consumo multimedia](#2-escenario-2---carga-procesamiento-y-consumo-multimedia)
3. [Escenario 3 - Ciclo de escalamiento automático (3A API, 3B workers)](#3-escenario-3---ciclo-de-escalamiento-automático)
4. [Conclusiones: qué cambió, qué limita y cuánto cuesta](#4-conclusiones-qué-cambió-qué-limita-y-cuánto-cuesta)

Todas las horas están en UTC (Colombia es UTC-5). Las series de Cloud
Monitoring están alineadas a 1 min y cada valor se marca al **final** de su
intervalo.

---

## 1. Escenario 1 - Actividad académica concurrente

### 1.1 Resumen

- **El límite se desplazó de la API a PostgreSQL.** En la Entrega 2 la CPU
  del Web Server llegó al 98 %. En la Entrega 3 la CPU de las instancias de
  API no pasa del 52 %, mientras Cloud SQL (1 vCPU) se mantiene al **100 %
  durante los 7 minutos de carga alta** de cada corrida.
- **No hubo mejora de capacidad.** El throughput se estanca en unos 112 a
  125 req/s, y la latencia p95 es igual o algo peor que en la Entrega 2:
  3,1 s frente a 2,9 s en las corridas comparables.
- **El autoescalado de la API no se activó por esta carga.** La CPU de la API
  nunca alcanza el objetivo del 60 %, porque la base de datos limita el
  throughput antes. Agregar réplicas no aumentaría la capacidad.
- **La integridad se conservó:** calificación del quiz correcta e idempotencia
  del envío duplicado al 100 % en todas las corridas. Los únicos errores 5xx
  son el defecto conocido de la Entrega 2 al crear intentos en paralelo. Ese
  defecto devuelve 500, pero la restricción `UNIQUE` impide intentos
  duplicados.

### 1.2 Diseño del experimento (igual a la Entrega 2)

- **Recorrido:** el mismo de la Entrega 2:
  - catálogo (listado y detalle);
  - outline de la inscripción;
  - progreso válido (open, heartbeat, complete);
  - quiz (crear intento y enviar, validando la nota).

  Script sin cambios:
  [`k6/escenario1_actividad_academica.js`](k6/escenario1_actividad_academica.js).
- **Datos sintéticos:** creados en el `setup` de cada corrida: 1 curso
  publicado (1 módulo, 3 lecturas y 1 quiz de 5 preguntas), 20 estudiantes
  inscritos (`N_STUDENTS=20`). Las sesiones se preparan antes de medir.
- **Patrón:** tasa de llegada creciente (open model). 5 → 20 → 50 → 90 → 90 →
  5 usuarios/s. Base y enfriamiento de 1 min; niveles de 2 min; pausas de 1 a
  3 s dentro del recorrido. Unos 10 min y 40 s por corrida.
- **Criterios (los de la Entrega 2):**
  - **Éxito:** latencia acotada y sin 5xx.
  - **Degradación:** p95 en segundos o aparición de 5xx.
  - **Saturación:** un recurso cerca del 100 %.
  - **Parada:** al terminar el perfil. No se fuerza más carga.
- **Clasificación de resultados:**
  - errores funcionales: 5xx y fallos de conexión;
  - rechazos de negocio esperados: 4xx, por ejemplo 409 al reenviar un
    intento ya enviado;
  - limitación de tasa: 429.
- **Repeticiones:** tres corridas completas (`r1`, `r2`, `r3`). Antes de `r2` y
  de `r3` se esperó a que `mooc-api` volviera a 1 instancia.
- **Agregación:**
  - las cifras de k6 son del conjunto de cada corrida, como en la Entrega 2;
  - las series de Cloud Monitoring están alineadas a 1 min;
  - los percentiles no se promedian entre corridas.

### 1.3 Condiciones y diferencias declaradas

| Elemento | Entrega 2 | Entrega 3 |
|---|---|---|
| Herramienta | k6 v1.6.1 | k6 v1.6.1 (`grafana/k6:1.6.1` en Docker) |
| Generador | Equipo de un estudiante, fuera de las VMs | PC de un integrante fuera de GCP: AMD Ryzen 5 3400G, 8 hilos, 15,5 GiB, Linux |
| CPU del generador | No registrada | Promedio 14 a 17 %, máximo 44 % (`r1`, `r2`) y un pico aislado de 82 % en `r3`. No limita la medición |
| Entrada | `https://136-114-53-21.sslip.io` (Caddy en el Web Server) | `https://34-149-212-102.sslip.io` (balanceador HTTPS externo de Google) |
| API | 1 VM `e2-highcpu-2` (API, frontend y monitoreo juntos) | Grupo `mooc-api` de VMs `e2-highcpu-2`, 1 a 3 por CPU (objetivo 60 %), solo la API |
| PostgreSQL | Cloud SQL 16, 1 vCPU, 3,75 GB | **Igual**: Cloud SQL 16, `db-custom-1-3840` |
| Pool de conexiones de la API | 25 (por defecto) en una sola instancia | 20 por instancia (`DB_MAX_OPEN_CONNS`) |
| `RATE_LIMIT_PER_MIN` | 1 000 000 | 10 000 000 000. Ambos desactivan el limitador para medir la plataforma; el login sigue en 30/min por IP |
| Versión de la app | commit `8745546` | plantillas `*-tpl-61f22e4` (incluye los ajustes para réplicas, `cd9a98d`) |
| Usuarios virtuales de k6 | Máximo 300 | Máximo 300 |

En ambas entregas k6 llegó a su tope de 300 usuarios virtuales en los
niveles altos. Por eso descartó unas 18 000 iteraciones, cantidad similar en
las dos entregas: la demanda efectiva fue comparable.

### 1.4 Resultados por corrida (k6, lo que ve el usuario)

| Métrica | Entrega 2 | E3 `r1` | E3 `r2` | E3 `r3` |
|---|---|---|---|---|
| Peticiones | 82 125 | 72 259 | 77 709 | 78 137 |
| Throughput medio (req/s) | 129,7 | 113,7 | 122,8 | 123,3 |
| Latencia p50 (ms) | 441 | 506 | 481 | 463 |
| Latencia p95 (ms) | 2 859 | 4 692 | 3 126 | 3 073 |
| Latencia p99 (ms) | 3 822 | 6 699 | 4 099 | 4 095 |
| Latencia máxima (ms) | 6 784 | 12 559 | 7 714 | 7 713 |
| Errores funcionales (5xx) | 1 086 (1,3 %) | 1 016 (1,4 %) | 1 054 (1,4 %) | 1 066 (1,4 %) |
| Rechazos de negocio (409) | 2 485 | 2 179 | 2 224 | 2 216 |
| Timeouts / 429 | 0 / 0 % | 0 / 0 % | 0 / 0 % | 0 / 0 % |
| Quiz calificado correcto | 100 % | 100 % | 100 % | 100 % |
| Idempotencia del envío duplicado | 100 % | 100 % | 100 % | 100 % |
| Iteraciones descartadas por k6 | 17 658 | 18 900 | 18 214 | 18 159 |

**Variación:** `r2` y `r3` difieren menos del 2 % en throughput y
percentiles. `r1` se aparta y no se usa para la comparación principal: empezó
12 s después de una corrida de humo, sin esperar a que el grupo volviera a 1
instancia, y el grupo pasó a 2 instancias en su primer minuto (23:37). Se
conserva como evidencia del comportamiento con 2 réplicas.

### 1.5 Comportamiento en el tiempo y recursos

Por minuto, `r3`; `r2` es equivalente.

- Las columnas de k6 se agrupan por el minuto en que se envió la petición.
- Cloud Monitoring marca cada valor al final de su intervalo de 1 min, por
  eso su serie aparece desplazada un minuto. Por ejemplo, el pico de k6 de
  las 05:13 aparece en el balanceador a las 05:14.

| Hora UTC | Throughput k6 (req/s) | p95 k6 (ms) | Peticiones/s en el balanceador | CPU Cloud SQL | CPU API (máx.) | Instancias API | Conexiones BD |
|---|---|---|---|---|---|---|---|
| 05:10 | 49 | 137 | 24 | 15 % | 13 % | 1 | 14 |
| 05:11 | 98 | 144 | 56 | 35 % | 22 % | 1 | 14 |
| 05:12 | 162 | 255 | 93 | 62 % | 33 % | 1 | 14 |
| 05:13 | **248 (pico)** | 737 | 157 | **100 %** | 46 % | 1 | 21 |
| 05:14 | 166 | 2 991 | **212** | **100 %** | **52 %** | 1 | 24 |
| 05:15 | 126 | 3 466 | 184 | **100 %** | 43 % | 1 | 24 |
| 05:16 | 125 | 3 467 | 128 | **100 %** | 33 % | 1 | 24 |
| 05:17 | 124 | 3 587 | 119 | **100 %** | 32 % | 1 | 24 |
| 05:18 | 125 | 3 508 | 117 | **100 %** | 32 % | 1 | 25 |
| 05:19 | 56 | 4 293 | 119 | **100 %** | 31 % | 1 | 24 |
| 05:20 | — | — | 67 | 78 % | 17 % | 1 | 14 |

Demanda de k6. La carga empezó a las 05:09:08 y cada tramo sube en forma
lineal hasta su objetivo:

| Desde | Hasta | Usuarios/s |
|---|---|---|
| 05:09 | 05:10 | 5 |
| 05:10 | 05:12 | de 5 a 20 |
| 05:12 | 05:14 | de 20 a 50 |
| 05:14 | 05:16 | de 50 a 90 |
| 05:16 | 05:18 | 90 |
| 05:18 | 05:19 | de 90 a 5 |

**Cloud SQL llega al 100 % a las 05:13, con unos 40 a 50 usuarios/s.**
Desde ese momento, subir la demanda hasta 90 ya no aumenta el throughput:
solo aumenta la latencia y las iteraciones que k6 descarta.

Resumen por corrida (Cloud Monitoring, ventana de cada corrida):

| Corrida | CPU máx. Cloud SQL | Minutos con Cloud SQL ≥ 99 % | CPU máx. API | Instancias API | Conexiones máx. | p95 máx. del balanceador |
|---|---|---|---|---|---|---|
| `r1` | 100 % | 7 | 46 % | 1 → 2 | 45 | 6 094 ms |
| `r2` | 100 % | 7 | 50 % | 1 | 25 | 4 014 ms |
| `r3` | 100 % | 7 | 52 % | 1 | 25 | 3 878 ms |

Las conexiones son la suma de las bases de la instancia: unas 20 del pool de
cada réplica de API más las internas de Cloud SQL.

La cola (`queue_backlog`) no se ejercita en este escenario: es síncrono, igual
que en la Entrega 2.

### 1.6 Tabla comparativa

Se comparan las corridas con la misma condición inicial (`r2`, `r3`) frente a
la corrida de la Entrega 2.

| Escenario y operación | Nivel de carga | Métrica y unidad | Entrega 2 | Entrega 3 (`r2` / `r3`) | Cambio observado y explicación |
|---|---|---|---|---|---|
| E1, recorrido completo | Misma demanda: 5→90 usuarios/s, 300 VUs máx. | Throughput (req/s) | 129,7 | 122,8 / 123,3 | **-5 %.** Cloud SQL al 100 % fija un techo de ~125 req/s en la meseta; las réplicas de API no pueden superarlo |
| E1, recorrido completo | Misma | Latencia p95 (ms) | 2 859 | 3 126 / 3 073 | **+8 a 9 %.** Las peticiones esperan en la base saturada. El salto extra del balanceador es menor y no explica la diferencia: con carga baja el p95 es ~140 ms |
| E1, recorrido completo | Misma | Latencia p99 (ms) | 3 822 | 4 099 / 4 095 | +7 %, misma causa |
| E1, recorrido completo | Misma | Errores 5xx (%) | 1,3 % | 1,4 % / 1,4 % | Igual. Mismo defecto de concurrencia al crear intentos (`quiz_start`) |
| E1, pico antes de saturar | Rampa de 20 a 50 usuarios/s (~40-50) | Throughput pico (req/s, 1 min) | No medido por minuto | 256 / 248 | Antes de saturar la base el sistema atiende ~250 req/s; el techo aparece cuando Cloud SQL llega al 100 % |
| E1, recurso limitante | Desde ~40-50 usuarios/s | CPU (%) | Web Server 98 % | API ≤ 52 %, **Cloud SQL 100 %** | **El límite pasó de la API a PostgreSQL** |

### 1.7 Análisis

**Qué mejoró.** La API dejó de ser el recurso limitante. Al separarla del
frontend y del monitoreo, que en la Entrega 2 compartían su VM, y al poder
tener réplicas detrás del balanceador, su CPU queda por debajo del 52 % con
la carga máxima. La integridad, el reparto por el balanceador y la
idempotencia funcionan entre réplicas.

**Qué quedó igual o empeoró.** El throughput y la latencia no mejoraron. La
carga del Escenario 1 es intensiva en escrituras pequeñas a PostgreSQL:
eventos de progreso, intentos y envíos de quiz. La única instancia de Cloud
SQL, con 1 vCPU, se satura desde unos 40 a 50 usuarios/s y fija un techo de
~125 req/s.

**Por qué el autoescalado no se activó.** El grupo `mooc-api` escala cuando su
CPU supera el 60 %. Con la base saturada, las peticiones esperan en la base
de datos y no consumen CPU de la API, que se queda entre 30 y 52 %. El
autoescalado no reacciona porque agregar réplicas no sería útil: solo
agregaría conexiones a una base ya saturada. En `r1`, con 2 réplicas, el
resultado fue peor (p95 de 4,7 s): más conexiones compitiendo por la misma
vCPU de Cloud SQL.

**Qué dependencia limita ahora.** PostgreSQL administrado. Las acciones con
mayor impacto esperado son:

1. Subir la clase de Cloud SQL, por ejemplo a 2 vCPU.
2. Revisar las consultas de escritura más frecuentes (`progress_event`,
   `quiz_submit`) con `pg_stat_statements`.

Ambas cambian el costo y quedan como propuesta. No se probaron en esta
entrega.

**Integridad.** Las verificaciones de negocio del script pasaron en todas las
corridas:

- **Nota del quiz:** se valida cada envío respondido con 200.
- **Envío duplicado:** un segundo envío no recalifica (409 esperado).

Los 5xx son siempre `POST /quizzes/{id}/attempts`. La API cuenta los intentos
y luego inserta `attempt_number = usados + 1`. Dos peticiones simultáneas del
mismo estudiante calculan el mismo número y la segunda viola
`UNIQUE (quiz_id, user_id, attempt_number)`: no se crean intentos duplicados,
pero se responde 500 en lugar de 409. Ocurre por igual en la Entrega 2. No se
corrigió para no cambiar la aplicación entre las dos mediciones.

### 1.8 Limitaciones

- Una sola corrida de la Entrega 2. La variación se informa solo para la
  Entrega 3.
- El generador cambió de máquina respecto a la Entrega 2. Ambos están fuera de
  la nube y su CPU no limitó, pero la latencia de red hasta GCP puede
  diferir.
- Con 20 cuentas y hasta 90 llegadas por segundo, varias iteraciones
  comparten estudiante. Eso provoca la carrera de `quiz_start`, igual en las
  dos entregas.
- No se hizo una verificación SQL posterior de intentos y progreso. La
  integridad se apoya en las validaciones del script y en las restricciones
  de la base.
- La memoria de las VMs se exportó sumada entre instancias, por lo que no se
  analiza por instancia.

### 1.9 Evidencias

| Archivo | Contenido |
|---|---|
| [`escenario1/correr_entrega3.sh`](escenario1/correr_entrega3.sh) | Ejecuta una corrida y deja condiciones, ventana UTC, salidas de k6 y CPU del generador |
| `escenario1/resultados/entrega3_r{1,2,3}_<fecha>/` | Por corrida: `condiciones.txt`, `ventana.txt`, `reporte.txt`, `summary.json`, `raw.json.gz` (todas las muestras), `salida.txt` y `generador.csv` |
| [`escenario1/resultados/cloud_monitoring_entrega3/`](escenario1/resultados/cloud_monitoring_entrega3/) | Exportación de Cloud Monitoring del 30-10-2026 23:30 al 31-10-2026 00:21 (hora de Colombia, UTC-5), alineada a 1 min: CPU y conexiones de Cloud SQL, tamaño de cada grupo, CPU y memoria de las VMs, peticiones y latencia (p50/p95/p99) del backend de la API en el balanceador |
| `escenario1/resultados/entrega3_humo*_<fecha>/` | Corridas de humo: validación del recorrido, no se usan en el análisis. La primera muestra el 48 % de 429 con el limitador en su valor normal |
| [`escenario1/escenario1_reporte.txt`](escenario1/escenario1_reporte.txt) | Corrida de la Entrega 2 (línea base) |

Ventanas UTC: `r1` 04:36:23 a 04:47:05; `r2` 04:54:50 a 05:05:30; `r3`
05:08:58 a 05:20:00 (31-10-2026).

---

## 2. Escenario 2 - Carga, procesamiento y consumo multimedia

### 2.1 Resumen

- **El tiempo hasta que el contenido está disponible bajó cuando sube la
  carga.** En L3 (4 cargas/min), el tiempo de `complete` a `ready` pasó de
  330,6 s a **37,7 s** en p50 (-89 %) y de 770,1 s a **329,1 s** en p95
  (-57 %). En L2, de 132,3 s a 96,8 s en p50 (-27 %) y de 488,2 s a 385,9 s
  en p95 (-21 %). En L1 la diferencia es pequeña.
- **La mejora la produjo el escalamiento de workers, no el de la API.** El
  grupo `mooc-worker` pasó de 1 a 2 instancias durante L2 y de 2 a 3 durante
  L3. La API se quedó en 1 instancia, con CPU ≤ 9 %, y la admisión de cargas
  no cambió de forma relevante.
- **El techo ahora es el trabajo más largo, no la cola.** Un video P3 (178 s,
  1080p, 4 rendiciones) tarda entre 479 y 502 s en procesarse en un solo
  worker. Ese tiempo ya no incluye espera en cola y no se reduce agregando
  instancias, porque un trabajo no se reparte entre máquinas. Fija el p95 de
  L2 y L3.
- **El consumo HLS no cambió de forma atribuible.** Los segmentos van directo
  de Cloud Storage al cliente y no pasan por la API ni por los workers.
- **Integridad:** 23 de 23 cargas llegaron a `ready` con la duración (±2 s) y
  el número de rendiciones esperados. No hubo errores de control ni de
  transferencia, y la cola registró **0 reintentos**.

### 2.2 Diseño y condiciones

El recorrido, los perfiles, el patrón de llegadas y los niveles son los de la
Entrega 2 (prueba mínima, [`escenario2/README.md`](escenario2/README.md)). El
generador es el mismo script
[`escenario2/correr_minima.sh`](escenario2/correr_minima.sh) con
[`escenario2/k6/escenario2.js`](escenario2/k6/escenario2.js).

| Elemento | Entrega 2 | Entrega 3 |
|---|---|---|
| Fecha | 28-09-2026 | 31-10-2026, 17:00:05 a 17:26:20 UTC |
| Generador | `vm-pruebas` (e2-standard-8, 8 vCPU, 31,3 GiB), us-central1-a | **Igual** |
| Herramienta | k6 v1.3.0 | **Igual** |
| Entrada de la API | `https://136-114-53-21.sslip.io` (Caddy) | `https://34-149-212-102.sslip.io` (balanceador) |
| Workers | 1 VM `e2-highcpu-2`, `WORKER_CONCURRENCY=4` | Grupo `mooc-worker` de 1 a 3 VMs `e2-highcpu-2`, **`WORKER_CONCURRENCY=2` por instancia** |
| Versión de la app | commit `70bc9f9` | plantillas `*-tpl-61f22e4`; generador en el commit `19d7f8e` |
| Niveles | L1 1 carga/min y 10 espectadores; L2 2/min y 20; L3 4/min y 40. 30 s de calentamiento y 3 min de medición | **Igual** |
| Perfiles multimedia | P1 30 s 480p, P2 90 s 720p, P3 178 s 1080p, A1 178 s audio | **Mismos parámetros, archivos regenerados** (ver abajo) |
| Repeticiones | 1 | 1 |

**Diferencias declaradas:**

1. **Concurrencia por instancia.** En la Entrega 2 había 4 trabajos
   simultáneos en una VM de 2 vCPU. En la Entrega 3 hay 2 por VM, de modo que
   cada ffmpeg tiene en promedio 1 vCPU. La concurrencia total depende del
   autoescalado: es de 2, 4 o 6 según las instancias activas (tabla 2.4). Esto
   explica parte de la mejora: con 4 trabajos en 2 vCPU, cada uno avanzaba a
   la mitad de velocidad. Por eso P3 tardaba 607 s en L2 de la Entrega 2 y 479
   s en la Entrega 3.
2. **Archivos multimedia.** Se regeneraron con
   [`generar_perfiles.sh`](escenario2/generar_perfiles.sh) a partir de otro
   original. La duración, la resolución y las rendiciones son iguales, y el
   tamaño difiere en menos de 0,01 %. Sin embargo, los SHA-256 **no**
   coinciden con los de la Entrega 2. El campo `media_igual_a_entrega2: true`
   de `condiciones.json` es incorrecto: compara contra el `perfiles.json`
   regenerado. Los perfiles usados quedan en
   [`perfiles_usados.json`](escenario2/resultados/entrega3_minima_20261031-1700/perfiles_usados.json).
3. **Cargas en L3:** 12 en la Entrega 3 y 13 en la Entrega 2. Es el redondeo
   de la tasa constante de llegada de k6.
4. **Muestreo de instancias.** El script intentó muestrear las instancias con
   `gcloud` cada 15 s y no obtuvo muestras (columna en `-` del `reporte.md`).
   Las instancias por nivel salen de Cloud Monitoring y del registro de
   auditoría del autoescalador (sección 3.2).

### 2.3 Resultados (k6)

**Carga y procesamiento (profesores).** Tiempos en segundos, salvo los
indicados en ms.

| Nivel | Cargas ok / iniciadas (E2 → E3) | Firma p95 ms (E2 → E3) | Confirmar p95 ms (E2 → E3) | `complete`→`ready` p50 (E2 → E3) | `complete`→`ready` p95 (E2 → E3) | Extremo a extremo p95 (E2 → E3) |
|---|---|---|---|---|---|---|
| L1 | 4/4 → 4/4 | 67,3 → 85,5 | 145,2 → **2 171,3** | 25,6 → 20,3 | 96,9 → 78,1 | 97,3 → 80,5 |
| L2 | 7/7 → 7/7 | 60,2 → 140,8 | 153,9 → 175,4 | 132,3 → 96,8 | 488,2 → 385,9 | 489,3 → 387,2 |
| L3 | 13/13 → 12/12 | 68,3 → 83,9 | 151,1 → 177,8 | **330,6 → 37,7** | **770,1 → 329,1** | 771,2 → 330,1 |

**Tiempo hasta `ready` por perfil**, promedio y máximo del nivel:

| Nivel | Perfil | Entrega 2 prom. / máx. (s) | Entrega 3 prom. / máx. (s) |
|---|---|---|---|
| L2 | P3 | 607,2 / 607,2 | 479,3 / 479,3 |
| L3 | P1 | 290,4 / 544,2 | 25,4 / 36,2 |
| L3 | P2 | 417,1 / 706,5 | 133,4 / 187,9 |
| L3 | A1 | 361,7 / 718,4 | 36,2 / 39,2 |
| L3 | P3 | 847,6 / 847,6 | 501,7 / 501,7 |

**Consumo HLS (estudiantes).** El manifiesto pasa por la API
(`GET /assets/{id}/playlist`); los segmentos van directo a Cloud Storage con
URL firmada.

| Nivel | Espectadores | Manifiesto p95 ms (E2 → E3) | Segmento p50 / p95 ms (E2 → E3) | Errores | Riesgo estimado de corte |
|---|---|---|---|---|---|
| L1 | 10 | 91,4 → 476,4 | 45,7 / 82,6 → 64,2 / 159,2 | 0 % → 0 % | 0 % → 0 % |
| L2 | 20 | 87,2 → 85,4 | 41,0 / 81,6 → 49,7 / 96,0 | 0 % → 0 % | 0 % → 0 % |
| L3 | 40 | 58,8 → 89,3 | 41,9 / 93,1 → 41,2 / 75,1 | 0 % → 0 % | 0 % → 0 % |

**Umbrales de k6.** En L1 se incumplió `confirmar p95 < 1000 ms`. Con 4
muestras, el p95 equivale al máximo. Dos confirmaciones fueron lentas: P2 a
las 17:01:36 (2 434 ms) y A1 a las 17:02:36 (685 ms); las otras dos tardaron
139 ms. L2 y L3 cumplen todos los umbrales.

### 2.4 Instancias, cola y recursos por nivel

Fuente: [`escenario3/cloud_monitoring_3b/linea_de_tiempo.txt`](escenario3/cloud_monitoring_3b/linea_de_tiempo.txt)
y el registro de auditoría del autoescalador. La concurrencia efectiva es
igual a los workers que consumen la cola multiplicados por 2.

| Nivel | Ventana UTC | Workers que consumen | Concurrencia efectiva | Backlog máx. (pendientes + activos) | Activos máx. | Antigüedad máx. del pendiente más viejo | Reintentos | CPU workers máx. | CPU API máx. | CPU Cloud SQL | Conexiones BD máx. |
|---|---|---|---|---|---|---|---|---|---|---|---|
| L1 | 17:00:05-17:03:55 | 2 → 1 (17:01:00, retiro de la instancia sobrante de una corrida previa) | 4 → 2 | 2 | 2 | 0 s | 0 | 79 % | 9 % | 6-8 % | 6 |
| L2 | 17:04:55-17:15:26 | 1 → 2 (desde 17:10:34) | 2 → 4 | 3 | 2 | **121 s** (17:11) | 0 | 100 % | 9 % | 6-8 % | 6 |
| L3 | 17:16:27-17:26:20 | 2 → 3 (desde 17:22:32) | 4 → 6 | **5** | **4** | 15 s | 0 | 100 % | 9 % | 7-8 % | 7 |

- Memoria máxima: workers 49 %, API 33 % e `infra-server` 34 %. La memoria no
  limita.
- La cola se vació a las 17:27, un minuto después de que terminó L3. Desde
  ahí, el backlog se mantuvo en 0.
- Cloud SQL no se acerca a su límite en este escenario (≤ 8 % de CPU). Esto
  contrasta con el Escenario 1.

### 2.5 Tabla comparativa

| Escenario y operación | Nivel de carga | Métrica y unidad | Entrega 2 | Entrega 3 | Cambio observado y explicación |
|---|---|---|---|---|---|
| E2, `complete`→`ready` | L3, misma demanda (4 cargas/min); 12 frente a 13 cargas | p50 (s) | 330,6 | 37,7 | **-89 %.** La cola dejó de acumular espera: el grupo pasó a 2 workers (4 trabajos simultáneos) antes de L3 y a 3 durante L3. La antigüedad máxima del pendiente fue de 15 s |
| E2, `complete`→`ready` | L3 | p95 (s) | 770,1 | 329,1 | **-57 %.** El p95 queda dominado por el tiempo de procesamiento de P2 y P3, no por la espera |
| E2, `complete`→`ready` | L2 (2 cargas/min) | p50 / p95 (s) | 132,3 / 488,2 | 96,8 / 385,9 | -27 % / -21 %. El segundo worker llegó a las 17:10:34, 5,5 min después de iniciar L2: la primera mitad del nivel corrió con 1 worker y concurrencia 2, la mitad que en la Entrega 2 |
| E2, `complete`→`ready` | L1 (1 carga/min) | p50 / p95 (s) | 25,6 / 96,9 | 20,3 / 78,1 | -21 % / -19 %. Sin escalamiento; la diferencia viene de menos contención por VM (2 trabajos en 2 vCPU frente a 4) |
| E2, P3 (178 s, 1080p) | L2 y L3 | `complete`→`ready` (s) | 607 / 848 | 479 / 502 | -21 % / -41 %. Es el piso del escenario: un único trabajo de ffmpeg no se reparte entre instancias |
| E2, admisión (`POST /uploads`) | L1-L3 | Firma p95 (ms) | 60-68 | 84-141 | +20 a +80 ms. Es el salto adicional del balanceador; la API está a ≤ 9 % de CPU. No afecta el tiempo hasta `ready` |
| E2, confirmación | L2 y L3 | p95 (ms) | 151-154 | 175-178 | +15 %, misma causa. En L1, 2 171 ms por dos casos aislados de 4 muestras (sección 2.3) |
| E2, consumo HLS (segmentos) | L1-L3 | p95 (ms) | 82-93 | 75-159 | Sin cambio atribuible: Cloud Storage los sirve directo y no dependen de la API ni de los workers. La variación de L1 coincide con el período de confirmaciones lentas y se basa en pocas muestras |
| E2, consumo HLS (manifiestos) | L2 y L3 | p95 (ms) | 59-87 | 85-89 | Igual; la API (1 instancia) no estaba cargada. L1: 476 ms con 32 muestras |
| E2, errores | L1-L3 | Control, transferencia y consumo (%) | 0 | 0 | Igual |

### 2.6 Análisis

**Efecto del escalamiento de workers.** La admisión es rápida en ambas
entregas. La diferencia está en la espera en cola. En la Entrega 2, con un
solo worker, la cola crecía durante todo L3 y la última carga esperaba más de
10 min. En la Entrega 3 la señal `queue_backlog` superó el objetivo de 2
trabajos por instancia en L2 y en L3, y el grupo agregó capacidad.

**Qué no mejora.** Hay tres aspectos que no cambian:

1. **Duración de un trabajo individual.** P3 tarda unos 8 min con 1 vCPU
   efectiva y ningún autoescalado lo reduce. Para reducirlo hay que acelerar
   ffmpeg: VMs con más CPU por instancia o transcodificar las rendiciones en
   trabajos separados.
2. **Tiempo de reacción.** El grupo tarda entre 1,8 y 2,7 min desde la
   decisión hasta que un worker nuevo consume (sección 3.2). En ráfagas cortas
   como L3, el tercer worker llegó cuando ya no quedaban trabajos pendientes y
   no procesó ninguno.
3. **Admisión y consumo.** Dependen de la API y de Cloud Storage, no de los
   workers. La API no escaló en este escenario y no tenía por qué hacerlo:
   ≤ 9 % de CPU.

**Separación por tramo.**

| Tramo | Medido con | Comentario |
|---|---|---|
| Autorización de la carga (`POST /uploads`) | k6, `API firma` | Tráfico de control en la API |
| Transferencia al almacenamiento | k6, `transferencia` | Directa a Cloud Storage. p50 de 0,1 s y 88 a 151 Mbps (E2: 101 a 123 Mbps) |
| Confirmación (`POST /uploads/{id}/complete`) | k6, `confirmar` | Tráfico de control en la API; encola el escaneo |
| Espera en cola | `queue_oldest_pending_seconds` y `queue_pending` | Máximo de 121 s en L2 y 15 s en L3. k6 no la separa del procesamiento por carga |
| Procesamiento | `queue_active` y CPU de los workers | Hasta 4 trabajos activos (2 workers × 2) |
| Tiempo hasta `available` | k6, `complete`→`ready` | Espera en cola más procesamiento |

### 2.7 Limitaciones

- **Una sola corrida** por entrega. No se informa variación del Escenario 2.
  Es el pendiente más importante de este informe.
- **Archivos regenerados** con distinto SHA-256 (diferencia 2).
- k6 no separa espera y procesamiento por carga individual. Se separan en
  agregado con las métricas de la cola, cada 30 s.
- La duplicación de efectos se verifica de forma indirecta: estado `ready`
  único por carga, 0 reintentos y las tres capas de idempotencia. No se
  ejecutó la consulta de reconciliación sobre `processed_jobs` (sección 3.4).

### 2.8 Evidencias

| Archivo | Contenido |
|---|---|
| [`escenario2/resultados/entrega3_minima_20261031-1700/`](escenario2/resultados/entrega3_minima_20261031-1700/) | `reporte.md`, `condiciones.json`, ventanas, resumen JSON, salida y consola de k6 (una línea `E2_CARGA` por carga), y reporte HTML por nivel |
| [`escenario2/resultados/entrega2_minima_20260928-0254/`](escenario2/resultados/entrega2_minima_20260928-0254/) | Línea base de la Entrega 2 |
| [`escenario3/cloud_monitoring_3b/`](escenario3/cloud_monitoring_3b/) | Series de Cloud Monitoring de la misma ventana (16:50 a 17:50): grupos, cola, CPU y memoria por VM, Cloud SQL y balanceador |

`datos/seed.json` (los tokens de sesión de la corrida) **no se versiona**.

---

## 3. Escenario 3 - Ciclo de escalamiento automático

### 3.1 Políticas evaluadas

Los valores efectivos se leyeron del autoescalador con la API de Compute
Engine el 31-10-2026.

| Parámetro | 3A: grupo `mooc-api` | 3B: grupo `mooc-worker` |
|---|---|---|
| Señal | `CPU_UTILIZATION`: CPU promedio del grupo | `custom.googleapis.com/mooc/queue_backlog` = pendientes + activos, publicada cada 30 s por `queue-metrics` desde `infra-server` |
| Objetivo | 0,60 | 2 trabajos por instancia (`singleInstanceAssignment`) |
| Mín. / máx. | 1 / 3 | 1 / 3 |
| Período de inicialización (`coolDownPeriodSec`) | 180 s | 180 s |
| Control de reducción | Máximo 1 instancia cada 600 s | Máximo 1 instancia cada 600 s |
| Estabilización de reducción (implícita de GCE) | 10 min | 10 min |

### 3.2 Variante 3B: escalamiento de workers

La carga es el Escenario 2 de la sección 2: L1 → L2 → L3 y luego drenaje.

**Línea de tiempo** (31-10-2026, UTC). Fuentes:
[`autoscaler_decisiones.json`](escenario3/autoscaler_decisiones.json),
[`eventos_instancias.csv`](escenario3/eventos_instancias.csv),
[`arranques.txt`](escenario3/arranques.txt),
[`worker_logs_3b.txt`](escenario3/worker_logs_3b.txt) y
[`linea_de_tiempo.txt`](escenario3/cloud_monitoring_3b/linea_de_tiempo.txt).

| Hora | Demanda | Señal | Decisión del servicio | Instancias | Evento |
|---|---|---|---|---|---|
| 17:00:05 | Inicia L1 (1 carga/min) | backlog 0 | — | 2 (`77ct`, `50g5`, este último sobrante de una corrida previa) | — |
| 17:00:59 | L1 | 0 | **Reducir a 1** (`STABILIZATION_PERIOD`) | 2 → 1 | `50g5`: `Waiting for all workers to finish... All workers have finished` en 1 s: no tenía trabajos activos |
| 17:04:55 | Inicia L2 (2 cargas/min) | 0 → 2 | — | 1 | — |
| 17:08:48 | L2 | **2,30** > 2 × 1 | **Crear 1** (`calculatedSize` 2) | 1 → 2 | Inserción de `mooc-worker-grvm` a las 17:08:49 |
| 17:10:34 | L2 | 3 (1 pendiente de 91 s) | — | 2 | `grvm`: `consumiendo la cola (concurrencia=2)`; `LISTA` a las 17:10:37 (t = 85 s desde el arranque del kernel) |
| 17:12 | L2 | 3 → 1 | — | 2 | El pendiente de 121 s se atiende y la cola baja |
| 17:16:27 | Inicia L3 (4 cargas/min) | 0 → 5 | — | 2 | — |
| 17:18-17:21 | L3 | 5, **4 activos** | — | 2 | 4 activos con concurrencia 2 por VM: **`grvm` procesa 2 trabajos** (CPU 82-100 %) |
| 17:19:48 | L3 | **4,07** > 2 × 2 | **Crear 1** (`calculatedSize` 3) | 2 → 3 | Inserción de `mooc-worker-bp1x` a las 17:19:49 |
| 17:22:32 | L3 | 2 | — | 3 | `bp1x` consume; `LISTA` a las 17:22:35 (t = 135 s). No le quedan trabajos pendientes |
| 17:23-17:26 | Fin de llegadas | 1 activo (P3 en `grvm`) | — | 3 | `bp1x` en 3 % de CPU |
| 17:26:20 | Termina L3 (P3 en `ready`) | 1 → 0 | — | 3 | — |
| 17:27 | Sin demanda | 0 | — | 3 | Cola vacía |
| 17:31:09 | — | 0 | **Reducir a 2** (`STABILIZATION_PERIOD`) | 3 → 2 | Retiro de `grvm`: apagado ordenado sin trabajos activos |
| 17:41:19 | — | 0 | **Reducir a 1** (`SCALE_IN_CONTROL`: 1 cada 600 s) | 2 → 1 | Retiro de `77ct`, igual. Queda `bp1x` |

**Tiempos de reacción:**

| Escalamiento | Desde el aumento de demanda hasta la decisión | Desde la decisión hasta el consumo de la cola | Arranque de la VM (`LISTA`, t desde el kernel) | Desde la cola vacía hasta la reducción |
|---|---|---|---|---|
| 1 → 2 | 3 min 53 s (inicio de L2 → 17:08:48) | **1 min 46 s** | 85 s | — |
| 2 → 3 | 3 min 21 s (inicio de L3 → 17:19:48) | **2 min 44 s** | 135 s | — |
| 3 → 2 | — | — | — | 4 min (17:27 → 17:31) |
| 2 → 1 | — | — | — | 14 min (17:27 → 17:41) |

El tiempo de decisión incluye el tiempo que tarda la cola en superar el
objetivo: llegan 2 o 4 cargas por minuto y cada una pasa por escaneo y
transcodificación. El arranque se reparte así, según `arranques.txt`:

- 15 a 34 s hasta que corre `startup.sh`;
- 30 a 77 s instalando Docker;
- unos 25 s instalando el Ops Agent;
- unos 10 s para leer el secreto, descargar la imagen y arrancar.

Una imagen de disco con Docker y el Ops Agent preinstalados recortaría
alrededor de 1 min.

**Otros ciclos automáticos observados el mismo día** (corridas previas, sin
análisis detallado; están en
[`cloud_monitoring_3b/previo_15h40_16h50/`](escenario3/cloud_monitoring_3b/previo_15h40_16h50/)):

- 15:43:15: crear 1 con backlog 2,00; reducción a las 15:55:59.
- 16:41:45: crear 1 con backlog 2,32; reducción a las 17:00:59.

### 3.3 Variante 3A: escalamiento de la API

La carga es la corrida de humo `humo2`, con `RATE_SCALE=0.2` y tramos de 30
s, seguida 13 s después por la corrida `r1` del Escenario 1, con el perfil
completo de 5 a 90 usuarios/s. La sección 1 la excluye de la comparación con
la Entrega 2 justamente porque el grupo escaló durante ella. Aquí es la
evidencia del ciclo.

**Línea de tiempo** (31-10-2026, UTC). Fuentes:
[`linea_de_tiempo.txt`](escenario3/cloud_monitoring_3a/linea_de_tiempo.txt),
[`reparto_api_3a.txt`](escenario3/reparto_api_3a.txt) y las mismas de 3B.

| Hora | Demanda | Señal | Decisión del servicio | Instancias | Evento |
|---|---|---|---|---|---|
| 04:33:04 | Inicia `humo2` (carga baja) | CPU de `wqv3` 9 a 12 % (media por minuto) | — | 1 (`wqv3`) | — |
| 04:35:45 | `humo2` en su tramo alto (~59 req/s en el balanceador) | **CPU 0,645** > 0,60 | **Crear 1** (`calculatedSize` 2) | 1 → 2 | Inserción de `mooc-api-zwx0` a las 04:35:46 |
| 04:36:23 | Inicia `r1` (5 → 90 usuarios/s) | — | — | 2 (1 operativa) | — |
| 04:37:57 | — | — | — | 2 | API de `zwx0` escuchando en :8080; Postgres, Redis y bucket conectados en el primer intento |
| 04:38:00 | — | — | — | 2 | `LISTA` (t = 104 s) |
| 04:38:01 | — | — | — | 2 | Primera verificación `/readyz` del balanceador con 200 |
| 04:38:14 | — | — | — | **2 operativas** | **Primera petición `/api/v1` atendida por `zwx0`** |
| 04:40-04:46 | Meseta de 90 usuarios/s | CPU de cada réplica 20 a 34 % | — | 2 | Cloud SQL al 100 %. Reparto de 04:44:00 a 04:44:20: `wqv3` 1 145 y `zwx0` 1 087 peticiones (51 % / 49 %) |
| 04:47:05 | Termina `r1` | CPU < 10 % | — | 2 | — |
| 04:52:11 | Sin demanda | CPU **0,032** | **Reducir a 1** | 2 → 1 | — |
| 04:53:42 | — | — | — | 1 | Retiro de `wqv3` tras el drenaje del balanceador (30 s) y `shutdown.sh` |
| 04:54:50 | Inicia `r2` | — | — | 1 (`zwx0`) | Las corridas `r2` y `r3` usan 1 instancia (sección 1) |

**Tiempos de reacción:**

- Desde el aumento de carga (inicio de `humo2`) hasta la decisión: **2 min
  41 s**.
- Desde la decisión hasta la primera petición atendida por la réplica nueva:
  **2 min 28 s**. Se reparte en 53 s de creación y arranque del kernel, 104 s
  de `startup.sh` hasta `LISTA` y 14 s hasta que el balanceador la incluye
  (2 verificaciones correctas cada 10 s).
- Desde el fin de la carga hasta la reducción: **5 min 6 s**. La última vez
  que la CPU justificó 2 instancias fue hacia las 04:42, y la estabilización
  de 10 min explica la hora de la decisión.

La media por minuto de Cloud Monitoring (46 % a las 04:36) es menor que el
valor de la señal que registró el autoescalador (64,5 %). El autoescalador
evalúa ventanas más cortas y la media por minuto suaviza los picos. Se
reporta el valor del registro de auditoría porque es el que motivó la
decisión.

**Resultado de la réplica adicional.** Las dos réplicas se repartieron la
carga casi por mitades, pero **el rendimiento no mejoró**. En `r1` el p95 fue
de 4,7 s, frente a 3,1 s de `r2` y `r3` con 1 instancia. Con Cloud SQL al
100 %, la segunda réplica solo agrega conexiones que compiten por la misma
vCPU de la base: 45 conexiones frente a 25 (sección 1.7). El grupo no llegó a
3 instancias porque la CPU de la API nunca volvió a superar el 60 %.

### 3.4 Integridad durante los ciclos

| Verificación | 3A | 3B |
|---|---|---|
| Operaciones de negocio | `r1`: quiz calificado correcto al 100 %, envío duplicado idempotente al 100 %. Solo aparecen los 5xx del defecto conocido de `quiz_start` (1,4 %, igual que con 1 instancia) | 23/23 cargas en `ready` con la duración y las rendiciones esperadas |
| Trabajos aceptados frente a terminados | No aplica (síncrono) | 23 aceptados y 23 en `ready`. 0 fallidos, **0 reintentos** (`queue_retry`) y cola vacía al final |
| Retiro con trabajo en curso | El retiro de `wqv3` ocurrió sin carga: el drenaje de 30 s no tuvo peticiones en curso que proteger | Los 3 retiros automáticos (`50g5`, `grvm`, `77ct`) ocurrieron con la VM inactiva. El apagado ordenado se ve en los logs, pero sin trabajo activo |
| Sesión entre réplicas | Las sesiones de `r1` se crearon en su `setup` (04:36:23), cuando solo `wqv3` atendía, y siguieron válidas en `zwx0` desde las 04:38:14: el token opaco se valida contra el caché de Redis y, si falla, contra Postgres (`internal/auth`), nunca en memoria de la réplica | No aplica |

**Pendiente: retiro controlado de un worker con un trabajo activo.** El
enunciado lo exige y **todavía no se ha ejecutado**. Ninguno de los retiros
automáticos lo cubre, porque todos encontraron la VM inactiva. Procedimiento
propuesto:

```bash
Z=--zone=us-central1-a
# 1. Minimo de 2 workers durante la prueba, para que otro worker retome el
#    trabajo sin esperar a que arranque una VM nueva
gcloud compute instance-groups managed update-autoscaling mooc-worker $Z --min-num-replicas=2
# 2. Subir un P3 (unos 8 min de procesamiento) desde el frontend o con k6
# 3. Identificar el worker que lo tomo: queue_active = 1 y CPU ~100 % en una VM
gcloud compute instance-groups managed list-instances mooc-worker $Z
# 4. Retirarlo a mitad del procesamiento. El grupo ejecuta shutdown.sh en la
#    VM y la reemplaza por otra (no cambia el tamano objetivo).
gcloud compute instance-groups managed recreate-instances mooc-worker $Z --instances=<vm>
# 5. Esperado en los logs (worker_logs) y en las metricas de la cola:
#    "apagando: se esperan los trabajos en curso" -> asynq espera 60 s
#    (WORKER_SHUTDOWN_TIMEOUT_SECONDS) -> el trabajo no alcanza a terminar
#    -> vuelve a la cola sin gastar reintento -> el otro worker lo toma
#    -> el asset llega a ready UNA vez (consulta SQL de abajo)
# 6. Restaurar el minimo
gcloud compute instance-groups managed update-autoscaling mooc-worker $Z --min-num-replicas=1
```

Para la reconciliación, ejecutar con `psql` sobre Cloud SQL, por ejemplo por
IAP desde `infra-server`:

```sql
-- Cada asset de la ventana debe estar en ready y tener exactamente un
-- escaneo y una transcodificacion terminados (job_key es la llave primaria
-- de processed_jobs, asi que un reintento no puede registrar un segundo).
SELECT a.status,
       count(*)                                   AS assets,
       count(s.job_key)                           AS con_escaneo,
       count(h.job_key)                           AS con_hls
  FROM assets a
  LEFT JOIN processed_jobs s ON s.job_key = 'scan:' || a.id
  LEFT JOIN processed_jobs h ON h.job_key = 'hls:'  || a.id
 WHERE a.created_at BETWEEN '2026-10-31 17:00Z' AND '2026-10-31 17:27Z'
 GROUP BY a.status;
-- Esperado: una fila 'ready' con assets = con_escaneo = con_hls = 23.
```

Los derivados HLS no se duplican porque cada reintento escribe en las mismas
llaves (`hls_prefix` del asset). Se puede contrastar el número de objetos bajo
ese prefijo con `gcloud storage ls gs://<bucket>/<hls_prefix>/** | wc -l`.

Hay respaldo automatizado del comportamiento, sin reemplazar la prueba en
GCP:

- `internal/tasks/interrupted_test.go`: un trabajo interrumpido no marca el
  asset como fallido.
- `internal/tasks/errorhandler_test.go`: la auditoría de la DLQ sobrevive al
  contexto cancelado.

### 3.5 Limitaciones del Escenario 3

- **3A no es una corrida dedicada.** Se reutiliza la secuencia `humo2` + `r1`.
  El ciclo está completo: 1 instancia, creación, atención por la réplica
  nueva y reducción. Pero la carga del Escenario 1 satura Cloud SQL antes que
  la CPU de la API, por eso no llegó a 3 instancias. Una corrida dedicada
  debería usar una mezcla con más lecturas, por ejemplo catálogo y outline,
  para cargar la CPU de la API sin saturar la base.
- **Ningún retiro, automático ni controlado, encontró trabajo en curso.**
  Queda pendiente la prueba de la sección 3.4.
- **Una sola corrida de cada variante.**
- El reemplazo de `mooc-api-qwrv` por `wqv3` (31-10, 04:29 a 04:31) fue una
  actualización manual del grupo, no autoescalado. Por eso el tamaño 2 de las
  04:31 no figura como ciclo en 3A.

---

## 4. Conclusiones: qué cambió, qué limita y cuánto cuesta

**Qué mejoró.**

- El procesamiento multimedia bajo carga: L3 pasó de 330 s a 38 s de p50
  hasta `ready`.
- La API dejó de ser el recurso limitante: en el Escenario 1 su CPU no pasa
  del 52 %.
- Las instancias se crean, se configuran solas y se retiran sin
  intervención. El balanceador las incorpora en unos 15 s después de quedar
  listas.

**Qué quedó igual o empeoró.**

- El throughput del Escenario 1: -5 %, con p95 +8 %.
- La latencia de la API añade de 15 a 80 ms por el balanceador.
- El consumo HLS no cambió, porque nunca dependió de las VMs.

**Qué limita ahora.**

- **Escenario 1:** Cloud SQL, con 1 vCPU, al 100 % desde unos 40 a 50
  usuarios/s. Escalar la API empeora la latencia porque agrega conexiones.
- **Escenario 2:** la duración de un trabajo individual (P3, unos 8 min) y el
  tiempo de reacción del grupo (1,8 a 2,7 min), que es comparable a la
  duración de una ráfaga corta.
- **Ambos:** `infra-server` (Redis) es punto único de falla. Su indisponibilidad
  del 30 al 31-10 impidió que la API arrancara y llevó al grupo a recrear la
  VM unas 10 veces (ver [`docs/entrega3/arquitectura.md`](../docs/entrega3/arquitectura.md),
  sección 6.4).

**Cuánto cuesta.** El costo fijo de la arquitectura elástica en su mínimo (1
API y 1 worker) es mayor que el de la Entrega 2: se agregan el balanceador,
`infra-server` y la VM del frontend. La estimación está en la sección 7 de
`arquitectura.md`. El costo **marginal** del autoescalado observado es bajo:

- 3B agregó 44 min de VM `e2-highcpu-2`, unos 0,04 USD;
- 3A agregó 18 min, unos 0,015 USD.

**Qué conviene hacer a continuación**, en orden de impacto esperado:

1. Subir la clase de Cloud SQL y revisar las escrituras frecuentes.
2. Usar una imagen de disco con Docker preinstalado para recortar 1 min de
   arranque.
3. Dividir la transcodificación por rendición.
4. Repetir las corridas para informar variación.

Ninguna de estas acciones se probó en esta entrega.
