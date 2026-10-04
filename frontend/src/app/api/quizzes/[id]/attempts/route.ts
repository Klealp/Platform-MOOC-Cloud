import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * BFF: inicia (o recupera) un intento de quiz.
 * El backend congela un snapshot y NUNCA devuelve las respuestas correctas.
 */
export async function POST(
  _req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const res = await apiFetchRaw(
    `/quizzes/${encodeURIComponent(id)}/attempts`,
    { method: "POST" },
  );
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
