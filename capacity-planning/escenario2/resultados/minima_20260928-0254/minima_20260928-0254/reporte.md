
## Escenario 2 - Carga, procesamiento y consumo multimedia (prueba minima)

### Condiciones

| Parametro | Valor |
|---|---|
| Fecha | 2026-09-28 03:27 |
| API | `https://136-114-53-21.sslip.io` |
| Generador de carga | vm-pruebas, 8 vCPU, 31.3 GiB (fuera de las VMs de la aplicacion) |
| Herramienta | k6 v1.3.0 |
| Concurrencia de workers | **4 (fija en todos los niveles)** |
| Duracion por nivel | 30s de calentamiento + 3m de medicion; se espera hasta 15m a que cada carga iniciada llegue a `ready` |
| Commit | 70bc9f9 |

### Recorridos

- **Profesores** (llegadas a tasa constante): `POST /uploads` (la API autoriza y firma) -> `PUT` de cada parte de 8 MiB **directo al almacenamiento de objetos** con URL firmada -> `POST /uploads/{id}/complete` -> consulta de `GET /assets/{id}` cada 3 s hasta `ready` -> validacion funcional (duracion +-2 s y numero de rendiciones del master HLS).
- Orden de los archivos subidos (se repite): `P1 P2 A1 P1 P3 P2 P1 A1 P2 P1`.
- **Estudiantes** (usuarios concurrentes): master `.m3u8` -> playlist de una rendicion (rotando por usuario) -> segmentos. Se precargan 3 segmentos y luego **cada segmento se pide al ritmo de reproduccion declarado en `#EXTINF`** (patron de reproduccion, no de descarga masiva). 20 segmentos por sesion y 5 s de pausa entre videos.
- Contenido consumido: curso publicado `escenario-2-contenido-hls-1790555982` con los perfiles ya procesados (P1: 2 rendiciones, P2: 3 rendiciones, P3: 4 rendiciones, A1: 1 rendiciones).

### Perfiles multimedia y rendiciones esperadas

Derivados de un unico video original de 1080p; ningun perfil supera la resolucion del original (sin upscaling).

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
| L1 | 1 | 4 / 4 | 67.3 | 145.2 | 0.1 / 0.2 | 100.6 | 25.6 / 96.9 | 97.3 | 0.00 % | 0.00 % |
| L2 | 2 | 7 / 7 | 60.2 | 153.9 | 0.1 / 1.0 | 121.3 | 132.3 / 488.2 | 489.3 | 0.00 % | 0.00 % |
| L3 | 4 | 13 / 13 | 68.3 | 151.1 | 0.1 / 0.9 | 122.7 | 330.6 / 770.1 | 771.2 | 0.00 % | 0.00 % |

**Consumo HLS (estudiantes, fase de medicion)**

| Nivel | Espectadores | Manifiestos | Manifiesto p95 (ms) | Segmentos | Segmento p50 / p95 (ms) | Mbps segmento p50 | Riesgo estimado de corte | Errores consumo | Rechazos 429 |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| L1 | 10 | 32 | 91.4 | 304 | 45.7 / 82.6 | 302.2 | 0.00 % | 0.00 % | - |
| L2 | 20 | 64 | 87.2 | 614 | 41.0 / 81.6 | 312.5 | 0.00 % | 0.00 % | - |
| L3 | 40 | 130 | 58.8 | 1231 | 41.9 / 93.1 | 273.0 | 0.00 % | 0.00 % | - |

**Por perfil (todas las cargas de cada nivel)**

| Nivel | Perfil | Cargas ok / total | Transferencia prom. | complete -> ready prom. | complete -> ready max | Rendiciones obtenidas |
|---|---|---:|---:|---:|---:|---|
| L1 | A1 | 1 / 1 | 0.2 | 33.1 | 33.1 | 1 |
| L1 | P1 | 2 / 2 | 0.1 | 18 | 18 | 2 |
| L1 | P2 | 1 / 1 | 0.2 | 108.2 | 108.2 | 3 |
| L2 | A1 | 1 / 1 | 0.2 | 30.1 | 30.1 | 1 |
| L2 | P1 | 3 / 3 | 0.1 | 86.2 | 210.4 | 2 |
| L2 | P2 | 2 / 2 | 0.1 | 169.8 | 207.4 | 3 |
| L2 | P3 | 1 / 1 | 1.3 | 607.2 | 607.2 | 4 |
| L3 | A1 | 3 / 3 | 0.2 | 361.7 | 718.4 | 1 |
| L3 | P1 | 5 / 5 | 0.1 | 290.4 | 544.2 | 2 |
| L3 | P2 | 4 / 4 | 0.2 | 417.1 | 706.5 | 3 |
| L3 | P3 | 1 / 1 | 1.8 | 847.6 | 847.6 | 4 |

**Criterios de exito (umbrales de k6)**

| Nivel | Umbrales incumplidos |
|---|---|
| L1 | ninguno |
| L2 | ninguno |
| L3 | ninguno |

Fallos de carga (si los hubo):

- ninguno

### Limitaciones de esta prueba minima

- Mide desde el cliente (k6): tiempos de la API, de la transferencia directa al almacenamiento, espera hasta `ready` (cola + procesamiento juntos) y latencia de manifiestos y segmentos.
- No separa espera en cola de tiempo de transcodificacion ni registra CPU/memoria de las VMs de la aplicacion (eso lo hace la prueba completa con `correr_nivel.sh` y los monitores).
- "Riesgo estimado de corte" = segmento que llego despues del momento en que el reproductor lo habria necesitado; es una estimacion, no una medicion de un reproductor real (no se mide tiempo hasta el primer cuadro).

Datos crudos: `capacity-planning/escenario2/resultados/minima_20260928-0254` (resumen JSON, consola y reporte HTML de k6 por nivel).
