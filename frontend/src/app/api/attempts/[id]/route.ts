import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/** BFF: consulta un intento propio. */
export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const res = await apiFetchRaw(`/attempts/${encodeURIComponent(id)}`);
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}

/** BFF: guardado parcial de respuestas. */
export async function PATCH(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const payload = await req.json().catch(() => ({}));
  const res = await apiFetchRaw(`/attempts/${encodeURIComponent(id)}`, {
    method: "PATCH",
    json: payload,
  });
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
