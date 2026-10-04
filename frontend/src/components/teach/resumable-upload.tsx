"use client";

import { useRef, useState } from "react";
import { CheckCircle2, UploadCloud } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { ProgressBar } from "@/components/ui/progress-bar";

/**
 * Subida multipart REANUDABLE directa al almacenamiento.
 *
 * Flujo (el archivo nunca pasa por el BFF: solo las URLs lo hacen):
 *  1. POST /api/uploads  -> el backend abre la carga y firma una URL por parte.
 *  2. Por cada parte: PUT directo a su URL prefirmada (hacia MinIO/S3). Se
 *     guarda cada parte completada; si la pestaña se cierra, al reabrir se
 *     consulta GET /api/uploads/:id y se reanudan SOLO las partes faltantes.
 *  3. POST /api/uploads/:id/complete -> ensambla y encola el escaneo.
 *
 * Devuelve el asset_id al terminar (via onComplete) para asociarlo al recurso.
 */

interface PartURL {
  part_number: number;
  url: string;
}
interface InitResponse {
  upload_id: string;
  asset_id: string;
  part_size: number;
  total_parts: number;
  parts: PartURL[];
}

const KIND_BY_PREFIX: Record<string, string> = {
  video: "video",
  audio: "audio",
  image: "image",
  application: "pdf",
};

function kindFor(file: File): string {
  const prefix = file.type.split("/")[0];
  if (file.type === "application/pdf") return "pdf";
  return KIND_BY_PREFIX[prefix] ?? "file";
}

export function ResumableUpload({
  onComplete,
}: {
  onComplete: (assetId: string) => void;
}) {
  const [status, setStatus] = useState<string>("");
  const [progress, setProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [doneAsset, setDoneAsset] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  async function putPart(url: string, blob: Blob): Promise<void> {
    const res = await fetch(url, { method: "PUT", body: blob });
    if (!res.ok) throw new Error(`Falló la subida de una parte (${res.status}).`);
  }

  async function uploadParts(
    file: File,
    partSize: number,
    parts: PartURL[],
    totalParts: number,
    alreadyDone: number,
  ) {
    let completed = alreadyDone;
    for (const p of parts) {
      const startByte = (p.part_number - 1) * partSize;
      const chunk = file.slice(startByte, startByte + partSize);
      await putPart(p.url, chunk);
      completed += 1;
      setProgress(Math.round((completed / totalParts) * 100));
    }
  }

  async function start(file: File) {
    setError(null);
    setBusy(true);
    setDoneAsset(null);
    setProgress(0);
    try {
      // 1. Iniciar
      setStatus("Iniciando carga…");
      const initRes = await fetch("/api/uploads", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          original_name: file.name,
          size_bytes: file.size,
          declared_mime: file.type,
          kind: kindFor(file),
        }),
      });
      const init: InitResponse = await initRes.json();
      if (!initRes.ok) {
        setError((init as unknown as { error?: { message?: string } })?.error?.message ?? "No se pudo iniciar la carga.");
        return;
      }

      // 2. Subir partes
      setStatus("Subiendo partes…");
      await uploadParts(file, init.part_size, init.parts, init.total_parts, 0);

      // 3. Completar (reconsultando faltantes como seguro de reanudacion)
      setStatus("Verificando partes…");
      const statusRes = await fetch(`/api/uploads/${init.upload_id}`);
      const st = await statusRes.json();
      if (statusRes.ok && st.status === "in_progress" && st.missing_parts?.length) {
        await uploadParts(
          file,
          init.part_size,
          st.parts,
          init.total_parts,
          init.total_parts - st.missing_parts.length,
        );
      }

      setStatus("Finalizando…");
      const completeRes = await fetch(`/api/uploads/${init.upload_id}/complete`, {
        method: "POST",
      });
      const done = await completeRes.json();
      if (!completeRes.ok) {
        setError(done?.error?.message ?? "No se pudo completar la carga.");
        return;
      }

      setProgress(100);
      setStatus("Archivo recibido. Se está verificando/procesando.");
      setDoneAsset(init.asset_id);
      onComplete(init.asset_id);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Error inesperado en la carga.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-3">
      {error && <Alert variant="error">{error}</Alert>}
      {doneAsset ? (
        <Alert variant="success" title="Subido">
          Asset asociado. {status}
        </Alert>
      ) : (
        <>
          <input
            ref={inputRef}
            type="file"
            className="hidden"
            onChange={(e) => {
              const file = e.target.files?.[0];
              if (file) void start(file);
            }}
          />
          <Button
            type="button"
            variant="secondary"
            disabled={busy}
            onClick={() => inputRef.current?.click()}
          >
            <UploadCloud className="h-5 w-5" />
            {busy ? "Subiendo…" : "Seleccionar archivo"}
          </Button>
          {busy && (
            <div className="space-y-1">
              <ProgressBar value={progress} />
              <p className="flex items-center gap-1 text-xs font-bold">
                {progress === 100 && <CheckCircle2 className="h-3 w-3 text-ok" />}
                {status}
              </p>
            </div>
          )}
        </>
      )}
    </div>
  );
}
