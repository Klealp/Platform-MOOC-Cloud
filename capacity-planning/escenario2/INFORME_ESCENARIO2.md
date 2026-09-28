# Escenario 2 - Carga, procesamiento y consumo multimedia

> Plantilla de la seccion del Escenario 2 para
> `capacity-planning/pruebas_de_carga_entrega2.md`. Se copia alli cuando
> existan los resultados de la nube. Lo marcado con **[COMPLETAR]** sale de
> `resultados/comparativa-nube/comparativa.md` y de cada `analisis.md`.

## 2.1 Definicion

**Objetivo.** Determinar cuantas cargas multimedia por minuto y cuantos
espectadores HLS concurrentes sostiene la configuracion basica (Web Server +
Worker Server de 2 vCPU / 2 GiB, `WORKER_CONCURRENCY` fija) y que componente
limita primero el flujo.

**Herramienta.** k6  v1.3.0, ejecutado desde una VM **[COMPLETAR
tipo]** en **[COMPLETAR region]**, fuera de las dos VMs de la aplicacion.
Se eligio porque permite modelar los dos recorridos en paralelo (llegadas de
profesores a tasa constante y espectadores concurrentes), cortar el archivo en
partes para la carga multipart con URLs firmadas, respetar la cadencia de
reproduccion HLS, validar el resultado funcional con `check` y exportar datos
crudos reproducibles (JSON), un resumen y un reporte HTML.

**Recorridos, endpoints, datos, patron, pausas y criterios.** 

| Aspecto | Definicion |
|---|---|
| Herramienta | k6 (version en `metadata.json`). Motivos: modela los dos recorridos en paralelo, corta archivos en partes para la carga multipart, respeta la cadencia HLS, valida resultados con `check`, exporta crudo (JSON), resumen y reporte HTML. |
| Actores | Profesores: tasa constante de llegada (`constant-arrival-rate`). Espectadores: usuarios concurrentes (`ramping-vus`: rampa en el calentamiento y luego constantes). |
| Recorrido profesor | `POST /uploads` -> `PUT` de cada parte a la URL firmada -> `POST /uploads/{id}/complete` -> `POST /units/{id}/resources` -> `GET /assets/{id}` cada 3 s hasta `ready`/`failed`/`infected` -> validacion (duracion +-2 s, `hls_available`, numero de rendiciones del master). |
| Recorrido espectador | `GET /assets/{id}/playlist` (master) -> `GET` playlist de una rendicion (rota por usuario) -> pre-carga 3 segmentos -> un segmento cada vez que la reproduccion avanza un segmento (cadencia = `EXTINF`, ~8.3 s), manteniendo 3 de ventaja -> 20 segmentos por sesion -> pausa 5 s -> siguiente contenido. |
| Mezcla de archivos | Patron fijo de 10 cargas: 4xP1, 3xP2, 2xA1, 1xP3 (se repite en todos los niveles). |
| Perfiles | Ver `media/perfiles.json`: P1 30 s 480p, P2 90 s 720p, P3 178 s 1080p (original), A1 178 s audio. Rendiciones esperadas: P1 {480,360}, P2 {720,480,360}, P3 {1080,720,480,360}, A1 {a128}. Sin upscaling. |
| Datos sinteticos | 20 profesores de carga (cada uno con su curso y unidad), 40 estudiantes inscritos, 1 curso publicado con 4 assets HLS. Cuentas creadas por el admin (ya verificadas). |
| Autenticacion | NO forma parte del recorrido medido: las sesiones se preparan antes (`preparar_datos.sh`). |
| Niveles | `niveles.json`: base (0.5 cargas/min, 5 espectadores), n1 (1/25), n2 (2/50), n3 (3/100), estres (5/150). Calentamiento 1 min, medicion 10 min. |
| Fijo entre corridas | Tipo de VMs, `WORKER_CONCURRENCY` (queda en `metadata.json`), patron de perfiles, cadencia, intervalos de consulta, version del codigo (commit). |
| Exito del nivel | Todos los umbrales de k6 cumplen (p95 firmar/confirmar < 1 s, p95 consultar estado < 500 ms, p95 manifiestos < 1 s, p95 segmentos < 2 s, errores < 1 %, riesgo de corte < 1 %, >= 99 % de cargas en `ready` y validadas) **y** la cola es estable: trabajos completados/min >= cargas/min y la cola drena. |
| Degradacion | p95 carga->ready mayor al doble de la linea base, o antiguedad de la cola `low` creciendo durante toda la ventana. |
| Saturacion | trabajos completados/min < cargas/min (la cola crece sin limite), o errores > 1 %, o riesgo de corte > 1 %, o p95 segmentos > 2 s. |
| Parada anticipada | Ctrl+C si hay errores > 5 % sostenidos, la cola `low` supera 60 trabajos, el disco de la Worker Server pasa de 90 %, o el presupuesto lo exige. k6 guarda igual el resumen. |

