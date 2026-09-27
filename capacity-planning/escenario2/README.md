# Escenario 2 - Carga, procesamiento y consumo multimedia

Pruebas de carga del **Escenario 2** de la Entrega 2 (ISIS4426): profesores que
suben video y audio **directamente** al almacenamiento de objetos mientras
estudiantes consumen contenido HLS ya disponible. Todo lo de esta carpeta es
reproducible: los mismos scripts sirven en local (ensayo) y en GCP (medicion).

> Resultado final: `capacity-planning/pruebas_de_carga_entrega2.md` (seccion
> Escenario 2). La plantilla de esa seccion esta en
> [`INFORME_ESCENARIO2.md`](./INFORME_ESCENARIO2.md).

---

## 1. Que hay en esta carpeta

```
capacity-planning/escenario2/
├── README.md                    <- esta guia
├── INFORME_ESCENARIO2.md        <- plantilla de la seccion del informe
├── niveles.json                 <- definicion de los niveles de carga (base, n1, n2, n3, estres)
├── generar_perfiles.sh          <- paso 1: crea P1, P2, P3 y A1 a partir de tu video
├── preparar_datos.sh            <- paso 2: profesores, estudiantes y contenido HLS listo
├── construir_herramientas.sh    <- compila las dos herramientas en Go
├── correr_nivel.sh              <- paso 3: corre UN nivel completo y lo analiza
├── monitor_recursos.sh          <- CPU / memoria / red / disco (docker stats + /proc)
├── k6/escenario2.js             <- el guion de k6 (profesores + espectadores)
├── herramientas/
│   ├── monitorcola/main.go      <- Go: observa la cola asynq en Redis
│   └── analizador/*.go          <- Go: tablas (Markdown) y graficas (SVG)
├── media/                       <- (generado, NO se versiona salvo perfiles.json)
├── datos/                       <- (generado, NO se versiona: tiene tokens)
├── bin/                         <- (generado, NO se versiona: binarios)
└── resultados/<fecha>_<nivel>_<modo>/   <- una carpeta por corrida (SI se versiona)
```

No se modifico ningun archivo existente del repositorio: todo es nuevo y vive
en esta carpeta (mas `postman/Platform-MOOC-Nube.postman_environment.json`).

---

## 2. Como funciona (en una pantalla)

```
                     generador de carga (k6)  -- fuera de las 2 VMs de la aplicacion --
          ┌───────────────────────┴──────────────────────────┐
   PROFESORES (N cargas/min)                         ESPECTADORES (M usuarios)
   1 POST /uploads ─────────────┐                    1 GET master .m3u8 ─────┐
   3 POST /uploads/{id}/complete├──► API (Web Server) 2 GET variante .m3u8 ──┤
   5 GET /assets/{id} cada 3 s ─┘        │ encola                            │
                                         ▼                                   │
   2 PUT partes de 8 MiB ──────────┐   Redis/asynq ─► Worker (escaneo + ffmpeg HLS)
                                   ▼                     │                   │
                        Almacenamiento de objetos ◄──────┘  3 GET segmentos .ts (cada ~8 s)
                        (MinIO local / Cloud Storage)  ◄─────────────────────┘

   monitorcola      -> cola.csv, tareas_*.csv   (profundidad, antiguedad, tiempos por trabajo)
   monitor_recursos -> recursos_*.csv           (CPU, memoria, red, disco de cada maquina)
   analizador (Go)  -> analisis.md + graficas/  (tablas y graficas para el informe)
```

- El **trafico de control** (autorizar, confirmar, consultar estado, pedir
  listas de reproduccion) pasa por la API. La **transferencia de archivos**
  (partes y segmentos) va directo al almacenamiento. El informe los separa.
- Un HTTP 2xx no basta: cada carga solo cuenta como exitosa si llega a
  `ready`, la duracion detectada coincide (+-2 s) y el master HLS tiene
  exactamente las rendiciones esperadas.

---

