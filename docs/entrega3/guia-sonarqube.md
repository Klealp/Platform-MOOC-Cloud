# Guia: calidad del codigo con SonarQube

Documento de trabajo del equipo para la Entrega 3. Explica **que es
SonarQube**, **como esta montado el pipeline**, **que hay que configurar una
sola vez** y **el primer diagnostico** del codigo. Es la base para
`docs/entrega3/deuda-tecnica.md` (entregable del enunciado).

Ver tambien [`guia-mig.md`](guia-mig.md) (arquitectura elastica).

Contenido:

1. [Que pide el enunciado](#1-que-pide-el-enunciado)
2. [Que es SonarQube, en terminos simples](#2-que-es-sonarqube-en-terminos-simples)
3. [Como esta montado en el repositorio](#3-como-esta-montado-en-el-repositorio)
4. [Configuracion inicial (una sola vez)](#4-configuracion-inicial-una-sola-vez)
5. [Primer diagnostico (analisis local)](#5-primer-diagnostico-analisis-local)
6. [Plan para los hallazgos criticos](#6-plan-para-los-hallazgos-criticos)
7. [Alcance y exclusiones justificadas](#7-alcance-y-exclusiones-justificadas)
8. [Como repetir el analisis en local](#8-como-repetir-el-analisis-en-local)
9. [Del reporte al documento de deuda tecnica](#9-del-reporte-al-documento-de-deuda-tecnica)
10. [Glosario](#10-glosario)

---

## 1. Que pide el enunciado

Paginas 10 a 12 de la Entrega 3 ("Calidad del codigo y deuda tecnica"):

- Ejecutar **SonarQube desde el pipeline de integracion continua** (GitHub
  Actions). El analisis automatizado es obligatorio.
- La automatizacion debe **seguir ejecutando las pruebas**.
- Umbrales, medidos sobre **todo** el codigo (API, workers y frontend), no
  solo el nuevo:

  | Indicador | Requisito |
  |---|---|
  | Duplicacion de codigo | Maximo 20 % |
  | Mantenibilidad | Calificacion A, B o C |
  | Deuda tecnica estimada | Registrar el valor y su interpretacion |

- Documentar fecha, commit, herramienta y version, **perfil de reglas**,
  alcance y exclusiones justificadas. **No excluir codigo de aplicacion**
  para aparentar que se cumple.
- Hallazgos **Major** abiertos: registrarlos con regla, ubicacion, impacto y
  motivo de aplazamiento. Hallazgos **Critical o Blocker**: resolverlos o
  dejar un plan explicito (pasos, responsable, plazo).
- Entregar el reporte asociado al **commit final** y el **enlace a la
  ejecucion del pipeline**.
- No se exige desplegar en la nube desde el pipeline.

## 2. Que es SonarQube, en terminos simples

SonarQube es un **revisor automatico de codigo**. Lee el codigo fuente (sin
ejecutarlo) y lo compara contra un conjunto de **reglas** del tipo "una
funcion no deberia ser tan compleja" o "este literal se repite 6 veces,
usa una constante".

```
 codigo fuente  ->  escaner (en el pipeline)  ->  servidor SonarQube  ->  panel web
                    lee los archivos,             aplica las reglas,      hallazgos,
                    los empaqueta y               calcula metricas,       metricas,
                    los sube                      guarda el historial     historial
```

Conceptos clave:

| Concepto | Que es | Ejemplo en este proyecto |
|---|---|---|
| **Regla** | Una buena practica que se verifica | `go:S3776`: complejidad cognitiva maxima de 15 por funcion |
| **Hallazgo (issue)** | Un lugar del codigo que rompe una regla | `HandleScanAsset` tiene complejidad 34 |
| **Severidad** | Que tan grave es: Info, Minor, Major, Critical, Blocker | Complejidad excesiva = Critical |
| **Tipo** | Bug (falla probable), Vulnerability (seguridad), Code Smell (mantenibilidad) | Casi todo lo nuestro son code smells |
| **Perfil de calidad** | El conjunto de reglas activas por lenguaje | "Sonar way" es el perfil por defecto |
| **Deuda tecnica** | Tiempo estimado para corregir todos los code smells | 4 h hoy (seccion 5) |
| **Mantenibilidad (A-E)** | Calificacion segun deuda / tamano del codigo. A = deuda menor al 5 % del tiempo estimado de escribir el codigo | A |
| **Duplicacion** | Porcentaje de lineas repetidas en bloques | 0,9 % |
| **Quality Gate** | Condiciones que un analisis debe cumplir para "pasar" | Ver nota abajo |

**Complejidad cognitiva**, que es la regla que mas aparece: mide que tan
dificil es *entender* una funcion. Suma un punto por cada `if`, `for`,
`switch`, `&&`... y suma mas cuando estan anidados. Una funcion con 34 es
dificil de leer y de modificar sin romper algo.

**Nota sobre el Quality Gate:** el de SonarQube por defecto ("Sonar way")
evalua solo el **codigo nuevo** (por ejemplo, exige 80 % de cobertura en lo
nuevo). El enunciado mide el **codigo completo**. Por eso el pipeline no
depende del Quality Gate: tiene un paso propio que verifica los umbrales del
enunciado (`.github/scripts/sonar-umbrales.sh`).

## 3. Como esta montado en el repositorio

| Archivo | Para que |
|---|---|
| [`.github/workflows/calidad.yml`](../../.github/workflows/calidad.yml) | El pipeline de GitHub Actions |
| [`sonar-project.properties`](../../sonar-project.properties) | Que se analiza, que se excluye, donde esta la cobertura |
| [`.github/scripts/sonar-umbrales.sh`](../../.github/scripts/sonar-umbrales.sh) | Verifica duplicacion <= 20 % y mantenibilidad <= C, y escribe el resumen |

El pipeline corre en cada push a `main` o a ramas `entrega-*`, en cada pull
request y a mano (boton "Run workflow"):

```
             +----------------+
 push  --->  | Pruebas Go     |  go vet, go test con cobertura (coverage.out)
             +----------------+ \
                                 +--> +------------------------------+
             +----------------+ /     | SonarQube                    |
             | Frontend       |       |  1. escanea (con coverage)   |
             | typecheck,build|       |  2. espera a que el servidor |
             +----------------+       |     procese ESE analisis     |
                                      |  3. verifica los umbrales    |
                                      |  4. resumen en la ejecucion  |
                                      +------------------------------+
```

- Si una prueba falla, SonarQube no corre: no se mide codigo roto.
- El paso 3 falla el pipeline si no se cumple un umbral.
- El paso 4 deja una tabla en la pagina de la ejecucion (pestana "Summary")
  con el **commit**, los valores y el enlace al panel. Esa pagina es la
  evidencia "trazable al commit" que pide el enunciado.

## 4. Configuracion inicial (una sola vez)

El repositorio es **publico**, asi que **SonarQube Cloud es gratis**. No hay
que montar ni pagar un servidor.

Lo hace una persona con permisos de administrador del repositorio
(`Klealp/Platform-MOOC-Cloud`):

1. **Entrar a <https://sonarcloud.io>** con "Log in with GitHub".
2. **Importar la organizacion**: "+" > "Analyze new project" > elegir la
   organizacion de GitHub `Klealp`. SonarQube Cloud crea una organizacion
   con una llave (normalmente `klealp`).
3. **Importar el proyecto** `Platform-MOOC-Cloud`. La llave suele quedar
   `Klealp_Platform-MOOC-Cloud`.
   - Si la organizacion o la llave quedan distintas, **cambiarlas en
     `sonar-project.properties`**.
4. **Apagar el analisis automatico**: en el proyecto, *Administration >
   Analysis Method* > desactivar **Automatic Analysis**. Si queda activo, el
   analisis del pipeline falla porque los dos chocan. Ademas, el automatico
   no usa la cobertura ni nuestra configuracion.
5. **Crear un token**: *My Account > Security > Generate Token*. Copiarlo
   (solo se ve una vez).
6. **Guardarlo en GitHub**: en el repositorio, *Settings > Secrets and
   variables > Actions > New repository secret*, nombre **`SONAR_TOKEN`**.
   Nunca pegarlo en el codigo.
7. **Perfil de reglas de Go** (ver seccion 5, por que importa):
   *Quality Profiles* > Go > copiar "Sonar way" como **"MOOC Go"** >
   activar las reglas de la tabla de abajo > *Set as default* (o asignarlo
   al proyecto). Revisar primero cuantas reglas trae "Sonar way" de Go en
   SonarQube Cloud: puede traer mas que el servidor local.
8. **Ejecutar el pipeline**: hacer push o, en la pestana *Actions* >
   "calidad" > *Run workflow*.

Reglas a activar en el perfil "MOOC Go" (las mismas del analisis local):

| Regla | Que verifica |
|---|---|
| `go:S3776` | Complejidad cognitiva de una funcion (maximo 15) |
| `go:S1192` | Literales de texto duplicados (usar constantes) |
| `go:S138` | Funciones demasiado largas (maximo 120 lineas) |
| `go:S134` | Anidamiento excesivo de `if`/`for`/`switch` |
| `go:S1067` | Expresiones con demasiados operadores logicos |
| `go:S107` | Funciones con demasiados parametros |
| `go:S1186` | Funciones vacias sin comentario |
| `go:S1871` | Ramas de `if`/`switch` con el mismo codigo |
| `go:S108` | Bloques vacios |
| `go:S100`, `go:S117` | Convenciones de nombres |
| `go:S1479` | `switch` con demasiados casos |
| `go:S4144` | Funciones con implementacion identica |

Activar mas reglas hace el analisis **mas estricto**, no mas facil. Eso esta
permitido; lo que el enunciado prohibe es reducir el alcance para aparentar
que se cumple. Documentar el perfil usado en `deuda-tecnica.md`.

## 5. Primer diagnostico (analisis local)

> **Esto es una vista previa**, hecha con un SonarQube local, no con el
> pipeline. La **linea base oficial** es la del pipeline sobre el commit
> final de la entrega. Sirve para saber desde ya que hay que corregir.

Condiciones:

| Dato | Valor |
|---|---|
| Fecha | 2026-10-24 |
| Commit | `cd9a98d` (rama `entrega-3-infra`) mas la refactorizacion de una prueba |
| Herramienta | SonarQube Community 26.10.0.133078 (local, Docker) y sonar-scanner-cli |
| Perfiles | TypeScript: Sonar way (198 reglas); CSS: Sonar way (25); Docker: Sonar way (18); **Go: "MOOC Go" (19 reglas)** |
| Alcance | `cmd/`, `internal/`, `frontend/src/`, `frontend/next.config.ts`, Dockerfiles |
| Tamano | 12.787 lineas: Go 6.668, TypeScript 5.925, CSS 121, Dockerfile 73 |

### El hallazgo mas importante: Go casi no se estaba revisando

Con el perfil por defecto, **Go tenia solo 6 reglas activas** (TypeScript
tiene 198). El primer analisis dio 2 hallazgos en todo el proyecto, ambos en
el frontend: Go parecia perfecto porque casi no se miraba. Por ejemplo, la
regla de complejidad cognitiva (`go:S3776`) no estaba activa, aunque
`handlers_quiz_attempts.go` suma 89 de complejidad cognitiva.

Con el perfil "MOOC Go" (19 reglas) aparece el diagnostico real.

### Resultados

| Indicador | Valor | Umbral | Cumple |
|---|---|---|---|
| Duplicacion | **0,9 %** | <= 20 % | Si |
| Mantenibilidad | **A** | A, B o C | Si |
| Deuda tecnica estimada | **4,0 h** (0,5 jornadas de 8 h) | Se registra | - |
| Ratio de deuda | 0,1 % | - | - |
| Bugs / vulnerabilidades / hotspots | 0 / 0 / 0 | - | - |
| Code smells | **20** (15 Critical, 5 Major) | - | - |
| Cobertura | 3,8 % (solo Go; el frontend no tiene pruebas) | - | - |

**Interpretacion:** los umbrales se cumplen con mucho margen, pero eso no
significa que no haya deuda:

- 4 horas de deuda en 12.800 lineas es poco, porque SonarQube solo cuenta el
  tiempo de arreglar los code smells que detecta.
- **No mide** la deuda de arquitectura, pruebas u operacion: la cobertura
  del 3,8 %, `infra-server` como punto unico de falla, el antimalware de
  prueba, etc. Esa deuda tambien va en `deuda-tecnica.md` (el enunciado lo
  pide: "la estimacion de la herramienta no sustituye la revision de
  decisiones arquitectonicas, configuracion, pruebas y operacion").

### Hallazgos por regla

| Regla | Cantidad | Severidad | Donde |
|---|---|---|---|
| `go:S3776` complejidad cognitiva > 15 | 9 | Critical | Handlers de la API y el escaneo (tabla de la seccion 6) |
| `go:S1192` literal duplicado | 5 | Critical | Mensajes de error repetidos: "Intento no encontrado." (6), "Archivo no encontrado." (6), "Carga no encontrada." (6), "Recurso no encontrado." (3), "resource:" (5) |
| `go:S1067` expresion compleja | 1 | Critical | `handlers_quiz_attempts.go:288` (`showFeedback`) |
| `go:S138` funcion > 120 lineas | 3 | Major | `validateForPublication` (162), `handleProgressEvent` (128), `HandleScanAsset` (121) |
| `typescript:S6850` encabezado sin contenido accesible | 1 | Major | `frontend/src/components/ui/card.tsx:43` |
| `typescript:S6853` etiqueta sin control asociado | 1 | Major | `frontend/src/components/ui/input.tsx:40` |

Corregido durante el diagnostico: una prueba escrita en esta entrega
(`internal/queuemetrics/queuemetrics_test.go`) tenia complejidad 19; se
dividio en ayudantes y el hallazgo se cerro.

## 6. Plan para los hallazgos criticos

El enunciado exige resolver los Critical o dejar un plan explicito. Propuesta
inicial (falta asignar responsables y fechas en el equipo):

| # | Funcion | Complejidad | Accion propuesta | Riesgo al tocarla |
|---|---|---|---|---|
| C1 | `HandleScanAsset` (`internal/tasks/scan.go:52`) | 34 | Separar en pasos: cargar asset, detectar MIME, antimalware, decidir siguiente tarea | Medio: hay smoke tests (`smoke_eicar.sh`, `smoke_pdf.sh`) |
| C2 | `validateForPublication` (`handlers_courses.go:620`) | 34 | Una funcion por regla de publicacion que devuelve sus errores; la principal solo las junta | Medio: reglas de negocio de publicacion |
| C3 | `idempotency` (`idempotency.go:29`) | 24 | Extraer la lectura de la respuesta guardada y el guardado a funciones aparte | Alto: idempotencia del envio de quizzes. Escribir pruebas antes |
| C4 | `handleSubmitAttempt` (`handlers_quiz_attempts.go:436`) | 21 | Separar validacion, calificacion y persistencia | Alto: calificacion. Pruebas antes |
| C5 | `validateAnswers` (`handlers_quiz_attempts.go:389`) | 19 | Validar cada pregunta en una funcion | Bajo: funcion pura, facil de probar |
| C6 | `handleAdminUpdateUser` (`handlers_admin.go:128`) | 19 | Separar cambio de rol y cambio de estado | Medio: regla "siempre un admin activo" |
| C7 | `gradeAttempt` (`handlers_quiz_attempts.go:554`) | 16 | Extraer el puntaje de una pregunta | Bajo: funcion pura |
| C8 | `handleProgressEvent` (`handlers_learning.go:482`) | 16 | Un manejador por tipo de evento (open, heartbeat, complete) | Medio: progreso calculado en el servidor |
| C9 | `handleCreateResource` (`handlers_content.go:167`) | 16 | Extraer la validacion por tipo de recurso | Bajo |
| C10 | Literales duplicados (5 hallazgos `go:S1192`) | - | Constantes para los mensajes de error repetidos | Muy bajo: cambio mecanico |
| C11 | `showFeedback` (`handlers_quiz_attempts.go:288`) | - | Extraer la condicion a una funcion con nombre | Muy bajo |

Orden sugerido: primero lo de **riesgo bajo y mucho impacto** (C10, C11, C5,
C7, C9); luego el resto **escribiendo antes pruebas unitarias** de la logica
(sobre todo C3 y C4, que tocan la calificacion y la idempotencia). Hoy casi
no hay pruebas: refactorizar sin ellas puede romper reglas de negocio sin que
nadie lo note.

Para cada correccion que se haga en esta entrega, `deuda-tecnica.md` debe
registrar el commit y la evidencia (el hallazgo cerrado en SonarQube y las
pruebas que pasan). Lo que quede abierto pasa a la linea base con su plan.

## 7. Alcance y exclusiones justificadas

| Que | Se analiza | Justificacion |
|---|---|---|
| `cmd/`, `internal/` (API, workers, `queue-metrics`) | Si | Codigo de aplicacion |
| `frontend/src/`, `frontend/next.config.ts` | Si | Codigo de aplicacion |
| `Dockerfile.*` | Si | Definen lo que se despliega |
| `*_test.go` | Como **pruebas**, no como fuente | Asi es como SonarQube debe tratarlas |
| `frontend/src/lib/api/schema.d.ts` | No | Generado por `openapi-typescript` desde `internal/openapi/openapi.yaml` (`npm run gen:api`); no se edita a mano |
| `capacity-planning/` | No | Herramientas y scripts de pruebas de carga; no se despliegan |
| `deploy/`, `docs/` (shell, YAML) | No | SonarQube Cloud no analiza shell |

Ninguna exclusion quita codigo de aplicacion.

## 8. Como repetir el analisis en local

Sirve para revisar un cambio antes de hacer push. Requiere Docker y unos 3 GB
de RAM libres.

```bash
# 1. Servidor SonarQube local (tarda 1-2 min en arrancar)
docker run -d --name sonar-local -p 127.0.0.1:9000:9000 \
  -e SONAR_ES_BOOTSTRAP_CHECKS_DISABLE=true sonarqube:community
# Abrir http://127.0.0.1:9000 (la primera vez: admin / admin, pide cambiarla)
# Crear un token en My Account > Security, y el perfil "MOOC Go" (seccion 4)

# 2. Cobertura de Go
go test -covermode=atomic -coverprofile=coverage.out ./...

# 3. Escaner (desde la raiz del repo)
docker run --rm --network host -u "$(id -u):$(id -g)" \
  -e SONAR_HOST_URL=http://127.0.0.1:9000 -e SONAR_TOKEN=<token> \
  -v "$PWD:/usr/src" sonarsource/sonar-scanner-cli:latest -Dsonar.organization=

# 4. Umbrales (el mismo script del pipeline)
SONAR_TOKEN=<token> .github/scripts/sonar-umbrales.sh

# Apagar / volver a encender el servidor
docker stop sonar-local
docker start sonar-local
```

`-Dsonar.organization=` anula la organizacion, que solo existe en SonarQube
Cloud.

## 9. Del reporte al documento de deuda tecnica

`docs/entrega3/deuda-tecnica.md` debe tener (pagina 11-12 del enunciado):

1. **Alcance y metodo**: componentes revisados, herramientas (SonarQube,
   revision manual de arquitectura y operacion) y actividades.
2. **Medicion inicial**: los tres indicadores con unidades, interpretacion y
   **enlace al reporte del commit final** (el panel de SonarQube Cloud y la
   ejecucion del pipeline).
3. **Inventario**: cada hallazgo con los campos de la tabla del enunciado
   (identificador, descripcion y ubicacion, evidencia, impacto, prioridad,
   tratamiento, responsable y plazo, estado). Incluir tambien deuda que
   SonarQube no ve, por ejemplo:
   - Cobertura de pruebas del 3,8 % y frontend sin pruebas.
   - `next lint` sin configuracion de ESLint (el pipeline no lo corre).
   - `go mod tidy` dentro de los Dockerfile (la imagen podria no
     corresponder exactamente al `go.sum` del commit).
   - `infra-server` (Redis) como punto unico de falla.
   - Antimalware de prueba (solo detecta EICAR).
   - Las peticiones anonimas por el BFF comparten la IP del Cloud NAT en el
     limite de tasa.
   - El estudiante no puede leer recursos `rich_text`.
4. **Priorizacion y plan**: orden, acciones, responsables, plazos.
5. **Correcciones hechas**: commit y evidencia; separadas de lo pendiente.

"No basta con copiar la lista de SonarQube": hay que explicar por que cada
hallazgo importa para esta plataforma (por ejemplo, la complejidad de
`handleSubmitAttempt` importa porque ahi se califican los quizzes).

## 10. Glosario

| Termino | Significado |
|---|---|
| **SonarQube Cloud** | SonarQube como servicio (antes SonarCloud). Gratis para repositorios publicos |
| **Escaner** | Programa que lee el codigo y lo sube al servidor (`sonarqube-scan-action` en el pipeline) |
| **Code smell** | Codigo que funciona pero es dificil de mantener |
| **Complejidad cognitiva** | Que tan dificil es entender una funcion; suma por cada bifurcacion y mas si esta anidada |
| **Deuda tecnica (sqale_index)** | Minutos estimados para corregir todos los code smells |
| **Ratio de deuda** | Deuda / costo estimado de escribir el codigo desde cero |
| **Calificacion de mantenibilidad** | A (ratio <= 5 %), B (<= 10 %), C (<= 20 %), D (<= 50 %), E (> 50 %) |
| **Perfil de calidad** | Conjunto de reglas activas para un lenguaje |
| **Quality Gate** | Condiciones de aprobado/reprobado de un analisis (por defecto, sobre codigo nuevo) |
| **Codigo nuevo** | Lo que cambio desde una referencia (version anterior, rama principal o N dias) |
| **Hotspot de seguridad** | Codigo sensible que una persona debe revisar (no es necesariamente una vulnerabilidad) |
| **report-task.txt** | Archivo que deja el escaner con la URL de la tarea en el servidor; lo usa el script de umbrales |
