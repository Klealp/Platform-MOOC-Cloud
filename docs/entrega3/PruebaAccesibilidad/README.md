# Prueba automática de accesibilidad (WCAG 2.2 AA)

Auditoría del frontend con **Playwright** (un navegador Chromium controlado por
código) y **axe-core** (el motor de reglas que usan Lighthouse y axe DevTools).
Cubre la condición de aceptación del enunciado: *"la auditoría automática de
accesibilidad no puede presentar incumplimientos críticos"*.

Todo corre **dentro de un contenedor**: no hay que instalar Node, Playwright ni
navegadores en la máquina, y no se modifica ningún archivo de la API ni del
`docker-compose.yml`.

## Qué hace

1. Arranca un contenedor con la imagen oficial `mcr.microsoft.com/playwright`,
   conectado a la red de Docker Compose del proyecto.
2. Inicia sesión como estudiante, profesor y administrador (una vez por rol).
3. Visita las páginas principales de cada rol, en **tema claro y oscuro**:

   | Rol | Páginas |
   |---|---|
   | Anónimo | inicio, catálogo, ficha de un curso, iniciar sesión, crear cuenta, página 404 |
   | Estudiante | panel, mis cursos, reproductor del curso, mis insignias, verificación pública de una insignia, mis sesiones |
   | Profesor | mis cursos, nuevo curso, editor de un curso |
   | Administrador | usuarios y roles, bitácora, insignias, todos los cursos |

4. Ejecuta axe sobre cada página con las reglas de WCAG 2.0, 2.1 y 2.2 en
   niveles A y AA.
5. Guarda el resultado en `resultados/` y termina en **APROBADA** solo si
   ninguna página tiene incumplimientos de impacto `critical` o `serious`.

La prueba **solo lee**: no inscribe, no inicia quizzes ni edita contenido. Lo
único que deja en el sistema es una sesión nueva por rol (y su línea
`user.login` en la bitácora).

## Requisitos

- Docker en marcha y el proyecto levantado (`make up`).
- Datos de prueba. Lo más sencillo es haber corrido al menos una vez
  `./docs/entrega1/testdata/smoke.sh`, que crea un profesor con un curso
  publicado y un estudiante inscrito. Sin esos datos, las páginas que los
  necesitan aparecen en el reporte como "no auditadas".

## Paso a paso

1. **Levanta el proyecto** (si no está corriendo), desde la raíz del repo:

   ```bash
   make up
   ```

   Espera a que el frontend responda en <http://localhost:3001>.

2. **(Solo la primera vez) crea datos de prueba**, si la base está vacía:

   ```bash
   ./docs/entrega1/testdata/smoke.sh
   ```

3. **Ejecuta la auditoría**, desde la raíz del repo:

   ```bash
   "./docs/entrega3/PruebaAccesibilidad/ejecutar.sh"
   ```

   Las comillas son necesarias por el espacio en el nombre de la carpeta. La
   primera vez descarga la imagen de Playwright (~1,5 GB); las siguientes
   tarda alrededor de un minuto.

   El script busca solo los usuarios: el último estudiante de `smoke.sh` con
   una inscripción activa, el último profesor de `smoke.sh` con cursos y el
   administrador del `.env`. Para usar otros, pásalos como variables:

   ```bash
   ESTUDIANTE_EMAIL=ana@mooc.local ESTUDIANTE_PASSWORD='...' \
   PROFESOR_EMAIL=profe@mooc.local PROFESOR_PASSWORD='...' \
   "./docs/entrega3/PruebaAccesibilidad/ejecutar.sh"
   ```

4. **Lee el resultado en la terminal.** Aparece una línea por página y tema
   (`ok` o `FALLA`) y al final:

   ```
   Resultado: APROBADA (38 auditorias, 0 omitidas)
   ```

   El código de salida es `0` si aprueba, `1` si hay incumplimientos críticos o
   graves y `2` si la prueba no pudo ejecutarse.

5. **Revisa el reporte** en `resultados/REPORTE.md` (se reemplaza en cada
   ejecución):
   - **Resumen:** una tabla con cada página, rol y tema, y cuántos
     incumplimientos tiene por impacto.
   - **Páginas no auditadas:** las que se saltaron y por qué (faltan
     credenciales o datos).
   - **Detalle de incumplimientos:** la regla de axe, su criterio WCAG, un
     enlace a la guía para corregirla y los elementos HTML afectados.

   `resultados/detalle.json` tiene la salida completa de axe, por si se
   necesita más información de un incumplimiento.

## Problemas frecuentes

| Mensaje | Causa y solución |
|---|---|
| `No existe la red 'mooc-platform_default'` | El proyecto no está levantado, o la carpeta del repo tiene otro nombre. Corre `make up`; si el nombre difiere, pasa `RED=<nombre>` (lo ves con `docker network ls`). |
| `error getting credentials` al descargar la imagen | Falla del puente entre WSL y Docker Desktop. Reinicia Docker Desktop y ejecuta `wsl --shutdown` en PowerShell. |
| `no se pudo iniciar sesion como ...` | Credenciales incorrectas, usuario suspendido o sin verificar. Revisa los usuarios con la consulta de `docs/entrega1/CREDENCIALES.md`. |
| Páginas "no auditadas" por falta de datos | El estudiante no tiene inscripciones o insignias, o el profesor no tiene cursos. Corre `smoke.sh` o pasa otros usuarios. |

## Archivos

| Archivo | Para qué |
|---|---|
| `ejecutar.sh` | Elige los usuarios y lanza el contenedor de Playwright. |
| `prueba/auditoria.mjs` | Recorre las páginas, ejecuta axe y escribe el reporte. |
| `prueba/package.json` | Versiones fijas de Playwright y axe (deben coincidir con la imagen). |
| `resultados/` | Último `REPORTE.md` y `detalle.json`. |

Para subir de versión Playwright hay que cambiar a la vez `prueba/package.json`
y la etiqueta de la imagen en `ejecutar.sh`.
