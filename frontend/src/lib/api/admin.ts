import "server-only";

import { apiFetch } from "./server-client";
import type { AdminUser, AuditEntry } from "./types";

interface ListWrapper<T> {
  data: T[];
}

export async function listUsers(): Promise<AdminUser[]> {
  const res = await apiFetch<ListWrapper<AdminUser>>("/admin/users");
  return res.data ?? [];
}

/** Tope de GET /admin/audit: el backend no devuelve mas lineas por peticion. */
export const AUDIT_MAX_LIMIT = 500;

export async function listAudit(
  opts: { action?: string; limit?: number } = {},
): Promise<AuditEntry[]> {
  const sp = new URLSearchParams();
  if (opts.action) sp.set("action", opts.action);
  if (opts.limit) sp.set("limit", String(opts.limit));
  const qs = sp.toString();
  const res = await apiFetch<ListWrapper<AuditEntry>>(`/admin/audit${qs ? `?${qs}` : ""}`);
  return res.data ?? [];
}
