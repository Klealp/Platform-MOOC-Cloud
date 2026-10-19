import { NextResponse, type NextRequest } from "next/server";

import { apiFetchRaw } from "@/lib/api/server-client";

/**
 * Route Handler (BFF): devuelve la playlist HLS (.m3u8) con segmentos firmados.
 *
 * El backend reescribe el .m3u8 para que cada segmento lleve su propia URL
 * prefirmada. hls.js (en el navegador) carga esta playlist desde el mismo
 * origen; el servidor adjunta el Bearer desde la cookie HttpOnly.
 *
 * La lista maestra (index.m3u8) enumera sub-listas por calidad. El backend las
 * apunta a su propio endpoint (`.../api/v1/assets/:id/playlist?file=X`), pero
 * el navegador no tiene el Bearer y esa peticion directa responderia 401. Por
 * eso aqui se reenvia `?file=` y se reescriben esas lineas hacia este BFF.
 * Los segmentos (.ts) no se tocan: van firmados directo al almacenamiento.
 */
const NESTED_PLAYLIST = /^\S*\/api\/v1\/assets\/([^/?\s]+)\/playlist\?file=(\S+)$/;

export async function GET(
  req: NextRequest,
  { params }: { params: Promise<{ id: string }> },
) {
  const { id } = await params;
  const file = req.nextUrl.searchParams.get("file");
  const query = file ? `?file=${encodeURIComponent(file)}` : "";
  const res = await apiFetchRaw(`/assets/${encodeURIComponent(id)}/playlist${query}`);
  let body = await res.text();

  if (res.ok) {
    body = body
      .split("\n")
      .map((line) => {
        const m = line.trim().match(NESTED_PLAYLIST);
        return m ? `/api/assets/${m[1]}/playlist?file=${m[2]}` : line;
      })
      .join("\n");
  }

  return new NextResponse(body, {
    status: res.status,
    headers: {
      "Content-Type": res.ok
        ? "application/vnd.apple.mpegurl"
        : res.headers.get("Content-Type") ?? "application/json",
      "Cache-Control": "no-store",
    },
  });
}
