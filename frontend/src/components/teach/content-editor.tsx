"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { AlertTriangle, Check, Loader2 } from "lucide-react";

import { Textarea } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { MarkdownClient } from "@/components/content/markdown-client";

/**
 * Editor de contenido Markdown con AUTOSAVE y aviso de CONFLICTO.
 *
 * - Autosave con debounce: a los ~1.2s de inactividad guarda via
 *   PUT /resources/:id/content enviando expected_revision.
 * - Si el backend responde 409 (otra pestaña/editor guardo), NO se pisa el
 *   trabajo ajeno: se muestra un aviso con la revision del servidor y se ofrece
 *   recargar el contenido remoto o forzar el guardado.
 */

type SaveState = "idle" | "saving" | "saved" | "conflict" | "error";

export function ContentEditor({
  resourceId,
  initialContent,
  initialRevision,
  title,
}: {
  resourceId: string;
  initialContent: string;
  initialRevision: number;
  title: string;
}) {
  const [content, setContent] = useState(initialContent);
  const [revision, setRevision] = useState(initialRevision);
  const [state, setState] = useState<SaveState>("idle");
  const [serverRevision, setServerRevision] = useState<number | null>(null);
  const [errorMsg, setErrorMsg] = useState<string | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const dirty = useRef(false);

  const save = useCallback(
    async (opts?: { force?: boolean }) => {
      setState("saving");
      setErrorMsg(null);
      const res = await fetch(`/api/resources/${resourceId}/content/put`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          content_md: content,
          // En modo forzado no enviamos expected_revision: sobrescribe.
          expected_revision: opts?.force ? undefined : revision,
        }),
      });
      const data = await res.json().catch(() => null);

      if (res.status === 409) {
        setState("conflict");
        setServerRevision(data?.details?.current_revision ?? null);
        return;
      }
      if (!res.ok) {
        setState("error");
        setErrorMsg(data?.error?.message ?? "No se pudo guardar.");
        return;
      }
      // El backend devuelve la revision nueva.
      if (typeof data?.revision === "number") setRevision(data.revision);
      dirty.current = false;
      setState("saved");
    },
    [content, resourceId, revision],
  );

  // Autosave con debounce.
  useEffect(() => {
    if (!dirty.current) return;
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => void save(), 1200);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [content, save]);

  async function reloadRemote() {
    const res = await fetch(`/api/resources/${resourceId}/content`);
    const data = await res.json();
    if (res.ok) {
      setContent(data.content_md ?? "");
      setRevision(data.revision ?? 0);
      setState("idle");
      setServerRevision(null);
      dirty.current = false;
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-xl font-extrabold uppercase">{title}</h2>
        <SaveIndicator state={state} revision={revision} />
      </div>

      {state === "conflict" && (
        <Alert variant="warning" title="Conflicto de edición">
          Otro editor guardó cambios (revisión {serverRevision ?? "?"}) mientras
          editabas (tu revisión {revision}). Para no pisar su trabajo, elige:
          <div className="mt-2 flex gap-2">
            <Button size="sm" variant="outline" onClick={reloadRemote}>
              Cargar versión del servidor
            </Button>
            <Button size="sm" variant="danger" onClick={() => void save({ force: true })}>
              Sobrescribir con lo mío
            </Button>
          </div>
        </Alert>
      )}

      {state === "error" && errorMsg && <Alert variant="error">{errorMsg}</Alert>}

      <div className="grid gap-4 lg:grid-cols-2">
        <div>
          <p className="mb-1.5 text-sm font-bold uppercase">Markdown</p>
          <Textarea
            className="min-h-[420px] font-mono text-sm"
            value={content}
            onChange={(e) => {
              dirty.current = true;
              setContent(e.target.value);
              setState("idle");
            }}
          />
        </div>
        <div>
          <p className="mb-1.5 text-sm font-bold uppercase">Vista previa</p>
          <div className="min-h-[420px] brutal-border bg-[var(--card)] p-4">
            <MarkdownClient content={content} />
          </div>
        </div>
      </div>

      <Button onClick={() => void save()} disabled={state === "saving"}>
        Guardar ahora
      </Button>
    </div>
  );
}

function SaveIndicator({ state, revision }: { state: SaveState; revision: number }) {
  const map = {
    idle: { icon: null, text: `rev ${revision}`, cls: "opacity-60" },
    saving: { icon: <Loader2 className="h-4 w-4 animate-spin" />, text: "Guardando…", cls: "" },
    saved: { icon: <Check className="h-4 w-4 text-ok" />, text: "Guardado", cls: "" },
    conflict: { icon: <AlertTriangle className="h-4 w-4 text-warn" />, text: "Conflicto", cls: "text-warn" },
    error: { icon: <AlertTriangle className="h-4 w-4 text-danger" />, text: "Error", cls: "text-danger" },
  }[state];
  return (
    <span className={`flex items-center gap-1 text-sm font-bold uppercase ${map.cls}`}>
      {map.icon}
      {map.text}
    </span>
  );
}
