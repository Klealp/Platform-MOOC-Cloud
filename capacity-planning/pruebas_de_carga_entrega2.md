# Informe de capacidad - Entrega 2

> Plantilla con el formato exigido por el enunciado. Rellenar con las corridas
> reales sobre el entorno **desplegado en la nube**. Las corridas locales solo
> sirvieron para depurar los scripts y no se reportan como capacidad.

## 0. Resumen ejecutivo

- Capacidad sostenible (Escenario 1): **__ req/s** dentro de los umbrales.
- Inicio de degradacion: **nivel __** (**__ req/s**).
- Cuello de botella identificado: **__** (evidencia en §Escenario 1).
- Cambio propuesto de mayor impacto: **__**.

## 1. Entorno y configuracion (fija durante las corridas)

| Elemento | Valor |
|---|---|
| Proveedor / region | |
| Web Server (tipo, vCPU/RAM/disco) | |
| Worker Server (tipo, vCPU/RAM/disco) | |
| PostgreSQL administrado (clase, vCPU/RAM/almacenamiento, AZ) | |
| Almacenamiento de objetos (servicio) | |
| Concurrencia de workers | |
| `RATE_LIMIT_PER_MIN` durante la prueba | |
| Version de la app (tag/commit) | |
| Herramienta de carga (k6 version) | |
| Ubicacion y recursos del generador de carga | fuera de las 2 VMs |

Datos sinteticos: `N_STUDENTS=__`, 1 curso publicado (1 modulo, 3 lecturas
obligatorias + 1 quiz de 5 preguntas), inscripciones = N. Scripts:
`capacity-planning/k6/escenario1_actividad_academica.js`.

## 2. Escenario 1 - Actividad academica concurrente

### 2.1 Definicion

- **Recorrido por usuario virtual**: catalogo (list + detail) -> outline de la
  inscripcion -> progreso valido sobre un recurso no-quiz (open/heartbeat/complete)
  -> intento de quiz + submit (validando la nota). Cuentas e intentos distintos.
- **Mezcla lectura/escritura**: lecturas (catalogo, outline) + escrituras
  (eventos de progreso, submit del quiz).
- **Patron de inyeccion**: tasa de llegada creciente (open model).
- **Autenticacion**: preparada antes de la corrida (no medida). Rafaga de login:
  variante separada (§2.5).
- **Niveles** (RATE_SCALE=1): 5 -> 20 -> 50 -> 90 -> 90(rep) -> 5 req/s.
- **Calentamiento / duracion / pausas**: linea base 1 min; 2 min por nivel;
  pausas de 1-3 s entre acciones.
- **Criterios**: exito = p95 < __ ms, errores_funcionales = 0, checks > 98%.
  Saturacion = p95/p99 se disparan, o aparecen 5xx/timeouts, o la cola crece
  sin drenar. Parada = alcanzada la saturacion o el maximo presupuestado.

### 2.2 Resultados por nivel

| Nivel | Tasa objetivo (req/s) | Throughput real | p50 | p95 | p99 | Err func. | 429 | Rechazos negocio |
|---|---|---|---|---|---|---|---|---|
| Base |  |  |  |  |  |  |  |  |
| 1 |  |  |  |  |  |  |  |  |
| 2 |  |  |  |  |  |  |  |  |
| 3 |  |  |  |  |  |  |  |  |
| 3 (rep) |  |  |  |  |  |  |  |  |

Metricas de infraestructura por nivel (de Prometheus/Grafana):

| Nivel | CPU Web | Mem Web | CPU Worker | Conex. PG | CPU PG | Prof. cola |
|---|---|---|---|---|---|---|
|  |  |  |  |  |  |  |  |

Adjuntar graficas en `resultados/` y enlazarlas aqui.

### 2.3 Respuestas exigidas por el enunciado

- **Volumen sostenido y punto de degradacion**: __
- **Operaciones que concentran latencia/errores** y su relacion con API /
  Redis / cola / pool de conexiones / PostgreSQL: __
- **Integridad bajo concurrencia** (intentos, calificacion, progreso), incluida
  la **comprobacion de envio duplicado sin doble calificacion**: __
  (metrica `idempotencia_sin_doble_calificacion` = __; verificacion en BD:
  `SELECT count(*) FROM quiz_attempts WHERE ... status='submitted'` antes/despues.)
- **Cambio que aumentaria la capacidad y medicion que lo respalda**: __

### 2.4 Cuello de botella (sustentado)

Componente: __. Evidencia: __ (correlacionar metrica del generador con
Web/Worker/PostgreSQL/Redis).

### 2.5 Variante: rafaga de inicios de sesion

Login topado a 30/min por IP (scope publico). Resultado de la rafaga y su
efecto (no atribuible a la actividad academica): __

## 3. Escenario 2 - Carga, procesamiento y consumo multimedia

> Pendiente. (Cargas directas al almacenamiento + consumo HLS; separar latencia
> de control de la transferencia de archivos; medir cola y estados hasta
> `available`.)

## 4. Limitaciones del experimento

- Corridas locales excluidas del analisis de capacidad.
- __ (presupuesto, maximo probado si no se alcanzo la saturacion, etc.)

## 5. Propuesta de evolucion

Cambio propuesto -> medicion que respalda la mejora esperada. __
