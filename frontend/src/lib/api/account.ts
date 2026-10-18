import "server-only";

import { apiFetch } from "./server-client";
import type { ActiveSession } from "./types";

interface ListWrapper<T> {
  data: T[];
}

/** Lista las sesiones activas del usuario (GET /auth/sessions). */
export async function listActiveSessions(): Promise<ActiveSession[]> {
  const res = await apiFetch<ListWrapper<ActiveSession>>("/auth/sessions");
  return res.data ?? [];
}
