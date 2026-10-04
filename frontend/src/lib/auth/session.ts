import "server-only";

import { cookies } from "next/headers";
import { cache } from "react";

import { env } from "@/lib/env";
import { apiFetch, ApiRequestError } from "@/lib/api/server-client";
import type { Role, SessionUser } from "@/lib/api/types";

/**
 * Gestion de sesion server-side.
 *
 * El token opaco del backend se guarda en una cookie HttpOnly + SameSite=Lax
 * (y Secure en produccion). HttpOnly impide que cualquier script del navegador
 * lo lea, de modo que un XSS no puede robar la sesion. SameSite=Lax mitiga CSRF
 * para las navegaciones de terceros.
 */

const SESSION_MAX_AGE = 60 * 60 * 24; // 24h, acorde a SESSION_TTL_HOURS del backend.

export async function setSessionCookie(token: string, maxAgeSecs?: number) {
  const store = await cookies();
  store.set(env.sessionCookieName, token, {
    httpOnly: true,
    secure: env.isProd,
    sameSite: "lax",
    path: "/",
    maxAge: maxAgeSecs ?? SESSION_MAX_AGE,
  });
}

export async function clearSessionCookie() {
  const store = await cookies();
  store.delete(env.sessionCookieName);
}

export async function hasSessionCookie(): Promise<boolean> {
  const store = await cookies();
  return store.has(env.sessionCookieName);
}

/**
 * Devuelve el usuario autenticado consultando `/auth/me` en el backend, o null
 * si no hay sesion valida. Es la UNICA fuente de verdad de identidad: el
 * frontend nunca decodifica ni confia en datos del cliente.
 *
 * Envuelto en cache() de React: dentro de un mismo render/request solo se
 * llama una vez al backend aunque varios componentes lo pidan.
 */
export const getCurrentUser = cache(async (): Promise<SessionUser | null> => {
  if (!(await hasSessionCookie())) return null;
  try {
    return await apiFetch<SessionUser>("/auth/me");
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 401) {
      return null;
    }
    throw err;
  }
});

export function hasRole(user: SessionUser | null, ...roles: Role[]): boolean {
  return user != null && roles.includes(user.role);
}
