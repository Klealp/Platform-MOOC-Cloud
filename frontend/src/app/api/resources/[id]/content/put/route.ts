import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * BFF: autosave de contenido con deteccion de conflicto.
 *
 * Reenvia expected_revision; si otra pestaña/editor guardo mientras tanto, el
 * backend responde 409 con la revision actual y aqui se propaga para que el
 * editor avise del conflicto en lugar de pisar el trabajo ajeno.
 *
 * (PUT no se puede exponer en la misma carpeta que el GET de content porque el
 * backend usa el mismo path para ambos verbos; aqui lo separamos en /put y
 * traducimos a PUT /resources/:id/content.)
 */
export async function POST(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const payload = await req.json().catch(() => ({}));
  const res = await apiFetchRaw(`/resources/${encodeURIComponent(id)}/content`, {
    method: "PUT",
    json: payload,
  });
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
