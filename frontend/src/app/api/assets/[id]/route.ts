import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * Route Handler (BFF): estado/metadatos de un asset (GET /assets/:id).
 *
 * Lo usa el profesor para seguir el procesamiento de un archivo recién subido
 * (scanning -> processing -> ready / failed / infected) sin exponer el token,
 * que es HttpOnly. El servidor adjunta el Bearer desde la cookie y reenvía.
 */
export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const res = await apiFetchRaw(`/assets/${encodeURIComponent(id)}`);
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
