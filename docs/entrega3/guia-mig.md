# Guia: aplicacion elastica con grupos de instancias (MIG)

Documento de trabajo del equipo para la Entrega 3. Explica **como funciona**
la arquitectura elastica, **que esta hecho** en la rama `entrega-3-infra` y
**que falta**. No reemplaza a `arquitectura.md` (entregable del enunciado),
pero es la base para escribirlo.

Contenido:

1. [El problema que resolvemos](#1-el-problema-que-resolvemos)
2. [Las piezas, una por una](#2-las-piezas-una-por-una)
3. [Como llega el programa a una VM nueva](#3-como-llega-el-programa-a-una-vm-nueva-paso-a-paso)
4. [Como se retira una VM sin perder trabajo](#4-como-se-retira-una-vm-sin-perder-trabajo)
5. [Como decide el autoescalador](#5-como-decide-el-autoescalador)
6. [Verificaciones de salud: dos tipos a proposito](#6-verificaciones-de-salud-dos-tipos-a-proposito)
7. [Donde vive cada cosa](#7-donde-vive-cada-cosa)
8. [Como desplegar desde cero](#8-como-desplegar-desde-cero)
9. [Como mirar lo que pasa](#9-como-mirar-lo-que-pasa)
10. [Estado actual de la rama](#10-estado-actual-de-la-rama)
11. [Pendientes](#11-pendientes)
12. [Glosario](#12-glosario)

---

## 1. El problema que resolvemos

En la Entrega 2 habia **dos maquinas fijas**: `web-server` (API + frontend) y
`worker-server` (worker + Redis). Si llegaban muchos usuarios, la API se
saturaba (en el informe de capacidad, la CPU llego al 98 % con 90
usuarios/s) y no habia nada que hacer salvo crear otra maquina a mano.

La Entrega 3 pide que el sistema **crezca y se encoja solo**:

- Entre 1 y 3 maquinas de API, segun la carga.
- Entre 1 y 3 maquinas de workers, segun los trabajos en cola.
- Sin que nadie entre a las maquinas a configurarlas.

Para eso, las maquinas dejan de ser "nuestras maquinas" y pasan a ser
**copias desechables** que Google crea y destruye. Esa es la idea central de
todo este documento: **ninguna VM de los grupos es especial**. Si una muere,
otra igual toma su lugar.

> En AWS esto se llama Auto Scaling Group y las maquinas son EC2. En GCP son
> **instancias de Compute Engine** dentro de un **Managed Instance Group
> (MIG)**. La idea es la misma.

## 2. Las piezas, una por una

### Imagen de disco vs. imagen Docker

Hay dos "imagenes" distintas y es facil confundirlas:

| | Imagen de disco | Imagen Docker |
|---|---|---|
| Que es | El sistema operativo de la VM | Nuestro programa empaquetado |
| Cual usamos | `debian-12` (publica de Google) | `api`, `worker`, `web`, `queue-metrics` |
| Donde vive | Proyecto `debian-cloud` de Google | **Artifact Registry** (`us-central1-docker.pkg.dev/<proyecto>/mooc/...`) |
| Quien la crea | Google | Nosotros, con `deploy/gcp/build.sh` |

La VM arranca con Debian "limpio" y luego descarga nuestro programa como
imagen Docker.

### Plantilla de instancia (instance template)

Es la **receta** de una VM. No es una maquina: es la descripcion de como
debe ser cada maquina del grupo.

```
mooc-api-tpl-cd9a98d   <- el sufijo es el commit
  tipo de maquina: e2-highcpu-2 (2 vCPU, 2 GiB)
  disco:           30 GB, Debian 12
  red:             subred mooc-app, SIN IP publica
  identidad:       cuenta de servicio mooc-api
  etiqueta:        mooc-api (las reglas de firewall la usan)
  metadatos:
    mooc-rol      = api
    mooc-version  = cd9a98d
    mooc-compose  = (contenido de deploy/gcp/mig/api/docker-compose.yml)
    startup-script  = (contenido de deploy/gcp/mig/startup.sh)
    shutdown-script = (contenido de deploy/gcp/mig/shutdown.sh)
```

Una plantilla **no se puede modificar**. Para desplegar codigo nuevo se crea
otra plantilla (`mooc-api-tpl-<nuevo-commit>`) y se le dice al grupo que la
use. Eso hace que siempre se sepa que version corre cada maquina.

### Grupo de instancias administrado (MIG)

Es un **gerente** al que se le dice: "manten N maquinas hechas con esta
plantilla". El MIG:

- **Crea** maquinas hasta tener N (les pone nombre solo: `mooc-api-7xk2`).
- **Borra** maquinas si sobran.
- **Recrea** una maquina si su verificacion de salud falla
  (autorreparacion).
- **Reemplaza gradualmente** las maquinas cuando cambia la plantilla.

Tenemos tres grupos:

| Grupo | Plantilla | Tamano | Quien decide N |
|---|---|---|---|
| `mooc-api` | `mooc-api-tpl-<sha>` | 1 a 3 | Autoescalador, por CPU |
| `mooc-worker` | `mooc-worker-tpl-<sha>` | 1 a 3 | Autoescalador, por trabajos en cola |
| `mooc-web` | `mooc-web-tpl-<sha>` | siempre 1 | Nadie (fijo) |

El grupo `mooc-web` no escala, pero esta en un grupo para tener plantilla y
autorreparacion gratis.

### Autoescalador

Es el que **decide N**. Mira una senal (CPU, trabajos en cola) cada pocos
segundos y le dice al MIG "ahora quiero 2". Ver la [seccion 5](#5-como-decide-el-autoescalador).

### Balanceador de carga

Es la **unica puerta de entrada** desde Internet. Tiene una IP publica fija y
un certificado HTTPS. Reparte las peticiones entre las maquinas sanas del
grupo `mooc-api` (y manda todo lo que no es `/api/v1/*` al frontend).

Lo importante: el balanceador **no conoce maquinas, conoce grupos**. Cuando
el MIG agrega una VM, el balanceador la incluye solo en cuanto pasa la
verificacion de salud.

### infra-server

Una VM **fija**, fuera de los grupos, con Redis (la cola), Mailpit y el
publicador de la metrica de la cola. No puede estar en un grupo: si el
autoescalador la borrara, se perderia la cola de todos.

## 3. Como llega el programa a una VM nueva (paso a paso)

Esta es la pregunta clave: **el MIG no sabe nada de nuestro programa**. Solo
sabe crear VMs con la plantilla. El programa llega asi:

```
 t = 0 s    El autoescalador ve CPU alta y pide 2 maquinas en vez de 1.
            |
 t ~ 1 s    El MIG crea la VM "mooc-api-p9q1" usando la plantilla.
            |   Google enciende Debian 12 (sin nuestro programa).
            v
 t ~ 30 s   Debian termina de arrancar y el agente de Google lee el
            metadato "startup-script" y lo ejecuta como root.
            Ese script es deploy/gcp/mig/startup.sh:
            |
            |  1. Lee los metadatos:  rol=api, version=cd9a98d
            |  2. Instala Docker y el Ops Agent (si no estan)
            |  3. Pide a Secret Manager el secreto "mooc-api-env"
            |       -> /etc/mooc/api.env  (contrasenas, IPs, llaves)
            |  4. Escribe /opt/mooc/docker-compose.yml (del metadato)
            |  5. docker pull .../mooc/api:cd9a98d  (de Artifact Registry)
            |  6. docker compose up -d    -> la API escucha en :8080
            |  7. Espera a que /healthz responda y escribe "LISTA" en el log
            v
 t ~ 2 min  El balanceador consulta /readyz cada 10 s. Tras 2 respuestas
            buenas seguidas, empieza a mandarle peticiones.
            |
            v
            La VM nueva atiende trafico. Nadie entro a configurarla.
```

Por que funciona sin intervencion:

- **La identidad de la VM** (cuenta de servicio `mooc-api`) tiene permiso
  para leer *solo* su secreto y descargar imagenes. No hay contrasenas
  escritas en la plantilla ni en el disco de la imagen.
- **La version viene en la plantilla**, asi que todas las maquinas del grupo
  descargan exactamente la misma imagen.
- **La VM no guarda nada importante**: la sesion esta en Postgres/Redis, los
  archivos en Cloud Storage. Por eso cualquier VM puede atender cualquier
  peticion.

Los tiempos de arriba son estimados: el tiempo real de arranque se mide con
la linea `LISTA` del log (`t=...s` es el tiempo desde que encendio la VM) y
se reporta en el informe de capacidad.

## 4. Como se retira una VM sin perder trabajo

Cuando baja la carga, el autoescalador pide menos maquinas y el MIG elige
una para borrar:

```
 1. El balanceador deja de mandarle peticiones NUEVAS y espera hasta 30 s
    a que terminen las que estan en curso (connection draining).
 2. Google ejecuta el metadato "shutdown-script" (mig/shutdown.sh):
      docker compose stop   -> envia SIGTERM a la aplicacion
 3. La aplicacion se apaga ordenadamente:
      API:    deja de aceptar conexiones y termina las peticiones (20 s)
      Worker: deja de tomar trabajos y espera los activos (60 s); lo que
              no termina, asynq lo DEVUELVE A LA COLA y otro worker lo
              retoma desde el principio
 4. Google apaga la VM y borra su disco.
```

Tiempos del worker, encadenados para que cada capa respete a la siguiente:

```
60 s (asynq espera)  <  80 s (Docker espera)  <  85 s (shutdown.sh)  <  ~90 s (Google apaga)
```

Si la VM muere de golpe (sin shutdown-script), el trabajo tambien vuelve a la
cola: asynq "presta" cada trabajo por 30 s y lo renueva mientras el worker
vive. Si el worker desaparece, el prestamo vence y el trabajo se reencola.

Y si un trabajo se reintenta, **no se duplican efectos** por la
idempotencia de tres capas que ya existia (`asynq.TaskID`, tabla
`processed_jobs`, restricciones unicas en Postgres). Ademas, desde el commit
`cd9a98d`, un trabajo interrumpido **no marca el archivo como fallido**
(funcion `interrupted` en `internal/tasks/tasks.go`).

## 5. Como decide el autoescalador

### API: por CPU (objetivo 60 %)

El autoescalador calcula cuantas maquinas hacen falta para que la CPU
promedio quede en 60 %:

```
Hay 1 VM al 90 % de CPU.
Carga total = 1 x 90 % = 90 "puntos".
Maquinas necesarias = 90 / 60 = 1.5  -> redondea hacia arriba = 2.
```

Con 2 VMs la carga se reparte: ~45 % cada una. Por que 60 % y no 90 %: la VM
nueva tarda un par de minutos en arrancar, y mientras tanto las existentes
tienen que aguantar el pico. En la Entrega 2 la API se degrado cerca del
100 %.

### Workers: por trabajos en cola (objetivo 2 por VM)

La CPU no sirve para los workers: ffmpeg pone la CPU al 100 % igual con 2
trabajos que con 200 en cola. Lo que mide la demanda es **cuantos trabajos
hay por atender**.

`queue-metrics` (en infra-server) publica cada 30 s, en Cloud Monitoring, la
metrica `custom.googleapis.com/mooc/queue_backlog` = pendientes + activos.

```
backlog = 6 trabajos, objetivo = 2 por VM  ->  6 / 2 = 3 VMs (el maximo)
backlog = 3                                 ->  3 / 2 = 1.5 -> 2 VMs
backlog = 1                                 ->  1 VM (el minimo)
```

Los **activos** cuentan a proposito: una cola sin pendientes puede tener
trabajos en ejecucion, y retirar workers mirando solo los pendientes cortaria
transcodificaciones a medias.

### Frenos para no oscilar

| Parametro | Valor | Para que |
|---|---|---|
| `--cool-down-period` | 180 s | No juzgar a una VM recien creada mientras arranca |
| `--scale-in-control` | max 1 VM cada 600 s | Reducir despacio: si la carga vuelve, aun hay capacidad |
| Minimo / maximo | 1 / 3 | Lo pide el enunciado. Al llegar a 3, no crece mas: la latencia sube o la cola crece |

## 6. Verificaciones de salud: dos tipos a proposito

| Verificacion | Endpoint | La usa | Si falla |
|---|---|---|---|
| `mooc-hc-api-lb` | `/readyz` (la API y sus dependencias responden) | El **balanceador** | Deja de mandarle trafico. La VM sigue viva |
| `mooc-hc-api-vida` | `/healthz` (el proceso vive) | El **MIG** | **Borra y recrea** la VM |
| `mooc-hc-worker-vida` | `/healthz` en :9100 | El MIG | Recrea la VM |
| `mooc-hc-web` | `/login` en :3000 | Balanceador y MIG | Igual que arriba |

Por que dos para la API: si Redis se cae, `/readyz` falla en **todas** las
VMs a la vez. Si el MIG usara `/readyz`, recrearia todas las maquinas sin
arreglar nada (el problema es Redis, no ellas). Con `/healthz`, el MIG solo
recrea una VM cuando el proceso de verdad murio.

Esto es exactamente lo que pide el enunciado: "diferenciar la exclusion de un
destino por el balanceador de su eventual reparacion o reemplazo por el
grupo".

## 7. Donde vive cada cosa

| Que | Donde | Quien lo crea |
|---|---|---|
| Codigo de las VMs de los grupos | `deploy/gcp/mig/startup.sh`, `shutdown.sh` | Repo |
| Que contenedores corre cada rol | `deploy/gcp/mig/<rol>/docker-compose.yml` | Repo |
| Configuracion sin secretos | `deploy/gcp/mig/<rol>/<rol>.env.example` | Repo |
| Configuracion con secretos | Secret Manager: `mooc-api-env`, `mooc-worker-env`, `mooc-web-env`, `mooc-infra-env` | `provision.sh secretos` |
| Copia local de los secretos | `~/.mooc-secrets/<proyecto>/` (permisos 600, fuera del repo) | `provision.sh` |
| Imagenes Docker | Artifact Registry `mooc` | `build.sh` |
| Plantillas, grupos, balanceador | Compute Engine | `provision.sh grupos balanceador` |
| Cola (Redis) | `infra-server`, IP fija `10.20.4.10` | `provision.sh infra` |
| Base de datos | Cloud SQL `mooc-pg` (la misma de la Entrega 2) | Ya existe |
| Archivos | Cloud Storage (el mismo bucket de la Entrega 2) | Ya existe |

Red:

| Subred | Rango | Que hay |
|---|---|---|
| `mooc-app` | `10.20.3.0/24` | VMs de `mooc-api` y `mooc-web` |
| `mooc-worker` | `10.20.2.0/24` | VMs de `mooc-worker` |
| `mooc-infra` | `10.20.4.0/24` | `infra-server` |

Ninguna VM tiene IP publica. Salen a Internet por **Cloud NAT** (para
instalar Docker) y llegan a los servicios de Google por **Private Google
Access**. Solo el balanceador recibe trafico de Internet.

## 8. Como desplegar desde cero

Todo se ejecuta **desde un PC con `gcloud` o desde Cloud Shell**, nunca
dentro de las VMs.

```bash
export PROJECT=dsc-uniandes-20262

# 0. Una sola vez, en la consola: pedir aumento de cuota de vCPU
#    (IAM y administracion > Cuotas > CPUS_ALL_REGIONS, pedir 24).
#    3 API + 3 workers en e2-highcpu-2 ya son 12 vCPU.

# 1. Red, firewall, identidades, registro y secretos (~5 min)
./deploy/gcp/provision.sh elastica

# 2. Construir y subir las imagenes del commit actual (~5-10 min)
./deploy/gcp/build.sh

# 3. infra-server (Redis + metrica de la cola)
./deploy/gcp/provision.sh infra
#    Esperar 1-2 min para que se publique el primer punto de la metrica:
#    el autoescalador de workers la necesita al crearse.

# 4. Grupos y balanceador
./deploy/gcp/provision.sh grupos balanceador
#    El certificado HTTPS tarda 15-60 min en quedar ACTIVE.

# 5. Ver el estado
./deploy/gcp/provision.sh resumen
```

Para desplegar una version nueva despues de un cambio de codigo:

```bash
git commit ...                       # build.sh exige que no haya cambios sin commit
./deploy/gcp/build.sh
./deploy/gcp/provision.sh desplegar  # plantillas nuevas + reemplazo gradual
```

El reemplazo gradual (`max-surge=1`, `max-unavailable=0`) primero crea una VM
nueva y solo cuando esta sana borra una vieja: la capacidad nunca baja.

**Ensayo sin GCP:** todo lo de arriba se probo con un `gcloud` falso que solo
registra los comandos (ver seccion 10). La primera ejecucion real puede
encontrar algun argumento que GCP no acepte; el error de `gcloud` lo dice
claramente.

## 9. Como mirar lo que pasa

Mirar esta bien siempre. **Configurar a mano no**: lo que se haga en una VM
se pierde cuando el grupo la reemplaza.

```bash
Z=--zone=us-central1-a

# Que VMs tiene cada grupo y en que estado
gcloud compute instance-groups managed list-instances mooc-api $Z
gcloud compute instance-groups managed list-instances mooc-worker $Z

# Que decidio el autoescalador y por que
gcloud compute instance-groups managed describe mooc-api $Z --format="yaml(status.autoscaler)"

# Salud de las VMs vistas por el balanceador
gcloud compute backend-services get-health mooc-api-be --global

# Log del arranque de una VM (busca la linea LISTA y su t=)
gcloud compute instances get-serial-port-output mooc-api-p9q1 $Z | grep mooc-startup

# A que replica llega cada peticion
for i in $(seq 10); do curl -sI https://<dominio>/healthz | grep -i x-instance-id; done

# Lineas de la metrica de la cola (linea de tiempo del escenario 3B)
gcloud logging read 'jsonPayload.message:"backlog="' --limit=20 --format="value(timestamp,jsonPayload.message)"

# Entrar a una VM solo para MIRAR (por IAP, no tiene IP publica)
gcloud compute ssh mooc-api-p9q1 $Z --tunnel-through-iap
```

En la consola web: **Compute Engine > Grupos de instancias** (pestana
"Supervision" muestra el tamano del grupo en el tiempo) y **Network
Services > Balanceo de cargas** (salud de cada backend).

## 10. Estado actual de la rama

Rama `entrega-3-infra` (publicada en GitHub):

| Commit | Que hace |
|---|---|
| `db52cfa` | Artifact Registry y `build.sh` |
| `bcdeed9` | `startup.sh` y `shutdown.sh` de los grupos; compose por rol |
| `7d88e6e` | Pasos de la Entrega 3 en `provision.sh`: red, firewall, identidades, secretos, plantillas, grupos, balanceador, despliegue |
| `0ea1992` | `cmd/queue-metrics`: metrica de la cola para el autoescalado de workers |
| `cd9a98d` | API y worker listos para replicas: pool de Postgres configurable, IP real del cliente tras el balanceador, `X-Instance-Id`, apagado ordenado de 60 s, trabajos interrumpidos no marcan fallo |

Que esta probado y como:

| Que | Como se probo | Resultado |
|---|---|---|
| `build.sh` | Construccion local sin subir | OK, etiqueta con el commit |
| `startup.sh` (4 roles) | Contenedor Debian con `gcloud`, `docker` y metadatos simulados | OK, idempotente en reinicio |
| `provision.sh` | `gcloud` falso que registra los comandos | 166 comandos en orden, `.env` sin marcadores pendientes |
| `queue-metrics` | Contra un Redis real en local | El backlog sube de 0 a 6 al encolar 6 trabajos |
| API y worker | 13 pruebas unitarias (`go test ./...`) | OK |

**Nada se ha ejecutado contra GCP todavia.**

Archivos de la Entrega 2 que NO cambiaron (para poder reconstruir la linea
base): `deploy/gcp/startup.sh`, `deploy/gcp/web/`, `deploy/gcp/worker/` y los
pasos `todo`, `vms`, `config` de `provision.sh`.

## 11. Pendientes

Infraestructura y codigo:

- [ ] Pedir el aumento de cuota de vCPU.
- [ ] Primera ejecucion real en GCP y correccion de los argumentos que fallen.
- [ ] Prometheus en `infra-server` con descubrimiento de las VMs de los grupos.
- [ ] Adaptar `migrate.sh` (lee `/etc/mooc/web.env`, que ya no existe) para
      reconstruir la base desde cero.
- [ ] Endpoint para que el estudiante lea recursos `rich_text` (flujo minimo
      del frontend).
- [ ] Borrar `web-server` y `worker-server` de la Entrega 2 cuando la 3
      funcione (costo).

Calidad:

- [ ] Analisis de calidad con SonarQube (lo implementa el equipo).
- [ ] `docs/entrega3/deuda-tecnica.md`.

Documentacion y pruebas (entregables del enunciado):

- [ ] `docs/entrega3/arquitectura.md` con diagramas del despliegue real.
- [ ] Matriz de recorridos del frontend en `docs/entrega3`.
- [ ] `capacity-planning/pruebas_de_carga_entrega3.md`: escenarios 1 y 2
      repetidos, 3A (API) y 3B (workers), comparacion con la Entrega 2.
- [ ] Video de sustentacion.

Riesgos conocidos:

- Algunos argumentos de `gcloud` no se pudieron verificar sin GCP:
  autoescalado por metrica de grupo (`--update-stackdriver-metric`,
  `--stackdriver-metric-single-instance-assignment`), `url-maps import` desde
  `/dev/stdin` y la subred como URL completa en las plantillas.
- `infra-server` sigue siendo un punto unico de falla (Redis). Se documenta.
- Las peticiones anonimas que pasan por el BFF comparten la IP del Cloud NAT
  del frontend en el limite de tasa (afecta al login).

## 12. Glosario

| Termino | Significado |
|---|---|
| **MIG** | Managed Instance Group: grupo de VMs identicas que Google crea, borra y repara |
| **Plantilla de instancia** | Receta inmutable de una VM del grupo |
| **Autoescalador** | Decide cuantas VMs debe tener el grupo segun una senal |
| **Autorreparacion** | El MIG recrea una VM cuyo health check de vida falla |
| **Health check** | Peticion periodica a un endpoint para saber si la VM esta bien |
| **Backend service** | La configuracion del balanceador que apunta a un grupo |
| **Connection draining** | Tiempo que el balanceador espera a que terminen las peticiones de una VM que sale |
| **Metadatos** | Datos clave-valor adjuntos a una VM; la VM los lee en `metadata.google.internal` |
| **startup-script** | Metadato especial: Google lo ejecuta como root en cada arranque |
| **Cuenta de servicio** | Identidad de la VM ante los servicios de Google (no ante Postgres) |
| **Artifact Registry** | Registro privado de imagenes Docker de GCP |
| **Secret Manager** | Almacen de secretos de GCP; cada secreto tiene versiones |
| **Cloud NAT** | Salida a Internet para VMs sin IP publica |
| **Private Google Access** | Acceso a APIs de Google desde VMs sin IP publica |
| **sslip.io** | Dominio publico que resuelve a la IP escrita en el nombre (`34-1-2-3.sslip.io` -> `34.1.2.3`) |
| **Backlog** | Trabajos por atender en la cola: pendientes + activos |
