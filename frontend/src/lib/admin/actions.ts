"use server";

import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import { apiFetch, ApiRequestError } from "@/lib/api/server-client";
import { requireAdmin } from "@/lib/auth/guards";
import type { Role } from "@/lib/api/types";

export type AdminResult = { error?: string; ok?: boolean };

/**
 * Server Actions de administracion.
 *
 * CLAVE DE SEGURIDAD: cada accion vuelve a comprobar el rol admin en el
 * SERVIDOR con requireAdmin() antes de llamar al backend. Nunca se confia en
 * que la UI haya ocultado el boton. Ademas el backend Go exige rol admin
 * (requireRole("admin")): defensa en profundidad. Un estudiante que invocara
 * estas acciones directamente seria rechazado dos veces.
 */

export async function createUserAction(
  _prev: AdminResult,
  formData: FormData,
): Promise<AdminResult> {
  await requireAdmin();

  const email = String(formData.get("email") ?? "").trim();
  const full_name = String(formData.get("full_name") ?? "").trim();
  const password = String(formData.get("password") ?? "");
  const role = String(formData.get("role") ?? "teacher") as Role;

  if (!email || !full_name || !password) {
    return { error: "Completa todos los campos." };
  }
  if (password.length < 10) {
    return { error: "La contraseña debe tener al menos 10 caracteres." };
  }

  try {
    await apiFetch("/admin/users", {
      method: "POST",
      json: { email, full_name, password, role },
    });
  } catch (err) {
    if (err instanceof ApiRequestError) {
      if (err.status === 409) return { error: "Ya existe un usuario con ese correo." };
      return { error: err.message };
    }
    return { error: "No se pudo crear el usuario." };
  }
  revalidatePath("/admin/users");
  return { ok: true };
}

export async function updateUserAction(
  userId: string,
  data: { role?: Role; status?: string },
): Promise<AdminResult> {
  await requireAdmin();
  try {
    await apiFetch(`/admin/users/${encodeURIComponent(userId)}`, {
      method: "PATCH",
      json: data,
    });
  } catch (err) {
    if (err instanceof ApiRequestError) {
      // 409 = protección del último administrador activo.
      return { error: err.message };
    }
    return { error: "No se pudo actualizar el usuario." };
  }
  revalidatePath("/admin/users");
  return { ok: true };
}

export async function revokeUserSessionsAction(userId: string): Promise<AdminResult> {
  await requireAdmin();
  try {
    await apiFetch(`/admin/users/${encodeURIComponent(userId)}/revoke-sessions`, {
      method: "POST",
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudieron revocar las sesiones." };
  }
  return { ok: true };
}

export async function revokeBadgeAction(badgeId: string): Promise<AdminResult> {
  await requireAdmin();
  try {
    await apiFetch(`/admin/badges/${encodeURIComponent(badgeId)}/revoke`, {
      method: "POST",
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo revocar la insignia." };
  }
  revalidatePath("/admin/badges");
  return { ok: true };
}