**Perfiles multimedia** (de `escenario2/media/perfiles.json`):

| Perfil | Archivo | Duracion | Resolucion | Tamano | Partes (8 MiB) | Rendiciones esperadas |
|---|---|---:|---:|---:|---:|---|
| P1 | p1_corto_480p.mp4 | 30 s | 480p (854x480) | 0,65 MiB | 1 | v480, v360 |
| P2 | p2_medio_720p.mp4 | 90 s | 720p (1280x720) | 2,43 MiB | 1 | v720, v480, v360 |
| P3 | p3_largo_1080p.mp4 | 178 s | 1080p (1920x1080) | 49,91 MiB | 7 | v1080, v720, v480, v360 |
| A1 | a1_audio.m4a | 178 s | audio | 6,80 MiB | 1 | a128 |

**Infraestructura efectiva y condiciones fijas.** **[COMPLETAR: tipo de VM,
disco, instancia de Cloud SQL, bucket, WORKER_CONCURRENCY, commit/tag]**.

## 2.2 Resultados por nivel

**[COMPLETAR: pegar las dos tablas de `comparativa.md`]**

Graficas: `comp_ready.svg`, `comp_trabajos.svg`, `comp_cola.svg`,
`comp_segmentos.svg`, `comp_api.svg` y, por corrida, `graficas/*.svg`.

Enlaces a cada corrida (scripts, datos crudos y analisis):

| Nivel | Carpeta | Repeticion |
|---|---|---|
| base | `escenario2/resultados/...` | |
| n1 | | |
| n2 | | |
| n3 | | |
| estres | | |

## 2.3 Analisis

Responder, con cifras de las tablas:

1. **Tiempos de la API vs. transferencia.** La API solo firma, confirma y
   consulta estados (p95 **[X] ms**); la transferencia va directa al
   almacenamiento (**[Y] Mbps** p50). ¿Cambio la latencia de la API al subir la
   tasa de cargas? (`comp_api.svg`)
2. **Cola y procesamiento.** ¿A partir de que nivel los trabajos completados
   por minuto dejaron de igualar a las llegadas (`comp_trabajos.svg`)? ¿Como
   evolucionaron la profundidad y la antiguedad de la cola `low`? ¿Cuanto tardo
   el drenaje? ¿Hubo reintentos o trabajos en la DLQ?
3. **Consumo.** Latencia y errores de manifiestos y segmentos por nivel;
   riesgo estimado de corte. Aclarar que no se midio tiempo hasta el primer
   cuadro (no se uso reproductor).
4. **Recursos.** CPU/memoria del contenedor worker vs. la API (`cpu_contenedores.svg`)
   y de la VM generadora (demostrar que no fue el limite).
5. **Punto de degradacion / maximo probado.** Nivel sostenible, nivel donde
   empieza la degradacion y evidencia de estabilidad (repeticion).
6. **Cuello de botella.** Componente que limita primero y la medicion que lo
   demuestra (tipicamente: CPU del Worker Server al 100 % de 2 vCPU mientras la
   API esta holgada y la cola `low` crece).
7. **Evolucion propuesta** y la medicion que la respalda:
   - mas capacidad de procesamiento (mas vCPU o mas Worker Servers): con los
     trabajos/min medidos por vCPU se estima la capacidad nueva;
   - ajustar `WORKER_CONCURRENCY` (4 trabajos ffmpeg compitiendo por 2 vCPU);
   - CDN para los segmentos: hoy cada segmento sale de Cloud Storage; con CDN
     la latencia y el costo por GB bajan y la API no cambia (sigue firmando);
   - concurrencia de transferencias: hoy las partes se suben en serie; subir
     varias en paralelo reduciria el tiempo de transferencia de P3.
