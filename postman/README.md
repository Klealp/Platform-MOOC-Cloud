# Colecciones de Postman — Plataforma MOOC

Dos archivos:

- **`Platform-MOOC.postman_collection.json`** — la API completa (74 requests en 7
  carpetas), con scripts que **capturan solos** los tokens y los IDs entre
  peticiones (igual que los smoke scripts).
- **`Platform-MOOC-Local.postman_environment.json`** — environment con `baseUrl`
  y credenciales para el stack local.

## Importar

En Postman: **Import** → arrastra los dos `.json`. Selecciona el environment
**"Platform MOOC - Local"** arriba a la derecha. (La colección ya trae valores por
defecto en sus variables, así que también funciona sin el environment.)

Requisito: el stack levantado (`docker compose up --build`) en `localhost:8080`.

## Orden sugerido (el flujo se encadena solo)

1. **2. Auth y sesión → Login (admin)** — guarda `adminToken`.
2. **3. Admin → Crear profesor** — crea el profesor (guarda `teacherEmail`).
3. **2. Auth → Login (profesor)** — guarda `teacherToken`.
4. **4. Autoría** — Crear curso → módulo → unidad → recurso de texto → recurso de
   quiz → crear pregunta.
5. **5. Media** — Cargar video: `1) init` → `2) PUT parte` → `4) complete` →
   `6) estado del asset` (repite hasta `ready`).
6. **4. Autoría → Publicar versión**.
7. **2. Auth** — Registrar estudiante → Verificar correo → Login (estudiante).
8. **6. Aprendizaje** — Inscribirse → outline → eventos de progreso.
9. **7. Quizzes** — Iniciar intento → guardar → enviar.

Los IDs (`courseId`, `versionId`, `moduleId`, `unitId`, `resourceId`, `quizId`,
`assetId`, `enrollmentId`, `attemptId`, …) se guardan automáticamente en las
variables de la colección; no hay que copiarlos a mano.

## La carga del video (importante)

El binario **no pasa por la API**: se sube directo a MinIO con una URL prefirmada.

- **`1) Iniciar carga (init)`** devuelve `asset_id` y `parts[0].url` (se guarda en
  `partUrl`).
- **`2) Subir parte → MinIO`**: es un `PUT` a `{{partUrl}}` (host `localhost:9000`,
  **sin** `Authorization`). En Postman: pestaña **Body → binary → selecciona tu
  `.mp4`**. Postman no permite fijar la ruta del archivo desde el JSON, así que ese
  archivo se elige a mano.
- **`4) Completar carga`** encola el escaneo; el worker valida el MIME real y
  transcodifica a HLS. Consulta **`6) estado del asset`** hasta `ready`.

> Para un solo archivo pequeño basta con que `videoSize` sea > 0 (aprox). Para una
> carga multipart real (archivos grandes en varias partes) es más cómodo el script
> `testdata/smoke_video.sh`, que parte el archivo y sube cada parte.

## Notas

- La autenticación va por **carpeta**: Admin usa `adminToken`, Autoría/Media usan
  `teacherToken`, Aprendizaje/Quizzes usan `studentToken`. Los fija el login.
- La carpeta **1. Operación** (health, `/metrics`, `/openapi.yaml`, `/docs`,
  catálogo público) no requiere sesión.
- Hay una petición de **prueba negativa** ("Rechazo: porcentaje del cliente (422)")
  que debe devolver **422** a propósito: demuestra que el progreso lo valida el
  servidor.
- El envío del quiz manda `Idempotency-Key: {{$guid}}` automáticamente.
