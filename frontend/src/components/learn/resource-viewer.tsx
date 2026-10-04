"use client";

import { useState, useTransition } from "react";
import { CheckCircle2, Download, ExternalLink } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { Card, CardBody } from "@/components/ui/card";
import { VideoPlayer } from "./video-player";
import { QuizRunner } from "./quiz-runner";
import { RichTextContent } from "./rich-text-content";
import type { OutlineResource } from "@/lib/api/types";

/**
 * Renderiza el recurso activo segun su tipo y conecta las senales de progreso.
 *
 * - video/audio: reproductor HLS con heartbeats (envia open/heartbeat/complete).
 * - rich_text/pdf/download/external_link: muestra el contenido y ofrece un
 *   boton "Marcar como visto" que envia la senal `complete` (el backend valida
 *   el tiempo minimo de permanencia).
 * - quiz: lanza el QuizRunner.
 */
export function ResourceViewer({
  resource,
  enrollmentId,
  completed,
  onProgress,
}: {
  resource: OutlineResource;
  enrollmentId: string;
  completed: boolean;
  onProgress: (completed: boolean, pct?: number) => void;
}) {
  const [pending, start] = useTransition();
  const [error, setError] = useState<string | null>(null);

  async function markComplete() {
    setError(null);
    const res = await fetch("/api/progress", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        enrollment_id: enrollmentId,
        resource_stable_id: resource.stable_id,
        event_type: "complete",
        delta_secs: 30,
      }),
    });
    if (res.ok) {
      const data = await res.json();
      onProgress(data.resource?.completed ?? true, data.progress?.progress_pct);
    } else {
      const data = await res.json().catch(() => null);
      setError(data?.error?.message ?? "No se pudo marcar como completado.");
    }
  }

  const completeButton = resource.type !== "quiz" && resource.type !== "video" && (
    <div className="space-y-2 pt-4">
      {error && <Alert variant="warning">{error}</Alert>}
      <Button
        variant={completed ? "outline" : "primary"}
        disabled={pending || completed}
        onClick={() => start(markComplete)}
      >
        <CheckCircle2 className="h-5 w-5" />
        {completed ? "Completado" : pending ? "Guardando…" : "Marcar como visto"}
      </Button>
    </div>
  );

  switch (resource.type) {
    case "video":
    case "audio":
      return (
        <div className="space-y-4">
          {resource.asset_id ? (
            <VideoPlayer
              assetId={resource.asset_id}
              enrollmentId={enrollmentId}
              resourceStableId={resource.stable_id}
              onProgress={onProgress}
            />
          ) : (
            <Alert variant="warning">Este recurso no tiene archivo asociado.</Alert>
          )}
        </div>
      );

    case "rich_text":
      return (
        <Card>
          <CardBody className="space-y-4">
            {resource.has_content ? (
              <RichTextContent resourceId={resource.id} />
            ) : (
              <Alert variant="info">Este recurso aún no tiene contenido.</Alert>
            )}
            {completeButton}
          </CardBody>
        </Card>
      );

    case "external_link":
      return (
        <Card>
          <CardBody className="space-y-4">
            <a href={resource.external_url ?? "#"} target="_blank" rel="noopener noreferrer">
              <Button variant="secondary">
                <ExternalLink className="h-5 w-5" /> Abrir recurso externo
              </Button>
            </a>
            {completeButton}
          </CardBody>
        </Card>
      );

    case "pdf":
    case "download":
    case "image":
      return (
        <Card>
          <CardBody className="space-y-4">
            {resource.asset_id ? (
              <a href={`/api/assets/${resource.asset_id}/url`} target="_blank" rel="noopener noreferrer">
                <Button variant="secondary">
                  <Download className="h-5 w-5" /> Abrir / descargar
                </Button>
              </a>
            ) : (
              <Alert variant="warning">Sin archivo asociado.</Alert>
            )}
            {completeButton}
          </CardBody>
        </Card>
      );

    case "quiz":
      return (
        <QuizRunner
          resource={resource}
          onPassed={() => onProgress(true)}
        />
      );

    default:
      return <Alert variant="info">Tipo de recurso no soportado.</Alert>;
  }
}
