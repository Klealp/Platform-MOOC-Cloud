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

export const env = {
  /** URL base del backend Go, p. ej. http://localhost:8080/api/v1 */
  apiBaseUrl: required("API_BASE_URL", "http://localhost:8080/api/v1"),
  /** Nombre de la cookie HttpOnly que guarda el token opaco de sesion. */
  sessionCookieName: required("SESSION_COOKIE_NAME", "mooc_session"),
  /** true en produccion: activa Secure en la cookie (solo HTTPS). */
  isProd: process.env.NODE_ENV === "production",
};
