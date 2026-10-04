"use server";

import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import { apiFetch, ApiRequestError } from "@/lib/api/server-client";
import type { ProgressEventBody } from "@/lib/api/types";

export type ActionResult = { error?: string; ok?: boolean };

/** Inscribe al usuario en un curso por slug y lo lleva a su area de estudio. */
export async function enrollAction(slug: string): Promise<ActionResult> {
  let enrollmentId = "";
  try {
    const res = await apiFetch<{ id: string }>("/enrollments", {
      method: "POST",
      json: { slug },
    });
    enrollmentId = res.id;
  } catch (err) {
    if (err instanceof ApiRequestError) {
      if (err.status === 401) redirect(`/login?from=/catalog/${slug}`);
      return { error: err.message };
    }
    return { error: "No se pudo completar la inscripción." };
  }
  revalidatePath("/learn");
  redirect(`/learn/${enrollmentId}`);
}

/** Retira al estudiante del curso (conserva el progreso). */
export async function withdrawAction(enrollmentId: string): Promise<ActionResult> {
  try {
    await apiFetch(`/enrollments/${encodeURIComponent(enrollmentId)}`, {
      method: "DELETE",
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo procesar el retiro." };
  }
  revalidatePath("/learn");
  return { ok: true };
}

/**
 * Reporta una senal de progreso (open/heartbeat/complete).
 *
 * IMPORTANTE: el cliente solo envia SENALES, nunca porcentajes. El backend
 * rechaza con 422 cualquier intento de fijar progress_pct o completed. Por eso
 * este tipo (ProgressEventBody) no incluye esos campos.
 */
export async function sendProgressEvent(
  body: ProgressEventBody,
): Promise<{ ok: boolean; completed?: boolean; progressPct?: number; error?: string }> {
  try {
    const res = await apiFetch<{
      resource: { completed: boolean };
      progress: { progress_pct: number };
    }>("/progress/events", { method: "POST", json: body });
    return {
      ok: true,
      completed: res.resource?.completed,
      progressPct: res.progress?.progress_pct,
    };
  } catch (err) {
    if (err instanceof ApiRequestError) {
      return { ok: false, error: err.message };
    }
    return { ok: false, error: "Error de red al reportar progreso." };
  }
}
