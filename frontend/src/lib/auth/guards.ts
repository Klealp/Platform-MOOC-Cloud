import "server-only";

import { redirect } from "next/navigation";

import { getCurrentUser, hasRole } from "./session";
import type { Role, SessionUser } from "@/lib/api/types";

/**
 * Guards de autorizacion que se ejecutan EN EL SERVIDOR (Server Components y
 * Server Actions). El cliente nunca decide si puede ver una pagina: o recibe
 * el contenido ya resuelto, o un redirect. Aunque un usuario manipulara el
 * DOM, no obtendria datos protegidos porque el fetch al backend tambien exige
 * el rol (defensa en profundidad: Next.js + backend Go).
 */

/** Exige sesion. Redirige a /login (con retorno) si no hay usuario. */
export async function requireUser(returnTo?: string): Promise<SessionUser> {
  const user = await getCurrentUser();
  if (!user) {
    const qs = returnTo ? `?from=${encodeURIComponent(returnTo)}` : "";
    redirect(`/login${qs}`);
  }
  return user;
}

/** Exige uno de los roles dados. Redirige a /login o a una pagina 403. */
export async function requireRole(
  roles: Role[],
  returnTo?: string,
): Promise<SessionUser> {
  const user = await requireUser(returnTo);
  if (!hasRole(user, ...roles)) {
    redirect("/forbidden");
  }
  return user;
}

export const requireAdmin = (returnTo?: string) =>
  requireRole(["admin"], returnTo);

export const requireTeacher = (returnTo?: string) =>
  requireRole(["teacher", "admin"], returnTo);
