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
