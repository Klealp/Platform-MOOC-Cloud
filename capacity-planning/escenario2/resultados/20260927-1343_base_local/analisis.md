# Escenario 2 - Analisis de la corrida `20260927-1343_base_local`

> Generado por `herramientas/analizador` el 2026-09-27 13:59 a partir de los archivos crudos de esta carpeta. No editar a mano: volver a generar.

## Condiciones

| Parametro | Valor |
|---|---|
| Nivel | base |
| Modo (local/nube) | local |
| Inicio | 2026-09-27T13:43:43-05:00 |
| API | http://localhost:8080 |
| Version de k6 | k6 v1.3.0 (commit/5870e99ae8, go1.25.1, linux/amd64) |
| Commit evaluado | c710154 |
| Concurrencia de workers | 4 |
| Generador de carga | KEVINVICTUS, 12 vCPU, 3.6 GiB |
| Cargas por minuto | 0.5 |
| Espectadores concurrentes | 5 |
| Calentamiento / duracion | 1m / 10m |

## Resumen

| Indicador | Valor |
|---|---|
| Cargas iniciadas / llegaron a `ready` y validadas | 6 / 6 |
| API: autorizar y firmar p95 | 1.07 s |
| API: confirmar carga p95 | 202 ms |
| Carga completa -> `ready` p50 / p95 / max | 55.6 s / 4.6 min / 5.3 min |
| Espera en cola de transcodificacion p95 / max | 2.84 s / 3.00 s |
| Trabajos HLS completados por minuto | 0.45 |
| Profundidad maxima de la cola `low` (HLS) / antiguedad maxima | 3 / 13.4 s |
| Drenaje de la cola tras la ultima carga | 3.5 min |
| Segmento HLS p50 / p95 / p99 | 76 ms / 1.72 s / 2.59 s |
| Riesgo estimado de corte de reproduccion | 0.00 % |
| Errores: control / transferencia / segmentos / manifiestos | 0.00 % / 0.00 % / 0.00 % / 0.00 % |
| Rechazos por limite de tasa (429, esperados) | 0 |

### Criterios de exito (umbrales declarados en k6)

| Metrica | Umbral | Resultado |
|---|---|---|
| `e2_api_firma_ms` | `p(95)<1000` | **NO cumple** |
| `e2_carga_exitosa` | `rate>0.99` | cumple |
| `e2_manifiesto_ms{fase:medicion}` | `p(95)<1000` | cumple |
| `e2_riesgo_corte{fase:medicion}` | `rate<0.01` | cumple |
| `e2_segmento_ms{fase:medicion}` | `p(95)<2000` | cumple |
| `http_req_duration{name:GET /assets/:id (consultar estado)}` | `p(95)<500` | **NO cumple** |
| `http_req_duration{name:POST /units/:id/resources}` | `p(95)<1000` | cumple |
| `http_req_duration{name:POST /uploads (autorizar y firmar)}` | `p(95)<1000` | **NO cumple** |
| `http_req_duration{name:POST /uploads/:id/complete (confirmar)}` | `p(95)<1000` | cumple |
| `http_req_duration{name:PUT parte (almacenamiento)}` | `p(95)<30000` | cumple |
| `http_req_failed{tipo:consumo,fase:medicion}` | `rate<0.01` | cumple |
| `http_req_failed{tipo:consumo_api,fase:medicion}` | `rate<0.01` | cumple |
| `http_req_failed{tipo:control,fase:medicion}` | `rate<0.01` | cumple |
| `http_req_failed{tipo:transferencia,fase:medicion}` | `rate<0.01` | cumple |

## 1. Plano de control de la API (trafico que SI pasa por la API)

| Operacion | n | p50 | p95 | p99 | max |
|---|---:|---:|---:|---:|---:|
| POST /uploads (autorizar y firmar) | 6 | 594 ms | 1.07 s | 1.12 s | 1.13 s |
| POST /uploads/:id/complete (confirmar) | 6 | 158 ms | 202 ms | 204 ms | 204 ms |
| POST /units/:id/resources | 6 | 34 ms | 39 ms | 40 ms | 40 ms |
| GET /assets/:id (consultar estado) | 182 | 47 ms | 1.43 s | 2.44 s | 8.84 s |
| GET playlist maestro (estudiantes) | 35 | 130 ms | 1.32 s | 1.98 s | 2.15 s |
| GET playlist variante (estudiantes) | 35 | 38 ms | 616 ms | 1.04 s | 1.21 s |

Peticiones de control por segundo (medicion): 0.25. Tasa de error de control: 0.00 %.

## 2. Transferencia directa al almacenamiento de objetos (NO pasa por la API)

| Perfil | n | transferencia p50 | p95 | max | Mbps p50 | Mbps min |
|---|---:|---:|---:|---:|---:|---:|
| A1 | 1 | 1.42 s | 1.42 s | 1.42 s | 40.1 | 40.1 |
| P1 | 2 | 2.37 s | 4.31 s | 4.53 s | 13.4 | 1.2 |
| P2 | 2 | 2.08 s | 3.35 s | 3.49 s | 17.9 | 5.8 |
| P3 | 1 | 9.96 s | 9.96 s | 9.96 s | 42.0 | 42.0 |

