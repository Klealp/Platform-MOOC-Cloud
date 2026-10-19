"use client";

import { useEffect, useState } from "react";
import { Download } from "lucide-react";

import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";

/**
 * Muestra en linea un archivo (imagen o PDF) y ofrece su descarga.
 *
 * /api/assets/:id/url devuelve un JSON con `download_url` (URL prefirmada
 * hacia el almacenamiento); el archivo nunca pasa por la API ni por el BFF.
 *
 * - Imagen: <img> sobre la URL firmada (el navegador ignora el
 *   Content-Disposition en etiquetas <img>).
 * - PDF: el backend firma con `Content-Disposition: attachment`, asi que un
 *   <iframe> sobre la URL lo descargaria en vez de mostrarlo. Se baja el PDF
 *   con fetch (CORS GET del bucket) y se muestra desde un blob local.
 * - Descarga: se pide una URL fresca al hacer clic, porque la firma expira.
 */

type Preview = "image" | "pdf" | "none";

async function fetchDeliveryURL(assetId: string): Promise<string> {
  const res = await fetch(`/api/assets/${assetId}/url`, { cache: "no-store" });
  const data = await res.json().catch(() => null);
  if (!res.ok || !data?.download_url) {
    throw new Error(data?.error?.message ?? "No se pudo obtener el archivo.");
  }
  return data.download_url as string;
}

export function AssetFileViewer({
  assetId,
  preview,
  title,
  allowDownload,
}: {
  assetId: string;
  preview: Preview;
  title: string;
  allowDownload: boolean;
}) {
  const [src, setSrc] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [downloading, setDownloading] = useState(false);

  useEffect(() => {
    if (preview === "none") return;
    let active = true;
    let blobUrl: string | null = null;
    setSrc(null);
    setError(null);

    (async () => {
      const url = await fetchDeliveryURL(assetId);
      if (preview === "image") {
        if (active) setSrc(url);
        return;
      }
      const res = await fetch(url);
      if (!res.ok) throw new Error("No se pudo descargar el PDF.");
      const blob = new Blob([await res.arrayBuffer()], { type: "application/pdf" });
      blobUrl = URL.createObjectURL(blob);
      if (active) setSrc(blobUrl);
    })().catch((e: Error) => {
      if (active) setError(e.message);
    });

    return () => {
      active = false;
      if (blobUrl) URL.revokeObjectURL(blobUrl);
    };
  }, [assetId, preview]);

  async function download() {
    setDownloading(true);
    try {
      // La URL firmada lleva Content-Disposition: attachment, asi que navegar
      // a ella descarga el archivo sin salir de la pagina.
      window.location.assign(await fetchDeliveryURL(assetId));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setDownloading(false);
    }
  }

  return (
    <div className="space-y-4">
      {error && <Alert variant="warning">{error}</Alert>}

      {preview !== "none" && !error && !src && (
        <div
          className="aspect-[4/3] w-full animate-pulse rounded-[var(--radius-md)] bg-[var(--color-surface-alt)]"
          aria-busy="true"
        >
          <span className="sr-only">Cargando archivo…</span>
        </div>
      )}

      {preview === "image" && src && (
        // eslint-disable-next-line @next/next/no-img-element -- URL firmada externa y temporal
        <img
          src={src}
          alt={title}
          className="mx-auto max-h-[75vh] w-auto rounded-[var(--radius-md)] border border-[var(--color-border)]"
          onError={() => setError("No se pudo cargar la imagen.")}
        />
      )}

      {preview === "pdf" && src && (
        <iframe
          src={src}
          title={title}
          className="h-[75vh] w-full rounded-[var(--radius-md)] border border-[var(--color-border)] bg-white"
        />
      )}

      {allowDownload && (
        <Button variant="secondary" onClick={download} disabled={downloading}>
          <Download className="h-5 w-5" /> {downloading ? "Preparando…" : "Descargar"}
        </Button>
      )}
    </div>
  );
}
