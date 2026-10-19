import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * BFF: lee el Markdown de un recurso rich_text como estudiante.
 *
 * Proxy de GET /enrollments/:id/resources/:resourceId/content. El backend
 * valida inscripcion activa, curso publicado y version vigente; cualquier
 * otro caso responde 404.
 */
export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ id: string; resourceId: string }> },
) {
  const { id, resourceId } = await params;
  const res = await apiFetchRaw(
    `/enrollments/${encodeURIComponent(id)}/resources/${encodeURIComponent(resourceId)}/content`,
  );
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
