"use server";

import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

import { apiFetch, ApiRequestError } from "@/lib/api/server-client";
import type { LoginResponse } from "@/lib/api/types";
import { clearSessionCookie, setSessionCookie } from "./session";

/**
 * Server Actions de autenticacion.
 *
 * El login llama al backend, recibe el access_token EN EL SERVIDOR y lo guarda
 * en la cookie HttpOnly. El token jamas llega al JavaScript del navegador: no
 * se devuelve al cliente ni se escribe en ningun almacenamiento accesible por
 * scripts. Esto cumple el requisito "el token no vive en localStorage".
 */

export type FormState = { error?: string; ok?: boolean };

export async function loginAction(
  _prev: FormState,
  formData: FormData,
): Promise<FormState> {
  const email = String(formData.get("email") ?? "").trim();
  const password = String(formData.get("password") ?? "");
  const from = String(formData.get("from") ?? "");

  if (!email || !password) {
    return { error: "Ingresa correo y contraseña." };
  }

  let res: LoginResponse;
  try {
    res = await apiFetch<LoginResponse>("/auth/login", {
      method: "POST",
      anonymous: true,
      json: { email, password },
    });
  } catch (err) {
    if (err instanceof ApiRequestError) {
      if (err.status === 401) return { error: "Credenciales inválidas." };
      if (err.status === 403) return { error: err.message };
      return { error: err.message };
    }
    return { error: "No se pudo conectar con el servidor." };
  }

  await setSessionCookie(res.access_token, res.expires_in);

  const target = from && from.startsWith("/") ? from : "/dashboard";
  redirect(target);
}

export async function logoutAction(): Promise<void> {
  try {
    await apiFetch("/auth/logout", { method: "POST" });
  } catch {
    // Aunque el backend falle, limpiamos la cookie local para cerrar sesion.
  }
  await clearSessionCookie();
  revalidatePath("/", "layout");
  redirect("/login");
}

export async function registerAction(
  _prev: FormState,
  formData: FormData,
): Promise<FormState> {
  const email = String(formData.get("email") ?? "").trim();
  const password = String(formData.get("password") ?? "");
  const full_name = String(formData.get("full_name") ?? "").trim();

  if (!email || !password || !full_name) {
    return { error: "Completa todos los campos." };
  }
  if (password.length < 10) {
    return { error: "La contraseña debe tener al menos 10 caracteres." };
  }

  try {
    await apiFetch("/auth/register", {
      method: "POST",
      anonymous: true,
      json: { email, password, full_name },
    });
  } catch (err) {
    if (err instanceof ApiRequestError) return { error: err.message };
    return { error: "No se pudo completar el registro." };
  }

  return { ok: true };
}
