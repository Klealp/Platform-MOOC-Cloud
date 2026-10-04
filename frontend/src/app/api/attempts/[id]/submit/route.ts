import { NextResponse, type NextRequest } from "next/server";
import { randomUUID } from "node:crypto";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * BFF: envia (califica) un intento.
 *
 * El backend EXIGE la cabecera Idempotency-Key en este endpoint: es la
 * operacion mas sensible a duplicados. La generamos en el servidor; el cliente
 * puede pasar su propia llave en el body (idempotency_key) para que un
 * reintento desde el navegador reutilice la misma y no recalifique.
 */
export async function POST(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const payload = await req.json().catch(() => ({}));
  const key =
    typeof payload?.idempotency_key === "string" && payload.idempotency_key
      ? payload.idempotency_key
      : randomUUID();

  // No reenviamos idempotency_key en el cuerpo (el backend espera {answers}).
  const { idempotency_key: _omit, ...rest } = payload ?? {};
  void _omit;

  const res = await apiFetchRaw(`/attempts/${encodeURIComponent(id)}/submit`, {
    method: "POST",
    json: rest,
    extraHeaders: { "Idempotency-Key": key },
  });
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: { "Content-Type": "application/json" },
  });
}
