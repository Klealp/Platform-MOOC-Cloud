# Auditoría automática de accesibilidad — WCAG 2.2 AA

- **Resultado:** ✅ APROBADA
- **Fecha:** 2026-10-24 23:21:49 UTC
- **Sitio auditado:** `http://web:3000` (frontend en Docker Compose)
- **Herramientas:** Playwright 1.49.1 (Chromium) + axe-core vía @axe-core/playwright 4.10.1
- **Reglas:** wcag2a, wcag2aa, wcag21a, wcag21aa, wcag22aa
- **Criterio de aprobación:** ninguna página con incumplimientos de impacto `critical` o `serious`. Los `moderate` y `minor` se reportan pero no bloquean.
- **Usuarios:** estudiante: estudiante1792884081@mooc.local · profesor: profe1792884081@mooc.local · admin: admin@mooc.local

> axe detecta de forma automática alrededor de un tercio de los criterios WCAG. Lo que no puede juzgar (calidad del texto alternativo, orden lógico de lectura, subtítulos de los videos) requiere revisión manual.

## Resumen

| Página | Rol | Tema | Ruta | Críticas | Graves | Moderadas | Menores | Estado |
|---|---|---|---|---:|---:|---:|---:|---|
| Inicio | anonimo | claro | `/` | 0 | 0 | 0 | 0 | ✅ |
| Inicio | anonimo | oscuro | `/` | 0 | 0 | 0 | 0 | ✅ |
| Catalogo | anonimo | claro | `/catalog` | 0 | 0 | 0 | 0 | ✅ |
| Catalogo | anonimo | oscuro | `/catalog` | 0 | 0 | 0 | 0 | ✅ |
| Ficha de curso | anonimo | claro | `/catalog/fundamentos-de-la-nube-4` | 0 | 0 | 0 | 0 | ✅ |
| Ficha de curso | anonimo | oscuro | `/catalog/fundamentos-de-la-nube-4` | 0 | 0 | 0 | 0 | ✅ |
| Iniciar sesion | anonimo | claro | `/login` | 0 | 0 | 0 | 0 | ✅ |
| Iniciar sesion | anonimo | oscuro | `/login` | 0 | 0 | 0 | 0 | ✅ |
| Crear cuenta | anonimo | claro | `/register` | 0 | 0 | 0 | 0 | ✅ |
| Crear cuenta | anonimo | oscuro | `/register` | 0 | 0 | 0 | 0 | ✅ |
| Pagina 404 | anonimo | claro | `/pagina-que-no-existe` | 0 | 0 | 0 | 0 | ✅ |
| Pagina 404 | anonimo | oscuro | `/pagina-que-no-existe` | 0 | 0 | 0 | 0 | ✅ |
| Panel | estudiante | claro | `/dashboard` | 0 | 0 | 0 | 0 | ✅ |
| Panel | estudiante | oscuro | `/dashboard` | 0 | 0 | 0 | 0 | ✅ |
| Mis cursos | estudiante | claro | `/learn` | 0 | 0 | 0 | 0 | ✅ |
| Mis cursos | estudiante | oscuro | `/learn` | 0 | 0 | 0 | 0 | ✅ |
| Reproductor del curso | estudiante | claro | `/learn/25a60e9e-f61f-4ed5-8eaf-e5f320c2c787` | 0 | 0 | 0 | 0 | ✅ |
| Reproductor del curso | estudiante | oscuro | `/learn/25a60e9e-f61f-4ed5-8eaf-e5f320c2c787` | 0 | 0 | 0 | 0 | ✅ |
| Mis insignias | estudiante | claro | `/learn/badges` | 0 | 0 | 0 | 0 | ✅ |
| Mis insignias | estudiante | oscuro | `/learn/badges` | 0 | 0 | 0 | 0 | ✅ |
| Verificacion publica de insignia | estudiante | claro | `/badges/verify/imflxsi7inqtwsvwq3qasvgr` | 0 | 0 | 0 | 0 | ✅ |
| Verificacion publica de insignia | estudiante | oscuro | `/badges/verify/imflxsi7inqtwsvwq3qasvgr` | 0 | 0 | 0 | 0 | ✅ |
| Mis sesiones | estudiante | claro | `/account/sessions` | 0 | 0 | 0 | 0 | ✅ |
| Mis sesiones | estudiante | oscuro | `/account/sessions` | 0 | 0 | 0 | 0 | ✅ |
| Mis cursos (autoria) | profesor | claro | `/teach` | 0 | 0 | 0 | 0 | ✅ |
| Mis cursos (autoria) | profesor | oscuro | `/teach` | 0 | 0 | 0 | 0 | ✅ |
| Nuevo curso | profesor | claro | `/teach/new` | 0 | 0 | 0 | 0 | ✅ |
| Nuevo curso | profesor | oscuro | `/teach/new` | 0 | 0 | 0 | 0 | ✅ |
| Editor de curso | profesor | claro | `/teach/8d5e407e-9526-4b35-8772-c724cd870972` | 0 | 0 | 0 | 0 | ✅ |
| Editor de curso | profesor | oscuro | `/teach/8d5e407e-9526-4b35-8772-c724cd870972` | 0 | 0 | 0 | 0 | ✅ |
| Usuarios y roles | admin | claro | `/admin/users` | 0 | 0 | 0 | 0 | ✅ |
| Usuarios y roles | admin | oscuro | `/admin/users` | 0 | 0 | 0 | 0 | ✅ |
| Bitacora de auditoria | admin | claro | `/admin/audit` | 0 | 0 | 0 | 0 | ✅ |
| Bitacora de auditoria | admin | oscuro | `/admin/audit` | 0 | 0 | 0 | 0 | ✅ |
| Insignias (administracion) | admin | claro | `/admin/badges` | 0 | 0 | 0 | 0 | ✅ |
| Insignias (administracion) | admin | oscuro | `/admin/badges` | 0 | 0 | 0 | 0 | ✅ |
| Todos los cursos | admin | claro | `/teach` | 0 | 0 | 0 | 0 | ✅ |
| Todos los cursos | admin | oscuro | `/teach` | 0 | 0 | 0 | 0 | ✅ |

## Detalle de incumplimientos

No se encontraron incumplimientos.
