# testdata

Solo lo estrictamente necesario para demostrar comportamientos concretos.
Para probar cargas reales usa tus propios archivos.

| Archivo             | Para que sirve |
|---------------------|----------------|
| `smoke.sh`          | Recorre el flujo completo de extremo a extremo con curl. Requiere `jq`. |
| `smoke_video.sh`    | Curso con 2 modulos x 2 unidades y un video real subido por carga multipart en una de las unidades (recibe la ruta del video por argumento o `VIDEO_PATH`). Requiere `jq` y `sha256sum`/`shasum`. |
| `smoke_imagenes.sh` | Curso con 1 modulo y 3 unidades (texto + 2 imagenes) y un quiz de 5 preguntas x 4 opciones. Prueba la calificacion sobre varias preguntas (un intento con 3/5 que reprueba y otro con 5/5 que aprueba) y ademas **consume todo el material obligatorio** (texto + las 2 imagenes, con eventos `open`/`heartbeat`/`complete`) antes de terminar. Resultado esperado: la inscripcion llega a `state=approved` y **SI se emite la insignia** (ver "Insignia" mas abajo). Las imagenes pueden ser propias (argumentos o `IMAGE1_PATH`/`IMAGE2_PATH`, con conversion automatica de rutas de Windows) o, si no se pasan, dos PNG minimos generados en caliente. Requiere `jq` y `sha256sum`/`shasum`. |
| `smoke_pdf.sh`      | Curso con 3 modulos, cada uno con 1 unidad y hasta 3 recursos: texto, PDF real y (solo en el Modulo 3) un quiz de 3 preguntas x 3 opciones. El estudiante **aprueba el quiz pero a proposito nunca consume los textos ni los PDF** (no manda ningun evento de progreso sobre ellos). Resultado esperado: el quiz queda `passed=true` pero la inscripcion se queda en `state=in_progress` y **NO se emite insignia** (ver "Insignia" mas abajo). Recibe las 3 rutas de los PDF por argumento o en `PDF1_PATH`/`PDF2_PATH`/`PDF3_PATH`, con conversion automatica de rutas de Windows. Requiere `jq` y `sha256sum`/`shasum`. |
| `eicar.txt`         | Archivo de prueba EICAR. No es un virus: es un texto que todos los antivirus reconocen como si lo fuera, creado justamente para probar la deteccion. Subelo como `kind=file` y el asset debe quedar en estado `infected`, con el objeto borrado del almacenamiento y una linea `asset.infected` en la auditoria. |
| `smoke_eicar.sh`    | Prueba de humo NEGATIVA: un profesor nuevo crea un curso con 1 modulo/1 unidad e intenta subir `eicar.txt` como recurso descargable. Muestra los tres puntos de control donde la API lo rechaza: el asset termina en `status=infected` (el worker borra el objeto de MinIO), `GET /assets/{id}/url` responde HTTP 409 porque nunca se firma un archivo que no este `ready`, y si igual se adjunta como recurso y se intenta publicar, `POST /versions/{id}/publish` responde HTTP 422 con el problema `asset_not_ready`. No requiere argumentos. Requiere `jq` y `sha256sum`/`shasum`. |
| `smoke_version_nueva.sh` | Prueba de humo de la restriccion 7 (version publicada inmutable): un profesor crea un curso de 1 modulo/2 unidades (texto, y texto + imagen), lo publica, un estudiante lo consume completo y se gana su insignia. Despues el profesor crea una version 2 con `POST /courses/{id}/versions` (clona conservando `stable_id`), le cambia el titulo y el texto a la lectura de la Unidad 1, reemplaza la imagen de la Unidad 2 (borra el recurso original y sube una nueva marcada `required:false`, porque no existe un endpoint para cambiar el `asset_id` de un recurso existente) y publica esa version 2. Verifica que el estudiante que ya habia aprobado sigue en `state=approved` (su progreso sobrevive porque los recursos que contaban para su aprobacion no cambiaron de `stable_id`) y que su insignia es exactamente la misma (mismo `public_code`, mismo `issued_at`, sigue siendo `valid:true` en `GET /public/badges/{code}`), sin importar que el contenido del curso haya cambiado debajo de sus pies. Recibe las 2 rutas de imagen por argumento o en `IMAGE1_PATH`/`IMAGE2_PATH` (con conversion automatica de rutas de Windows; por defecto usa las mismas imagenes de `smoke_curso_completo.sh`). Requiere `jq` y `sha256sum`/`shasum`. |
| `smoke_curso_completo.sh` | Curso "grande" con 5 modulos: 3 con 1 sola unidad de solo texto y 2 (Modulo 3 y Modulo 5) con 3 unidades cada uno, y cada unidad de esos con 2-3 recursos de formatos distintos a la vez (texto + PDF/imagen/video/enlace externo/PPTX). El PPTX se sube como `kind=file` y `type=download` porque el contrato no tiene un tipo de recurso para presentaciones de oficina; el Modulo 5 reutiliza el `asset_id` del video y de un PDF/imagen ya subidos en el Modulo 3 en vez de volver a subirlos. Incluye un quiz de cierre de 10 preguntas x 2 opciones. Dos estudiantes nuevos se inscriben: uno consume TODO el material y aprueba el quiz (10/10) -> `state=approved` y SI recibe insignia; el otro tambien consume todo el material pero reprueba el quiz (3/10, 30% < 70%) -> nunca llega a `approved` y NO recibe insignia, aunque vio el 100% del contenido (variante de la regla de "Insignia" de mas abajo, vista con el mismo estudiante consumiendo todo). Recibe las 6 rutas por argumento o en `PPTX_PATH`/`VIDEO_PATH`/`IMAGE1_PATH`/`IMAGE2_PATH`/`PDF1_PATH`/`PDF2_PATH` (con conversion automatica de rutas de Windows); si no se pasa nada usa unas rutas de ejemplo por defecto que hay que ajustar a tu maquina. Requiere `jq` y `sha256sum`/`shasum`. |