## 3. Instalacion (una sola vez, en la terminal de Ubuntu/WSL)

### 3.1 k6

Opcion A (repositorio oficial de Grafana):

```bash
sudo gpg -k
sudo gpg --no-default-keyring --keyring /usr/share/keyrings/k6-archive-keyring.gpg \
  --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys C5AD17C747E3415A3642D57D77C6C491D6AC1D69
echo "deb [signed-by=/usr/share/keyrings/k6-archive-keyring.gpg] https://dl.k6.io/deb stable main" \
  | sudo tee /etc/apt/sources.list.d/k6.list
sudo apt-get update && sudo apt-get install -y k6
k6 version
```

Opcion B (binario de GitHub, si la A falla):

```bash
cd /tmp && curl -LO https://github.com/grafana/k6/releases/download/v1.3.0/k6-v1.3.0-linux-amd64.tar.gz
tar xzf k6-v1.3.0-linux-amd64.tar.gz && sudo mv k6-v1.3.0-linux-amd64/k6 /usr/local/bin/ && k6 version
```

Anota la version que salga: va en el informe (queda tambien en `metadata.json`).

### 3.2 Otras utilidades

```bash
sudo apt-get install -y jq curl ffmpeg   # ffmpeg es opcional: si no esta, se usa el del contenedor worker
```

### 3.3 Herramientas en Go

```bash
./capacity-planning/escenario2/construir_herramientas.sh
```

Usa Go si lo tienes en WSL; si no, compila dentro de un contenedor `golang`
(no necesitas instalar nada). Deja `bin/monitorcola` y `bin/analizador`.

### 3.4 Postman

Ya tienes la coleccion. En Postman: **Import** -> selecciona
`postman/Platform-MOOC.postman_collection.json` y los dos entornos
(`...-Local...` y `...-Nube...`). Arriba a la derecha eliges el entorno.

---

## 4. Ensayo en LOCAL (paso a paso)

Todos los comandos se corren **desde la raiz del repositorio**, en WSL.

**Paso 0 - Plataforma arriba**

```bash
docker compose up -d --build
curl -s localhost:8080/readyz     # {"ready":true,...}
```

**Paso 1 - Perfiles multimedia** (una vez; tarda ~1 min)

```bash
./capacity-planning/escenario2/generar_perfiles.sh "C:\Users\kevin\Pictures\fotos bonitas tatacoa\Cantos bajo la Luna.mp4"
```

Crea `media/p1_corto_480p.mp4`, `p2_medio_720p.mp4`, `p3_largo_1080p.mp4`,
`a1_audio.m4a` y `perfiles.json` (la ficha tecnica que va al informe).

**Paso 2 - Datos de prueba** (antes de cada dia de pruebas; ~5 min)

```bash
./capacity-planning/escenario2/preparar_datos.sh
```

Crea 1 profesor de contenido con un curso publicado que tiene los 4 perfiles
ya en `ready`, 20 profesores de carga y 40 estudiantes inscritos. Espera
mientras el worker transcodifica (el P3 de 1080p tarda unos minutos). Los
tokens duran 24 h: si pasa un dia, vuelve a correrlo.

**Paso 3 - Verificacion con Postman** (opcional, 2 min)

Con el entorno "Local", carpeta **5. Media**: en *1) Iniciar carga* pon
`videoSize` = tamano en bytes de `media/p1_corto_480p.mp4` (esta en
`perfiles.json`, campo `tamano_bytes`), en *2) Subir parte* elige ese archivo
en Body -> binary, luego *4) Completar* y repite *6) Estado del asset* hasta
`ready`. Si esto funciona, el sistema esta sano para medir. (P1 cabe en una
sola parte de 8 MiB, por eso sirve para Postman.)

**Paso 4 - Corrida de humo** (~8 min; solo para ver que todo funciona)

```bash
./capacity-planning/escenario2/correr_nivel.sh humo
```

