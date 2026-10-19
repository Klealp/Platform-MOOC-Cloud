"use client";

import { useEffect, useState } from "react";
import { Check, Copy, ExternalLink, Share2 } from "lucide-react";

import { Button } from "@/components/ui/button";

/**
 * URL publica para compartir una insignia. Apunta a la pagina legible
 * /badges/verify/:code del frontend (no al JSON del backend), que cualquiera
 * puede abrir sin registrarse. El origen se resuelve en el navegador para que
 * la URL sea la misma por la que el estudiante entro (localhost o sslip.io).
 */
export function ShareBadgeLink({ code, courseTitle }: { code: string; courseTitle: string }) {
  const path = `/badges/verify/${encodeURIComponent(code)}`;
  const [url, setUrl] = useState(path);
  const [copied, setCopied] = useState(false);
  const [canShare, setCanShare] = useState(false);

  // Se calcula tras montar para no romper la hidratacion (el servidor no
  // conoce el origen publico).
  useEffect(() => {
    setUrl(`${window.location.origin}${path}`);
    setCanShare(typeof navigator.share === "function");
  }, [path]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(url);
    } catch {
      // Sin permiso de portapapeles (p. ej. HTTP sin TLS): se selecciona el
      // texto para que el estudiante lo copie a mano.
      const input = document.getElementById(`badge-url-${code}`) as HTMLInputElement | null;
      input?.select();
      return;
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  async function share() {
    try {
      await navigator.share({ title: `Insignia: ${courseTitle}`, url });
    } catch {
      // El usuario cancelo el dialogo nativo; no es un error.
    }
  }

  return (
    <div className="space-y-2 text-left">
      <label htmlFor={`badge-url-${code}`} className="text-xs font-medium text-[var(--color-text-muted)]">
        Enlace público de verificación
      </label>
      <input
        id={`badge-url-${code}`}
        readOnly
        value={url}
        onFocus={(e) => e.currentTarget.select()}
        className="w-full truncate rounded-[var(--radius-sm)] border border-[var(--color-border)] bg-[var(--color-surface-alt)] px-2 py-1.5 font-mono text-xs"
      />
      <div className="flex flex-wrap items-center justify-center gap-2">
        <Button size="sm" variant="outline" onClick={copy}>
          {copied ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
          {copied ? "Copiado" : "Copiar enlace"}
        </Button>
        {canShare && (
          <Button size="sm" variant="ghost" onClick={share}>
            <Share2 className="h-4 w-4" /> Compartir
          </Button>
        )}
        <a
          href={path}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] hover:underline"
        >
          Ver página pública <ExternalLink className="h-4 w-4" />
        </a>
      </div>
    </div>
  );
}
