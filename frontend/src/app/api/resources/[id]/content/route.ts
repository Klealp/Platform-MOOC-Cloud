import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * BFF: lee el contenido Markdown de un recurso.
 *
 * NOTA: en el backend actual GET /resources/:id/content esta restringido a
 * profesor/administrador (autoria). Un estudiante inscrito recibira 404/403.
 * El cliente degrada con gracia mostrando un aviso. Esto senala un gap del
 * contrato: faltaria un endpoint de lectura de contenido para el estudiante.
 */
export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const res = await apiFetchRaw(`/resources/${encodeURIComponent(id)}/content`);
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