Mientras corre, abre **http://localhost:5665** en el navegador: es el panel en
vivo de k6 (buen material para el video). Opcional: Grafana en
http://localhost:3000, tablero "Plataforma MOOC - Vision general".

Al final veras `Analisis escrito en .../analisis.md`. Abrelo en VS Code
(Ctrl+Shift+V para verlo con formato) o en GitHub.

**Paso 5 - Linea base local** (~15-20 min)

```bash
./capacity-planning/escenario2/correr_nivel.sh base
```

Con eso el ensayo esta completo. Los niveles n1-n3 y estres se corren en la
nube; en local solo si quieres calibrar (tu PC no es la configuracion que se
evalua).

---

## 5. Definicion del escenario (lo que pide "Condiciones comunes")

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

---

## 6. Corridas en la NUBE (GCP)

### 6.1 Que se necesita del despliegue

- URL publica HTTPS de la API (Web Server) y `PUBLIC_BASE_URL` apuntando a ella.
- Acceso SSH a Web Server y Worker Server (`gcloud compute ssh`).
- Una **tercera VM pequena** (p. ej. e2-standard-2) en la **misma region**,
  solo para generar la carga. Motivos: el enunciado pide el generador fuera de
  las dos VMs; la velocidad de subida de una casa limitaria las cargas; y
  bajar segmentos de Cloud Storage dentro de la region no cobra salida a
  Internet. Se puede apagar entre corridas.

### 6.2 Preparar la VM generadora (una vez)

```bash
# en la VM generadora
sudo apt-get update && sudo apt-get install -y git jq curl
# instalar k6 (seccion 3.1)
git clone <repo> && cd <repo>
# copiar desde tu PC: media/ (4 archivos + perfiles.json) y bin/ (monitorcola, analizador)
#   gcloud compute scp --recurse capacity-planning/escenario2/media capacity-planning/escenario2/bin <vm-generadora>:~/<repo>/capacity-planning/escenario2/
API_URL=https://<url-de-la-api> ./capacity-planning/escenario2/preparar_datos.sh
```

### 6.3 Monitores en las VMs de la aplicacion (antes de cada nivel)

Copia `monitor_recursos.sh` a ambas VMs y `bin/monitorcola` a la Worker Server:

```bash
# Web Server
mkdir -p ~/e2 && HOST_ETIQUETA=web ./monitor_recursos.sh ~/e2/recursos_web.csv 5
# Worker Server (dos terminales)
mkdir -p ~/e2 && HOST_ETIQUETA=worker ./monitor_recursos.sh ~/e2/recursos_worker.csv 5
./monitorcola -redis 127.0.0.1:6379 -salida ~/e2     # usar la IP/puerto donde Redis escucha
```

Si Redis no publica su puerto en la VM, corre el monitor dentro de la red de Docker:
`docker run --rm --network <red-de-compose> -v ~/e2:/out -v ~/monitorcola:/monitorcola alpine /monitorcola -redis redis:6379 -salida /out`

Los relojes de las VMs de GCP estan sincronizados por NTP, asi que los tiempos
de k6 y de la cola se pueden unir.

### 6.4 Correr un nivel

```bash
# en la VM generadora
MODO=nube WORKER_CONCURRENCY=4 ./capacity-planning/escenario2/correr_nivel.sh base
```

El script pide confirmar que los monitores estan corriendo, ejecuta k6 y al
final te pide esperar el drenaje de la cola. Luego:

1. Deten los monitores de las VMs (Ctrl+C: monitorcola escribe `tareas_final.csv`).
2. Copia `~/e2/*.csv` de ambas VMs a la carpeta de la corrida.
3. `./capacity-planning/escenario2/bin/analizador -corrida capacity-planning/escenario2/resultados/<carpeta>`
4. Borra `~/e2` en las VMs antes del siguiente nivel.

Orden sugerido: base -> n1 -> n2 -> n3 -> estres -> **repetir** el nivel mas
alto que cumplio (estabilidad). Si el presupuesto no alcanza, reporta el
maximo probado y aclara que no es la capacidad maxima.

