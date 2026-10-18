"use client";

import { useEffect, useState } from "react";

import { Alert } from "@/components/ui/alert";
import { MarkdownClient } from "@/components/content/markdown-client";

/**
 * Carga y muestra el contenido Markdown de un recurso rich_text.
 * Si el backend no autoriza la lectura al estudiante (gap conocido del
 * contrato), degrada mostrando un aviso en lugar de romperse.
 */
export function RichTextContent({ resourceId }: { resourceId: string }) {
  const [content, setContent] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);

  useEffect(() => {
    let active = true;
    fetch(`/api/resources/${resourceId}/content`)
      .then(async (res) => {
        if (!res.ok) throw new Error(String(res.status));
        return res.json();
      })
      .then((data) => {
        if (active) setContent(data.content_md ?? "");
      })
      .catch(() => {
        if (active) setUnavailable(true);
      });
    return () => {
      active = false;
    };
  }, [resourceId]);

  if (unavailable) {
    return (
      <Alert variant="info">
        El contenido de esta lectura no está disponible por la API para
        estudiantes todavía.
      </Alert>
    );
  }
  if (content === null) {
    // Estado de carga con skeleton (no spinner), según la guía de estilo.
    return (
      <div className="reading space-y-3" aria-busy="true" aria-live="polite">
        <span className="sr-only">Cargando contenido…</span>
        <div className="h-6 w-2/3 animate-pulse rounded bg-[var(--color-surface-alt)]" />
        <div className="h-4 w-full animate-pulse rounded bg-[var(--color-surface-alt)]" />
        <div className="h-4 w-11/12 animate-pulse rounded bg-[var(--color-surface-alt)]" />
        <div className="h-4 w-full animate-pulse rounded bg-[var(--color-surface-alt)]" />
        <div className="h-4 w-4/5 animate-pulse rounded bg-[var(--color-surface-alt)]" />
      </div>
    );
  }
  // Contenido de lectura constreñido a ~720px para legibilidad.
  return (
    <div className="reading">
      <MarkdownClient content={content} />
    </div>
  );
}
