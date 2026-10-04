import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/** BFF: inicia una carga multipart. Devuelve URLs prefirmadas por parte. */
export async function POST(req: NextRequest) {
  const payload = await req.json().catch(() => ({}));
  const res = await apiFetchRaw("/uploads", { method: "POST", json: payload });
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
