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

export async function listAudit(): Promise<AuditEntry[]> {
  const res = await apiFetch<ListWrapper<AuditEntry>>("/admin/audit");
  return res.data ?? [];
}
