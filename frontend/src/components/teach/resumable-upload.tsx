"use client";

import { useEffect, useRef, useState } from "react";
import { CheckCircle2, Loader2, UploadCloud, XCircle } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { ProgressBar } from "@/components/ui/progress-bar";
import { Tag } from "@/components/ui/badge";

/**
 * Subida multipart REANUDABLE directa al almacenamiento, con seguimiento del
 * procesamiento posterior.
 *
 * Flujo (el archivo nunca pasa por el BFF: solo las URLs lo hacen):
 *  1. POST /api/uploads  -> el backend abre la carga y firma una URL por parte.
 *  2. Por cada parte: PUT directo a su URL prefirmada (hacia MinIO/S3). Se
 *     guarda cada parte completada; si la pestaña se cierra, al reabrir se
 *     consulta GET /api/uploads/:id y se reanudan SOLO las partes faltantes.
 *  3. POST /api/uploads/:id/complete -> ensambla y encola el escaneo.
 *  4. Se consulta GET /api/assets/:id en intervalos para mostrar el avance del
 *     procesamiento (scanning -> processing -> ready / failed / infected).
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

interface AssetInfo {
  status: string;
  hls_available?: boolean;
  failure_reason?: string;
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

/** Texto legible por estado de procesamiento del asset. */
function processingLabel(status: string): { text: string; done: boolean; error: boolean } {
  switch (status) {
    case "uploaded":
    case "scanning":
      return { text: "Verificando (análisis de seguridad)…", done: false, error: false };
    case "processing":
      return { text: "Procesando (transcodificación)…", done: false, error: false };
    case "ready":
      return { text: "Disponible", done: true, error: false };
    case "infected":
      return { text: "Rechazado por el análisis de seguridad", done: false, error: true };
    case "failed":
      return { text: "Error de procesamiento", done: false, error: true };
    default:
      return { text: `Estado: ${status}`, done: false, error: false };
  }
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
  const [assetStatus, setAssetStatus] = useState<string | null>(null);
  const [uploadId, setUploadId] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const abortRef = useRef(false);

  // Sondeo del estado de procesamiento una vez que el archivo fue recibido.
  useEffect(() => {
    if (!doneAsset) return;
    if (assetStatus && processingLabel(assetStatus).done) return;
    if (assetStatus && processingLabel(assetStatus).error) return;

    let cancelled = false;
    const timer = setInterval(async () => {
      try {
        const res = await fetch(`/api/assets/${doneAsset}`);
        if (!res.ok) return;
        const info: AssetInfo = await res.json();
        if (cancelled) return;
        setAssetStatus(info.status);
        const label = processingLabel(info.status);
        if (label.done || label.error) clearInterval(timer);
      } catch {
        // Reintentamos en el siguiente tick.
      }
    }, 3000);

    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [doneAsset, assetStatus]);

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
      if (abortRef.current) throw new Error("__aborted__");
      const startByte = (p.part_number - 1) * partSize;
      const chunk = file.slice(startByte, startByte + partSize);
      await putPart(p.url, chunk);
      completed += 1;
      setProgress(Math.round((completed / totalParts) * 100));
    }
  }

  async function cancelUpload() {
    abortRef.current = true;
    if (uploadId) {
      try {
        await fetch(`/api/uploads/${uploadId}`, { method: "DELETE" });
      } catch {
        // Si falla la cancelación remota, la carga expira sola a las 24h.
      }
    }
    resetState();
    setStatus("Carga cancelada.");
  }

  function resetState() {
    setBusy(false);
    setProgress(0);
    setUploadId(null);
    abortRef.current = false;
  }

  async function start(file: File) {
    setError(null);
    setBusy(true);
    setDoneAsset(null);
    setAssetStatus(null);
    setProgress(0);
    abortRef.current = false;
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
        setError(
          (init as unknown as { error?: { message?: string } })?.error?.message ??
            "No se pudo iniciar la carga.",
        );
        return;
      }
      setUploadId(init.upload_id);

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
      setStatus("Archivo recibido. Procesando…");
      setDoneAsset(init.asset_id);
      setAssetStatus(done?.status ?? "uploaded");
      // Asociamos el asset al recurso aunque aún se esté procesando: el estado
      // se refleja luego en la vista (y el estudiante no lo verá hasta `ready`).
      onComplete(init.asset_id);
    } catch (e) {
      if (e instanceof Error && e.message === "__aborted__") {
        setStatus("Carga cancelada.");
        return;
      }
      setError(e instanceof Error ? e.message : "Error inesperado en la carga.");
    } finally {
      resetState();
    }
  }

  // Vista tras recibir el archivo: seguimiento del procesamiento.
  if (doneAsset) {
    const label = processingLabel(assetStatus ?? "uploaded");
    return (
      <Alert
        variant={label.error ? "error" : label.done ? "success" : "info"}
        title="Archivo asociado"
        className="items-center"
      >
        <span className="flex flex-wrap items-center gap-2">
          {label.done ? (
            <CheckCircle2 className="h-4 w-4 text-ok" />
          ) : label.error ? (
            <XCircle className="h-4 w-4" />
          ) : (
            <Loader2 className="h-4 w-4 animate-spin" />
          )}
          {label.text}
          {assetStatus && <Tag tone="neutral">{assetStatus}</Tag>}
        </span>
      </Alert>
    );
  }

  return (
    <div className="space-y-3">
      {error && <Alert variant="error">{error}</Alert>}
      <input
        ref={inputRef}
        type="file"
        className="hidden"
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) void start(file);
        }}
      />
      {!busy ? (
        <Button
          type="button"
          variant="secondary"
          onClick={() => inputRef.current?.click()}
        >
          <UploadCloud className="h-5 w-5" /> Seleccionar archivo
        </Button>
      ) : (
        <div className="space-y-2">
          <ProgressBar value={progress} />
          <div className="flex items-center justify-between gap-2">
            <p className="flex items-center gap-1 text-xs font-bold">
              {progress === 100 && <CheckCircle2 className="h-3 w-3 text-ok" />}
              {status}
            </p>
            <Button type="button" size="sm" variant="outline" onClick={cancelUpload}>
              <XCircle className="h-4 w-4" /> Cancelar
            </Button>
          </div>
        </div>
      )}
      {!busy && status && <p className="text-xs font-bold opacity-70">{status}</p>}
    </div>
  );
}