| Operacion | n | p50 | p95 | p99 | max |
|---|---:|---:|---:|---:|---:|
| PUT parte (almacenamiento) (cada parte de 8 MiB) | 12 | 522 ms | 1.68 s | 2.61 s | 2.84 s |

Tasa de error de transferencia: 0.00 %.

## 3. Procesamiento asincrono (worker)

Tiempos por carga obtenidos uniendo el registro de k6 con los trabajos de asynq (`scan:<asset>`, `hls:<asset>`). Resolucion: ~2 s (muestreo) y 1 s (`CompletedAt`).

| Perfil | n ok | cola+escaneo p50 | espera cola HLS p50 | p95 | transcodificacion p50 | p95 | carga -> ready p50 | p95 | max |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| A1 | 1 | 784 ms | 1.87 s | 1.87 s | 7.13 s | 7.13 s | 12.5 s | 12.5 s | 12.5 s |
| P1 | 2 | 1.13 s | 2.39 s | 2.94 s | 17.1 s | 20.6 s | 22.5 s | 26.1 s | 26.5 s |
| P2 | 2 | 372 ms | 2.18 s | 2.36 s | 1.8 min | 2.3 min | 1.9 min | 2.4 min | 2.4 min |
| P3 | 1 | 348 ms | 1.47 s | 1.47 s | 5.3 min | 5.3 min | 5.3 min | 5.3 min | 5.3 min |
| **Todas** | 6 | 511 ms | 1.93 s | 2.84 s | 51.0 s | 4.5 min | 55.6 s | 4.6 min | 5.3 min |

- Reintentos de trabajos de estas cargas: 0. Trabajos en DLQ (archivados) al final: 0.
- Extremo a extremo (inicio de la carga -> `ready`) p50 / p95: 59.4 s / 4.8 min.

## 4. Cola de mensajeria (Redis + asynq)

| Cola | Uso | profundidad max (pend+activos+reint) | antiguedad max del pendiente mas viejo |
|---|---|---:|---:|
| default | escaneo (media:scan) | 1 | 652 ms |
| low | transcodificacion HLS (media:transcode) | 3 | 13.4 s |

Trabajos HLS completados por minuto (desde la primera confirmacion hasta el ultimo `ready`): 0.45.

## 5. Consumo HLS (estudiantes)

| Operacion | n | p50 | p95 | p99 | max |
|---|---:|---:|---:|---:|---:|
| GET playlist maestro | 35 | 130 ms | 1.32 s | 1.98 s | 2.15 s |
| GET playlist variante | 35 | 38 ms | 616 ms | 1.04 s | 1.21 s |
| GET segmento (almacenamiento) (todas) | 527 | 76 ms | 1.72 s | 2.59 s | 3.79 s |
|   segmento a128 | 197 | 37 ms | 1.08 s | 1.47 s | 1.88 s |
|   segmento v1080 | 40 | 149 ms | 2.29 s | 3.17 s | 3.69 s |
|   segmento v360 | 102 | 59 ms | 1.17 s | 2.58 s | 2.60 s |
|   segmento v480 | 24 | 86 ms | 1.51 s | 1.63 s | 1.64 s |
|   segmento v720 | 164 | 95 ms | 1.96 s | 2.95 s | 3.79 s |

- Tasa de transferencia por segmento p50: 56.5 Mbps. Errores en segmentos: 0.00 %; en manifiestos: 0.00 %.
- Riesgo estimado de corte: 0.00 % de los segmentos llegaron despues del momento en que la reproduccion simulada los necesitaba. Es una estimacion a partir de peticiones HTTP; NO es tiempo hasta el primer cuadro ni interrupciones medidas con un reproductor.
- Segmentos por segundo (medicion): 0.85.

## 6. Recursos de las maquinas y contenedores

Sin datos de recursos (faltan `recursos_*.csv`).

## 7. Graficas

![Profundidad de la cola (pendientes + activos + reintentos)](graficas/cola_profundidad.svg)

![Antiguedad del trabajo pendiente mas viejo](graficas/cola_antiguedad.svg)

![Tiempo desde la carga completa hasta ready, por carga](graficas/carga_a_ready.svg)

![Transcodificaciones completadas por minuto](graficas/trabajos_por_minuto.svg)

![API: latencia p95 por ventana de 30 s](graficas/latencia_control_p95.svg)

![Segmentos HLS: latencia p95 por ventana de 30 s](graficas/segmentos_p95.svg)

## Archivos de esta corrida

`metadata.json` (condiciones), `salida_k6.txt` y `resumen_k6.json` (resumen de k6), `reporte_k6.html` (panel de k6), `crudo_k6.json.gz` (todas las muestras), `consola_k6.log` (una linea por carga), `cola.csv`, `tareas_eventos.csv`, `tareas_final.csv` (cola), `recursos_*.csv` (maquinas) y `cargas.csv` (una fila por carga con todos sus tiempos).
