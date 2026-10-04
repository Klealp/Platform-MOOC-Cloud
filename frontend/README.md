# Frontend — Plataforma MOOC

Frontend en **Next.js 15 (App Router) + TypeScript + Tailwind v4**, con una capa
**BFF (Backend For Frontend)** que se sienta entre el navegador y la API Go.

Referencia visual estilo **Platzi** con una paleta **brutalista**: bordes negros
gruesos, sombras duras sin desenfoque, colores saturados y tipografía pesada.

## Puesta en marcha

```bash
cp .env.example .env.local      # ajusta API_BASE_URL si hace falta
npm install
npm run gen:api                 # regenera tipos desde ../internal/openapi/openapi.yaml
npm run dev                     # http://localhost:3000
```

Requiere el backend Go en marcha (por defecto `http://localhost:8080/api/v1`).
Levántalo desde la raíz del repo con `docker compose up --build`.

## Decisiones de arquitectura (requisitos de seguridad)

### 1. El token de sesión NO vive en localStorage

El backend devuelve un `access_token` opaco en el body del login. En lugar de
guardarlo en `localStorage`/`sessionStorage` (accesibles por JS y por tanto
vulnerables a XSS), el BFF lo almacena en una **cookie `HttpOnly` + `SameSite=Lax`**
(`Secure` en producción). El JavaScript del navegador **no puede leer** esa
cookie. El flujo es:

```
navegador  ──(cookie HttpOnly, same-origin)──▶  Next.js (BFF)  ──(Bearer token)──▶  API Go
```

El navegador nunca ve el token ni la URL del backend (`API_BASE_URL` es
server-side, sin prefijo `NEXT_PUBLIC_`).

- `src/lib/api/server-client.ts` — cliente server-only que lee la cookie y
  adjunta `Authorization: Bearer`.
- `src/lib/auth/session.ts` — set/clear de la cookie, `getCurrentUser()`.
- `src/lib/auth/actions.ts` — Server Actions de login/logout/registro.

### 2. Las comprobaciones de rol se hacen en el servidor

La autorización nunca depende del cliente. Las páginas protegidas llaman a
`requireUser` / `requireRole` / `requireAdmin` (`src/lib/auth/guards.ts`), que se
ejecutan en Server Components y consultan `/auth/me` en el backend. Un no-admin
recibe un `redirect` a `/forbidden` antes de que se renderice nada. Las Server
Actions de admin revalidan el rol además de que el backend Go lo exige
(`requireRole("admin")`): defensa en profundidad.

`src/middleware.ts` añade una primera barrera edge (presencia de cookie).

### 3. Sin CORS en el backend

Como el navegador solo habla con Next.js (same-origin) y el BFF reenvía al
backend server-side, no hace falta configurar CORS en Go.

## Estructura

- `src/app/` — rutas. Público/estudiante (`/catalog`, `/learn`), profesor
  (`/teach`), admin (`/admin`), auth (`/login`, `/register`).
- `src/app/api/` — Route Handlers del BFF (proxys que adjuntan el Bearer):
  progreso (heartbeats), playlist HLS, URLs de assets, intentos de quiz,
  uploads reanudables, autosave de contenido.
- `src/components/` — UI brutalista + componentes por dominio.
- `src/lib/api/` — cliente, tipos de dominio y tipos generados (`schema.d.ts`).

## Markdown y XSS

El backend guarda Markdown crudo y delega el saneo al frontend. `src/lib/markdown.ts`
convierte con `marked` y **sanea con DOMPurify** antes de renderizar.

## Funcionalidades

- **Estudiante:** catálogo con filtros, ficha de curso, inscripción, reproductor
  (video HLS + Markdown), barra de progreso por heartbeats, quizzes, "mis insignias".
- **Profesor:** editor de cursos con versionado, árbol módulos/unidades/recursos,
  autosave con aviso de conflicto (409), subida de media reanudable, constructor
  de quizzes.
- **Admin:** gestión de usuarios/roles, bitácora de auditoría, revocar insignias.

## Despliegue en GCP

En producción (Entrega 2) el frontend corre como un contenedor más en el
**Web Server**, junto a `api` y `caddy`:

- **Imagen:** `Dockerfile.web` (raíz del repo) — build multi-stage con salida
  `standalone` de Next.js, imagen final mínima, usuario no-root.
- **Enrutado:** Caddy sirve todo en un único dominio y reparte por ruta:
  `/api/v1/*`, `/openapi.yaml`, `/docs`, `/healthz`, `/readyz` → API Go;
  **todo lo demás** (páginas y Route Handlers `/api/*` del BFF) → frontend.
  Compartir origen es lo que permite que la cookie HttpOnly funcione sin CORS.
- **URL del backend:** `API_BASE_URL=http://api:8080/api/v1` — el nombre del
  servicio en la red interna de Docker. El BFF habla con la API por la red
  privada del host, sin pasar por Caddy ni salir a Internet.
- **Cookie Secure:** con `NODE_ENV=production` y HTTPS real (Caddy), la cookie
  de sesión se marca `Secure`.
- **Config:** plantilla en `deploy/gcp/web/web-frontend.env.example`;
  `provision.sh config` deja el resultado en `/etc/mooc/web-frontend.env`.
- **Subida directa / HLS:** el navegador sube las partes y lee los segmentos
  HLS directamente contra Cloud Storage (otro origen) usando las URLs firmadas
  que emite la API; esto requiere el CORS del bucket (`deploy/gcp/cors.json`,
  ya configurado para `GET/HEAD/PUT`).

El `docker compose -f deploy/gcp/web/docker-compose.yml up -d --build` levanta
`api`, `web` y `caddy` juntos.

## Gap conocido del backend

`GET /resources/:id/content` está restringido a profesor/admin, de modo que un
**estudiante** no puede leer por API el Markdown de un recurso `rich_text`
(el outline solo trae `has_content`, no el contenido). El frontend lo maneja con
gracia (muestra un aviso). Para una experiencia completa del estudiante haría
falta un endpoint de lectura de contenido para inscritos.
