import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * Route Handler (BFF): devuelve la playlist HLS (.m3u8) con segmentos firmados.
 *
 * El backend reescribe el .m3u8 para que cada segmento lleve su propia URL
 * prefirmada. hls.js (en el navegador) carga esta playlist desde el mismo
 * origen; el servidor adjunta el Bearer desde la cookie HttpOnly.
 */
export async function GET(
  _req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const res = await apiFetchRaw(`/assets/${encodeURIComponent(id)}/playlist`);
  const body = await res.text();
  return new NextResponse(body, {
    status: res.status,
    headers: {
      "Content-Type":
        res.headers.get("Content-Type") ?? "application/vnd.apple.mpegurl",
      "Cache-Control": "no-store",
    },
  });
}
