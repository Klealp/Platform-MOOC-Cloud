import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * Route Handler (BFF): entrega la URL firmada de un asset.
 *
 * El navegador no puede llamar al backend Go directamente (no tiene el token,
 * que es HttpOnly). En su lugar pide a este endpoint same-origin; el servidor
 * adjunta el Bearer desde la cookie y reenvia. La URL firmada que devuelve el
 * backend (hacia MinIO/CDN) si es consumible por el navegador.
 */
export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const res = await apiFetchRaw(`/assets/${encodeURIComponent(id)}/url`);
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
