import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * Route Handler (BFF): reenvia una senal de progreso al backend.
 *
 * Los heartbeats llegan con frecuencia desde el reproductor; un endpoint
 * same-origin ligero es mas adecuado que una Server Action para esto. El
 * cliente solo envia SENALES (open/heartbeat/complete). Si intenta mandar
 * progress_pct o completed, el backend responde 422 y aqui se propaga tal cual:
 * la autoridad sobre el progreso es del servidor.
 */
export async function POST(req: NextRequest) {
  let payload: unknown;
  try {
    payload = await req.json();
  } catch {
    return NextResponse.json(
      { error: { code: "validation_error", message: "JSON inválido." } },
      { status: 400 },
    );
  }

  const res = await apiFetchRaw("/progress/events", {
    method: "POST",
    json: payload,
  });
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
