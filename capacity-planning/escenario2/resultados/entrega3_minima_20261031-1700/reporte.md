## Escenario 2 - Carga, procesamiento y consumo multimedia (prueba minima)

### Condiciones

| Parametro | Valor |
|---|---|
| Fecha (UTC) | 2026-10-31T17:00Z |
| API (balanceador) | `https://34-149-212-102.sslip.io` |
| Generador de carga | vm-pruebas, 8 vCPU, 31.3 GiB (fuera de los grupos de la aplicacion) |
| Herramienta | k6 v1.3.0 (commit/5870e99ae8, go1.25.1, linux/amd64) |
| Concurrencia por instancia de worker | **2** |
| Plantillas (version desplegada) | API `?`, worker `?` |
| Commit del generador | 19d7f8e |
| Archivos iguales a la Entrega 2 (SHA-256) | true |
| Duracion por nivel | 30s de calentamiento + 3m de medicion; se espera hasta 15m a que cada carga iniciada llegue a `ready` |

### Recorridos

- **Profesores** (llegadas a tasa constante): `POST /uploads` (la API autoriza y firma) -> `PUT` de cada parte de 8 MiB **directo al almacenamiento de objetos** con URL firmada -> `POST /uploads/{id}/complete` -> consulta de `GET /assets/{id}` cada 3 s hasta `ready` -> validacion funcional (duracion +-2 s y numero de rendiciones del master HLS).
- Orden de los archivos subidos (se repite): `P1 P2 A1 P1 P3 P2 P1 A1 P2 P1`.
- **Estudiantes** (usuarios concurrentes): master `.m3u8` -> playlist de una rendicion (rotando por usuario) -> segmentos. Se precargan 3 segmentos y luego **cada segmento se pide al ritmo de reproduccion declarado en `#EXTINF`**. 20 segmentos por sesion y 5 s de pausa entre videos.
- Contenido consumido: curso publicado `escenario-2-contenido-hls-1793465320` (P1: 2 rendiciones, P2: 3 rendiciones, P3: 4 rendiciones, A1: 1 rendiciones).

### Perfiles multimedia y rendiciones esperadas

| Perfil | Archivo | Tipo | Duracion (s) | Resolucion | Tamano (MB) | Partes de 8 MiB | Rendiciones esperadas |
|---|---|---|---:|---:|---:|---:|---|
| P1 | p1_corto_480p.mp4 | video | 30 | 854x480 | 0.6 | 1 | v480, v360 |
| P2 | p2_medio_720p.mp4 | video | 90 | 1280x720 | 2.4 | 1 | v720, v480, v360 |
| P3 | p3_largo_1080p.mp4 | video | 178 | 1920x1080 | 49.9 | 7 | v1080, v720, v480, v360 |
| A1 | a1_audio.m4a | audio | 178 | audio | 6.8 | 1 | a128 |

### Resultados por nivel

Tiempos en segundos salvo que se indique. p50/p95 sobre las cargas del nivel.

**Carga y procesamiento (profesores)**

| Nivel | Cargas/min | Cargas ok / iniciadas | API firma p95 (ms) | Confirmar p95 (ms) | Transferencia p50 / p95 | Mbps p50 | complete -> ready p50 / p95 | Extremo a extremo p95 | Errores control | Errores transferencia |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| L1 | 1 | 4 / 4 | 85.5 | 2171.3 | 0.1 / 0.2 | 87.8 | 20.3 / 78.1 | 80.5 | 0.00 % | 0.00 % |
| L2 | 2 | 7 / 7 | 140.8 | 175.4 | 0.1 / 1.1 | 115.1 | 96.8 / 385.9 | 387.2 | 0.00 % | 0.00 % |
| L3 | 4 | 12 / 12 | 83.9 | 177.8 | 0.1 / 0.8 | 150.6 | 37.7 / 329.1 | 330.1 | 0.00 % | 0.00 % |

**Consumo HLS (estudiantes, fase de medicion)**