## Insignia: aprobar el quiz no es lo mismo que aprobar el curso

`smoke_imagenes.sh` y `smoke_pdf.sh` son el mismo experimento visto desde los
dos lados, a proposito, para dejar visible una regla que no es obvia leyendo
solo los endpoints: **la insignia se emite por la inscripcion completa, no
por el quiz.** La calcula `progress.Recompute` (`internal/progress/progress.go`):
la inscripcion solo pasa a `approved` (y solo ahi se encola la insignia) si
`progress_pct` (recursos obligatorios visibles marcados `completed` via
eventos `open`/`heartbeat`/`complete`) llega al 100% **y ademas** todos los
quizzes obligatorios estan aprobados. Ninguna de las dos condiciones alcanza
sola.

- `smoke_imagenes.sh` cumple las dos: consume el texto y las 2 imagenes
  (`consume_resource`, con el dwell minimo real de `MinDwellSeconds`: 15s
  para `rich_text`, 5s para el resto) y aprueba el quiz. Termina con
  `state:"approved"`, `newly_approved:true` y una insignia real y verificable
  en `GET /public/badges/{code}`.
- `smoke_pdf.sh` solo aprueba el quiz (3/3) y **deliberadamente** no toca los
  6 recursos de texto/PDF. Termina con `passed:true` pero
  `state:"in_progress"` y `required_done` muy por debajo de
  `required_total` (el quiz aprobado marca como completo unicamente su
  propio recurso). `GET /badges` devuelve `[]`.

Si alguna vez ves lo contrario (insignia sin haber consumido el material, o
ausente habiendo consumido todo y aprobado el quiz), es una regresion en
`progress.Recompute` o en `markQuizResourceComplete`, no un problema de estos
scripts.
