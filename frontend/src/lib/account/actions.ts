"use server";

import { revalidatePath } from "next/cache";

import { apiFetch, ApiRequestError } from "@/lib/api/server-client";

export type AccountResult = { error?: string; ok?: boolean };

/**
 * Revoca una sesión activa del propio usuario (DELETE /auth/sessions/:id).
 *
 * El backend solo permite revocar sesiones del usuario autenticado (aísla por
 * user_id). Revocar la sesión actual cierra la sesión en uso; el guard
 * server-side redirigirá a /login en la siguiente navegación.
 */
export async function revokeSessionAction(
  sessionId: string,
): Promise<AccountResult> {
  try {
    await apiFetch(`/auth/sessions/${encodeURIComponent(sessionId)}`, {
      method: "DELETE",
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo cerrar la sesión." };
  }
  revalidatePath("/account/sessions");
  return { ok: true };
}