| Nivel | Espectadores | Manifiestos | Manifiesto p95 (ms) | Segmentos | Segmento p50 / p95 (ms) | Mbps segmento p50 | Riesgo estimado de corte | Errores consumo | Rechazos 429 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| L1 | 10 | 32 | 476.4 | 303 | 64.2 / 159.2 | 207.0 | 0.00 % | 0.00 % | - |
| L2 | 20 | 68 | 85.4 | 627 | 49.7 / 96.0 | 228.8 | 0.00 % | 0.00 % | - |
| L3 | 40 | 128 | 89.3 | 1240 | 41.2 / 75.1 | 278.2 | 0.00 % | 0.00 % | - |

**Por perfil (todas las cargas de cada nivel)**

| Nivel | Perfil | Cargas ok / total | Transferencia prom. | complete -> ready prom. | complete -> ready max | Rendiciones obtenidas |
|---|---|---:|---:|---:|---:|---|
| L1 | A1 | 1 / 1 | 0.2 | 25.2 | 25.2 | 1 |
| L1 | P1 | 2 / 2 | 0.1 | 15.2 | 15.4 | 2 |
| L1 | P2 | 1 / 1 | 0.2 | 87.4 | 87.4 | 3 |
| L2 | A1 | 1 / 1 | 0.3 | 27.2 | 27.2 | 1 |
| L2 | P1 | 3 / 3 | 0.1 | 61 | 143.9 | 2 |
| L2 | P2 | 2 / 2 | 0.1 | 132.4 | 167.9 | 3 |
| L2 | P3 | 1 / 1 | 1.4 | 479.3 | 479.3 | 4 |
| L3 | A1 | 2 / 2 | 0.3 | 36.2 | 39.2 | 1 |
| L3 | P1 | 5 / 5 | 0.1 | 25.4 | 36.2 | 2 |
| L3 | P2 | 4 / 4 | 0.1 | 133.4 | 187.9 | 3 |
| L3 | P3 | 1 / 1 | 1.4 | 501.7 | 501.7 | 4 |

**Criterios de exito (umbrales de k6)**

| Nivel | Umbrales incumplidos |
|---|---|
| L1 | `http_req_duration{name:POST /uploads/:id/complete (confirmar)}`: p(95)<1000 |
| L2 | ninguno |
| L3 | ninguno |

Fallos de carga (si los hubo):

- ninguno

### Instancias y ventanas de tiempo

Instancias operativas (RUNNING y sin accion en curso) muestreadas cada 15 s con gcloud. La concurrencia efectiva es workers operativos x concurrencia por instancia. Las ventanas sirven para tomar de Cloud Monitoring la CPU de los grupos, la metrica de la cola y las conexiones de Cloud SQL.

| Nivel | Inicio (UTC) | Fin (UTC) | API operativas min / max | Workers operativos min / max | Concurrencia efectiva max | Muestras |
|---|---|---|---:|---:|---:|---:|
| L1 | 2026-10-31T17:00:05Z | 2026-10-31T17:03:55Z | - | - | - | 0 |
| L2 | 2026-10-31T17:04:55Z | 2026-10-31T17:15:26Z | - | - | - | 0 |
| L3 | 2026-10-31T17:16:27Z | 2026-10-31T17:26:20Z | - | - | - | 0 |

### Limitaciones de esta prueba minima

- Mide desde el cliente (k6): tiempos de la API, de la transferencia directa al almacenamiento, espera hasta `ready` (cola + procesamiento juntos) y latencia de manifiestos y segmentos.
- No separa espera en cola de tiempo de transcodificacion. La CPU de los grupos, la profundidad de la cola y las conexiones de PostgreSQL se toman de Cloud Monitoring con las ventanas de arriba.
- "Riesgo estimado de corte" = segmento que llego despues del momento en que el reproductor lo habria necesitado; es una estimacion, no una medicion de un reproductor real (no se mide tiempo hasta el primer cuadro).

Datos crudos: `capacity-planning/escenario2/resultados/entrega3_minima_20261031-1700` (resumen JSON, consola y reporte HTML de k6 por nivel).
