"use server";

import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import { apiFetch, ApiRequestError } from "@/lib/api/server-client";
import type { PublishProblem } from "@/lib/api/types";

export type AuthoringResult = { error?: string; ok?: boolean };

/** Crea un curso (y su version 1 en borrador) y navega a su editor. */
export async function createCourseAction(
  _prev: AuthoringResult,
  formData: FormData,
): Promise<AuthoringResult> {
  const title = String(formData.get("title") ?? "").trim();
  const summary = String(formData.get("summary") ?? "").trim();
  const category = String(formData.get("category") ?? "").trim();
  const level = String(formData.get("level") ?? "beginner");

  if (!title) return { error: "El título es obligatorio." };

  let courseId = "";
  try {
    const res = await apiFetch<{ id: string }>("/courses", {
      method: "POST",
      json: { title, summary, category, level },
    });
    courseId = res.id;
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo crear el curso." };
  }
  revalidatePath("/teach");
  redirect(`/teach/${courseId}`);
}

export async function updateCourseAction(
  courseId: string,
  data: { title?: string; summary?: string; category?: string; level?: string },
): Promise<AuthoringResult> {
  try {
    await apiFetch(`/courses/${encodeURIComponent(courseId)}`, {
      method: "PATCH",
      json: data,
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudieron guardar los cambios." };
  }
  revalidatePath(`/teach/${courseId}`);
  return { ok: true };
}

export async function createDraftVersionAction(
  courseId: string,
): Promise<AuthoringResult> {
  try {
    await apiFetch(`/courses/${encodeURIComponent(courseId)}/versions`, {
      method: "POST",
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo crear el borrador." };
  }
  revalidatePath(`/teach/${courseId}`);
  return { ok: true };
}

export async function createModuleAction(
  versionId: string,
  title: string,
): Promise<AuthoringResult> {
  try {
    await apiFetch(`/versions/${encodeURIComponent(versionId)}/modules`, {
      method: "POST",
      json: { title },
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo crear el módulo." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export async function createUnitAction(
  moduleId: string,
  title: string,
): Promise<AuthoringResult> {
  try {
    await apiFetch(`/modules/${encodeURIComponent(moduleId)}/units`, {
      method: "POST",
      json: { title },
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo crear la unidad." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export interface CreateResourceInput {
  type: string;
  title: string;
  required?: boolean;
  visible?: boolean;
  content_md?: string;
  external_url?: string;
  asset_id?: string;
}

export async function createResourceAction(
  unitId: string,
  input: CreateResourceInput,
): Promise<AuthoringResult> {
  try {
    await apiFetch(`/units/${encodeURIComponent(unitId)}/resources`, {
      method: "POST",
      json: input,
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo crear el recurso." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export async function deleteModuleAction(moduleId: string): Promise<AuthoringResult> {
  try {
    await apiFetch(`/modules/${encodeURIComponent(moduleId)}`, { method: "DELETE" });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo eliminar." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export async function deleteUnitAction(unitId: string): Promise<AuthoringResult> {
  try {
    await apiFetch(`/units/${encodeURIComponent(unitId)}`, { method: "DELETE" });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo eliminar." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

export async function deleteResourceAction(resourceId: string): Promise<AuthoringResult> {
  try {
    await apiFetch(`/resources/${encodeURIComponent(resourceId)}`, { method: "DELETE" });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo eliminar." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}

/** Publica una version. Devuelve la lista EXHAUSTIVA de problemas si falla. */
export async function publishVersionAction(
  versionId: string,
  config?: { passing_score?: number; required_completion?: number },
): Promise<{ ok?: boolean; error?: string; problems?: PublishProblem[] }> {
  try {
    await apiFetch(`/versions/${encodeURIComponent(versionId)}/publish`, {
      method: "POST",
      json: config ?? {},
    });
  } catch (err) {
    if (err instanceof ApiRequestError) {
      if (err.status === 422) {
        const details = err.details as { problems?: PublishProblem[] } | undefined;
        return { error: err.message, problems: details?.problems ?? [] };
      }
      return { error: err.message };
    }
    return { error: "No se pudo publicar la versión." };
  }
  revalidatePath("/teach", "layout");
  return { ok: true };
}
