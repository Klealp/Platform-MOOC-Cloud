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

**Recorridos, endpoints, datos, patron, pausas y criterios.** Ver la tabla de
la seccion 5 de [`escenario2/README.md`](./escenario2/README.md) (se copia
aqui tal cual) y los niveles en [`escenario2/niveles.json`](./escenario2/niveles.json).

**Perfiles multimedia** (de `escenario2/media/perfiles.json`):

| Perfil | Archivo | Duracion | Resolucion | Tamano | Partes (8 MiB) | Rendiciones esperadas |
|---|---|---:|---:|---:|---:|---|
| P1 | p1_corto_480p.mp4 | **[COMPLETAR]** | 480p | | | v480, v360 |
| P2 | p2_medio_720p.mp4 | | 720p | | | v720, v480, v360 |
| P3 | p3_largo_1080p.mp4 | | 1080p | | | v1080, v720, v480, v360 |
| A1 | a1_audio.m4a | | audio | | | a128 |

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
