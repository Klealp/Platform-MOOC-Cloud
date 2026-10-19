import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * BFF: lee el contenido Markdown de un recurso (autoria, profesor/admin).
 *
 * El estudiante lee por /api/enrollments/:id/resources/:resourceId/content.
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
