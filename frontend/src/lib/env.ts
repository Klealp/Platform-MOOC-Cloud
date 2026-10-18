/**
 * Configuracion de entorno — SOLO server-side.
 *
 * Ninguna de estas variables lleva el prefijo NEXT_PUBLIC_, por lo que Next.js
 * las mantiene fuera del bundle del navegador. El cliente nunca conoce la URL
 * del backend ni el nombre de la cookie de sesion.
 */

function required(name: string, fallback?: string): string {
  const value = process.env[name] ?? fallback;
  if (value === undefined || value === "") {
    throw new Error(
      `Falta la variable de entorno ${name}. Copia .env.example a .env.local.`,
    );
  }
  return value;
}

/**
 * Decide si la cookie de sesion lleva el flag Secure (solo HTTPS).
 *
 * Por defecto sigue a NODE_ENV (production => Secure). Pero se puede forzar con
 * SESSION_COOKIE_SECURE ("true"/"false") sin tocar NODE_ENV. Esto es necesario
 * en despliegues donde el build es de produccion (imagen standalone) pero se
 * accede por http://localhost sin TLS: ahi Secure impediria guardar la cookie
 * y el login fallaria.
 */
function cookieSecure(): boolean {
  const override = process.env.SESSION_COOKIE_SECURE;
  if (override === "true") return true;
  if (override === "false") return false;
  return process.env.NODE_ENV === "production";
}

export const env = {
  /** URL base del backend Go, p. ej. http://localhost:8080/api/v1 */
  apiBaseUrl: required("API_BASE_URL", "http://localhost:8080/api/v1"),
  /** Nombre de la cookie HttpOnly que guarda el token opaco de sesion. */
  sessionCookieName: required("SESSION_COOKIE_NAME", "mooc_session"),
  /** true en produccion: activa Secure en la cookie (solo HTTPS). */
  isProd: process.env.NODE_ENV === "production",
  /** true si la cookie de sesion debe marcarse Secure (solo HTTPS). */
  cookieSecure: cookieSecure(),
};