### 6.5 Comparar niveles (la tabla principal del informe)

```bash
cd capacity-planning/escenario2
./bin/analizador -comparar resultados/<base>,resultados/<n1>,resultados/<n2>,resultados/<n3>,resultados/<estres> \
  -salida resultados/comparativa-nube
```

---

## 7. De donde sale cada cifra que pide el enunciado

| Lo que pide el enunciado | Fuente |
|---|---|
| Latencia de la API al autorizar y firmar | k6: `POST /uploads (autorizar y firmar)` |
| Tiempo de transferencia directa | k6: `e2_transferencia_ms` y `PUT parte (almacenamiento)`, Mbps por perfil |
| Tiempo de confirmacion de la carga completa | k6: `POST /uploads/:id/complete (confirmar)` |
| Espera en cola, duracion de procesamiento | analizador: une `consola_k6.log` + `tareas_eventos.csv` + `tareas_final.csv` por `scan:<asset>` / `hls:<asset>` |
| Carga completa -> `available` (`ready` en nuestra API) | k6: `e2_carga_a_ready_ms` |
| Trabajos por unidad de tiempo, reintentos, fallos | `tareas_final.csv` (CompletedAt, Retried), `cola.csv` (archivados = DLQ) |
| Profundidad y antiguedad de la cola | `cola.csv` (Inspector de asynq: pendientes/activos/reintentos y `latency`) |
| CPU y memoria de workers, carga de la API | `recursos_*.csv` (docker stats por contenedor + maquina completa) |
| Latencia y errores de manifiestos y segmentos | k6: `GET playlist maestro/variante`, `GET segmento (almacenamiento)` |
| Drenaje de la cola al final | `correr_nivel.sh` espera la cola vacia; el analizador calcula el tiempo |
| Que el generador no fue el limite | `recursos_generador.csv` (CPU/red de la VM de k6) |

Lo que **no** se afirma: tiempo hasta el primer cuadro ni interrupciones reales
(requieren un reproductor). Reportamos un "riesgo estimado de corte": el
porcentaje de segmentos que llegaron despues del momento en que la
reproduccion simulada los necesitaba.

---

## 8. Limitaciones conocidas

- La resolucion de "trabajo activo" es el intervalo de muestreo (2 s) y la de
  `CompletedAt` de asynq es 1 s. Frente a transcodificaciones de 10 s a 4 min
  es suficiente; los trabajos que duran menos de 2 s se marcan como aproximados.
- El estado del asset no distingue "esperando transcodificacion" de
  "transcodificando" (ambos son `processing`); por eso se usa la cola.
- Las cargas de cada nivel se siguen hasta `ready` aunque terminen despues de
  la ventana de medicion (eso ES el drenaje). La ventana de consumo termina a
  su hora; las sesiones interrumpidas al final se reportan como "interrupted"
  en k6 y no son errores.
- En local, k6 y la plataforma comparten la maquina: los numeros locales no se
  reportan como capacidad.

---

## 9. Problemas frecuentes

| Sintoma | Causa / solucion |
|---|---|
| `seed.json tiene mas de 23 h` | Los tokens vencen a las 24 h: vuelve a correr `preparar_datos.sh`. |
| `monitorcola no arranco` | Redis no escucha en `REDIS_ADDR`. En local: `docker compose ps redis`. |
| Muchos `429` | Limite de tasa (120/min por usuario). Sube `PROFESORES`/`ESTUDIANTES` en `preparar_datos.sh`. Los 429 se cuentan aparte. |
| Cargas en `timeout` | El worker no alcanza: es un resultado (saturacion). Revisa `cola.csv`. |
| `k6 termino con codigo 99` | Algun umbral no se cumplio. Es un resultado, no un error del script. |
| El panel de :5665 no abre | Solo existe mientras k6 corre; el reporte final queda en `reporte_k6.html`. |
