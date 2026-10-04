"use client";

import { useState, useTransition } from "react";
import { Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input, Select, Textarea, Field } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { createResourceAction, type CreateResourceInput } from "@/lib/authoring/actions";
import { ResumableUpload } from "./resumable-upload";
import type { ResourceType } from "@/lib/api/types";

const TYPES: { value: ResourceType; label: string }[] = [
  { value: "rich_text", label: "Lectura (Markdown)" },
  { value: "video", label: "Video" },
  { value: "audio", label: "Audio" },
  { value: "pdf", label: "PDF" },
  { value: "image", label: "Imagen" },
  { value: "download", label: "Descargable" },
  { value: "external_link", label: "Enlace externo" },
  { value: "quiz", label: "Quiz" },
];

const MEDIA_TYPES = new Set<ResourceType>(["video", "audio", "pdf", "image", "download"]);

export function AddResourceDialog({
  unitId,
  onDone,
}: {
  unitId: string;
  onDone: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [type, setType] = useState<ResourceType>("rich_text");
  const [title, setTitle] = useState("");
  const [contentMd, setContentMd] = useState("");
  const [externalUrl, setExternalUrl] = useState("");
  const [assetId, setAssetId] = useState("");
  const [required, setRequired] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pending, start] = useTransition();

  function reset() {
    setTitle("");
    setContentMd("");
    setExternalUrl("");
    setAssetId("");
    setType("rich_text");
    setRequired(true);
    setError(null);
  }

  function submit() {
    if (!title.trim()) {
      setError("El título es obligatorio.");
      return;
    }
    const input: CreateResourceInput = { type, title: title.trim(), required };
    if (type === "rich_text") input.content_md = contentMd;
    if (type === "external_link") input.external_url = externalUrl.trim();
    if (MEDIA_TYPES.has(type)) {
      if (!assetId) {
        setError("Sube un archivo antes de crear el recurso.");
        return;
      }
      input.asset_id = assetId;
    }
    start(async () => {
      const res = await createResourceAction(unitId, input);
      if (res.error) setError(res.error);
      else {
        reset();
        setOpen(false);
        onDone();
      }
    });
  }

  if (!open) {
    return (
      <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
        <Plus className="h-4 w-4" /> Recurso
      </Button>
    );
  }

  return (
    <div className="space-y-3 brutal-border bg-[var(--card)] p-3">
      {error && <Alert variant="error">{error}</Alert>}
      <div className="grid gap-3 sm:grid-cols-2">
        <Field label="Tipo">
          <Select value={type} onChange={(e) => setType(e.target.value as ResourceType)}>
            {TYPES.map((t) => (
              <option key={t.value} value={t.value}>{t.label}</option>
            ))}
          </Select>
        </Field>
        <Field label="Título">
          <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Título del recurso" />
        </Field>
      </div>

      {type === "rich_text" && (
        <Field label="Contenido (Markdown)">
          <Textarea
            value={contentMd}
            onChange={(e) => setContentMd(e.target.value)}
            placeholder="# Título&#10;&#10;Escribe en Markdown…"
          />
        </Field>
      )}

      {type === "external_link" && (
        <Field label="URL externa">
          <Input
            value={externalUrl}
            onChange={(e) => setExternalUrl(e.target.value)}
            placeholder="https://…"
          />
        </Field>
      )}

      {MEDIA_TYPES.has(type) && (
        <div>
          <p className="mb-1.5 text-sm font-bold uppercase">Archivo</p>
          <ResumableUpload onComplete={setAssetId} />
        </div>
      )}

      <label className="flex items-center gap-2 text-sm font-bold">
        <input
          type="checkbox"
          checked={required}
          onChange={(e) => setRequired(e.target.checked)}
          className="h-4 w-4 border-2 border-ink"
        />
        Obligatorio para completar el curso
      </label>

      <div className="flex gap-2">
        <Button size="sm" onClick={submit} disabled={pending}>
          {pending ? "Creando…" : "Crear recurso"}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => { reset(); setOpen(false); }}>
          Cancelar
        </Button>
      </div>
    </div>
  );
}
