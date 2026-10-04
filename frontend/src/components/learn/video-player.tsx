"use client";

import { useEffect, useRef, useState } from "react";
import Hls from "hls.js";

import { Alert } from "@/components/ui/alert";

/**
 * Reproductor de video HLS con heartbeats de progreso.
 *
 * - Carga la playlist .m3u8 desde el BFF (/api/assets/:id/playlist), que firma
 *   los segmentos. Usa hls.js donde el navegador no soporta HLS nativo.
 * - Mientras el video se reproduce, envia un "heartbeat" cada 30s con el tiempo
 *   transcurrido (delta_secs) y la posicion. NUNCA envia porcentajes: el avance
 *   lo calcula el servidor a partir de estas senales.
 * - Al abrir manda "open"; al llegar al final (o >=95%) manda "complete".
 */

const HEARTBEAT_SECS = 30;

type Signal = "open" | "heartbeat" | "complete";

export function VideoPlayer({
  assetId,
  enrollmentId,
  resourceStableId,
  onProgress,
}: {
  assetId: string;
  enrollmentId: string;
  resourceStableId: string;
  onProgress?: (completed: boolean, pct?: number) => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState<string | null>(null);
  const accumulated = useRef(0); // segundos vistos desde el ultimo envio
  const completed = useRef(false);

  async function sendSignal(signal: Signal, deltaSecs = 0) {
    const video = videoRef.current;
    const body = {
      enrollment_id: enrollmentId,
      resource_stable_id: resourceStableId,
      event_type: signal,
      position_secs: video ? Math.floor(video.currentTime) : 0,
      delta_secs: Math.min(60, Math.max(0, Math.round(deltaSecs))),
    };
    try {
      const res = await fetch("/api/progress", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      if (res.ok) {
        const data = await res.json();
        onProgress?.(data.resource?.completed ?? false, data.progress?.progress_pct);
      }
    } catch {
      // Reintentamos en el siguiente heartbeat; no interrumpimos la reproduccion.
    }
  }

  // Carga de la fuente HLS.
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const src = `/api/assets/${assetId}/playlist`;

    let hls: Hls | null = null;
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = src; // Safari / HLS nativo
    } else if (Hls.isSupported()) {
      hls = new Hls({
        xhrSetup: (xhr) => {
          xhr.withCredentials = true;
        },
      });
      hls.loadSource(src);
      hls.attachMedia(video);
      hls.on(Hls.Events.ERROR, (_e, data) => {
        if (data.fatal) setError("No se pudo cargar el video.");
      });
    } else {
      setError("Tu navegador no soporta la reproducción HLS.");
    }

    void sendSignal("open");
    return () => {
      hls?.destroy();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [assetId]);

  // Acumula tiempo real de reproduccion y emite heartbeats.
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    let last = video.currentTime;
    const onTimeUpdate = () => {
      if (video.paused || video.seeking) {
        last = video.currentTime;
        return;
      }
      const delta = video.currentTime - last;
      last = video.currentTime;
      // Ignora saltos grandes (seek): solo cuenta reproduccion continua.
      if (delta > 0 && delta < 2) {
        accumulated.current += delta;
        if (accumulated.current >= HEARTBEAT_SECS) {
          void sendSignal("heartbeat", accumulated.current);
          accumulated.current = 0;
        }
      }
    };

    const onEnded = () => {
      if (!completed.current) {
        completed.current = true;
        void sendSignal("complete", accumulated.current);
        accumulated.current = 0;
      }
    };

    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("ended", onEnded);
    return () => {
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("ended", onEnded);
      // Al desmontar, aprovecha el tiempo acumulado restante.
      if (accumulated.current > 0) void sendSignal("heartbeat", accumulated.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [enrollmentId, resourceStableId]);

  if (error) return <Alert variant="error">{error}</Alert>;

  return (
    <video
      ref={videoRef}
      controls
      className="aspect-video w-full brutal-border bg-black"
    />
  );
}
