import "server-only";

import { cookies } from "next/headers";

import { env } from "@/lib/env";
import type { ApiError } from "./types";

/**
 * Cliente del backend — SOLO server-side (marcado con "server-only").
 *
 * Punto clave de la arquitectura: el token de sesion vive en una cookie
 * HttpOnly que el JavaScript del navegador NO puede leer. Este modulo corre en
 * el servidor de Next.js, lee esa cookie y la reenvia al backend Go como
 * `Authorization: Bearer <token>`. El navegador nunca ve el token ni la URL
 * del backend. Asi se cumplen los dos requisitos:
 *   1. El token no vive en localStorage (vive en cookie HttpOnly).
 *   2. Las decisiones de acceso se toman en el servidor, no en el cliente.
 */

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly details?: unknown,
    public readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiRequestError";
  }
}

async function readSessionToken(): Promise<string | undefined> {
  const store = await cookies();
  return store.get(env.sessionCookieName)?.value;
}

interface RequestOptions extends RequestInit {
  /** Cuerpo JSON (se serializa automaticamente). */
  json?: unknown;
  /** Enviar sin la cabecera Authorization (endpoints publicos). */
  anonymous?: boolean;
  /** Cabeceras extra, p. ej. Idempotency-Key. */
  extraHeaders?: Record<string, string>;
}

/**
 * Ejecuta una peticion al backend y devuelve el cuerpo JSON tipado.
 * Lanza ApiRequestError con el codigo del backend ante respuestas >= 400.
 */
export async function apiFetch<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { json, anonymous, extraHeaders, ...init } = options;

  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (json !== undefined) {
    headers.set("Content-Type", "application/json");
  }
  if (extraHeaders) {
    for (const [k, v] of Object.entries(extraHeaders)) headers.set(k, v);
  }

  if (!anonymous) {
    const token = await readSessionToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }

  const url = `${env.apiBaseUrl}${path}`;
  const res = await fetch(url, {
    ...init,
    headers,
    body: json !== undefined ? JSON.stringify(json) : init.body,
    // El BFF nunca cachea respuestas con datos de usuario.
    cache: "no-store",
  });

  const text = await res.text();
  const payload = text ? safeJson(text) : undefined;

  if (!res.ok) {
    const err = payload as ApiError | undefined;
    throw new ApiRequestError(
      res.status,
      err?.error?.code ?? "internal_error",
      err?.error?.message ?? `Error ${res.status} del backend.`,
      err?.error?.details,
      err?.error?.request_id,
    );
  }

  return payload as T;
}

/** Igual que apiFetch pero devuelve la respuesta cruda (p. ej. playlist HLS). */
export async function apiFetchRaw(
  path: string,
  options: RequestOptions = {},
): Promise<Response> {
  const { json, anonymous, extraHeaders, ...init } = options;
  const headers = new Headers(init.headers);
  if (json !== undefined) headers.set("Content-Type", "application/json");
  if (extraHeaders) {
    for (const [k, v] of Object.entries(extraHeaders)) headers.set(k, v);
  }
  if (!anonymous) {
    const token = await readSessionToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }
  return fetch(`${env.apiBaseUrl}${path}`, {
    ...init,
    headers,
    body: json !== undefined ? JSON.stringify(json) : init.body,
    cache: "no-store",
  });
}

function safeJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}
