import "server-only";

import { apiFetch } from "./server-client";
import type { CatalogDetail, CatalogListResponse } from "./types";

export interface CatalogFilters {
  q?: string;
  category?: string;
  level?: string;
  language?: string;
  tag?: string;
  cursor?: string;
  limit?: number;
}

function toQuery(filters: CatalogFilters): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined && value !== "" && value !== null) {
      params.set(key, String(value));
    }
  }
  const qs = params.toString();
  return qs ? `?${qs}` : "";
}

/** Catalogo publico (no requiere sesion). */
export function listCatalog(filters: CatalogFilters = {}) {
  return apiFetch<CatalogListResponse>(`/catalog${toQuery(filters)}`, {
    anonymous: true,
  });
}

/** Ficha publica de un curso por slug. */
export function getCatalogDetail(slug: string) {
  return apiFetch<CatalogDetail>(`/catalog/${encodeURIComponent(slug)}`, {
    anonymous: true,
  });
}
