import "server-only";

import { apiFetch } from "./server-client";
import type { PublicBadge } from "./types";

/**
 * Verificación pública de una insignia por su código (GET /public/badges/:code).
 * Endpoint anónimo: no requiere sesión y no expone el correo del titular.
 */
export function verifyBadge(code: string) {
  return apiFetch<PublicBadge>(
    `/public/badges/${encodeURIComponent(code)}`,
    { anonymous: true },
  );
}
