import * as React from "react";
import { cn } from "@/lib/utils";

/**
 * Etiqueta/chip minimalista: píldora de fondo tenue y texto del color
 * semántico. Se conservan los nombres de tono para no tocar los llamadores.
 */
type Tone =
  | "brand"
  | "yellow"
  | "pink"
  | "blue"
  | "purple"
  | "ok"
  | "warn"
  | "danger"
  | "neutral";

const tones: Record<Tone, { bg: string; fg: string }> = {
  brand: {
    bg: "color-mix(in srgb, var(--color-primary) 12%, transparent)",
    fg: "var(--color-primary)",
  },
  blue: {
    bg: "color-mix(in srgb, var(--color-accent) 14%, transparent)",
    fg: "var(--color-accent)",
  },
  purple: {
    bg: "color-mix(in srgb, var(--color-primary) 12%, transparent)",
    fg: "var(--color-primary)",
  },
  yellow: {
    bg: "color-mix(in srgb, var(--color-warning) 14%, transparent)",
    fg: "var(--color-warning)",
  },
  pink: {
    bg: "color-mix(in srgb, var(--color-danger) 10%, transparent)",
    fg: "var(--color-danger)",
  },
  ok: {
    bg: "color-mix(in srgb, var(--color-success) 14%, transparent)",
    fg: "var(--color-success)",
  },
  warn: {
    bg: "color-mix(in srgb, var(--color-warning) 14%, transparent)",
    fg: "var(--color-warning)",
  },
  danger: {
    bg: "color-mix(in srgb, var(--color-danger) 12%, transparent)",
    fg: "var(--color-danger)",
  },
  neutral: {
    bg: "var(--color-surface-alt)",
    fg: "var(--color-text-muted)",
  },
};

export function Tag({
  tone = "neutral",
  className,
  style,
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & { tone?: Tone }) {
  const t = tones[tone];
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium",
        className,
      )}
      style={{ backgroundColor: t.bg, color: t.fg, ...style }}
      {...props}
    />
  );
}

/** Mapea un estado de dominio a un tono de etiqueta. */
export function statusTone(status: string): Tone {
  switch (status) {
    case "published":
    case "active":
    case "approved":
    case "ready":
      return "ok";
    case "draft":
    case "pending_verification":
    case "pending":
    case "processing":
      return "warn";
    case "suspended":
    case "revoked":
    case "withdrawn":
    case "failed":
      return "danger";
    default:
      return "neutral";
  }
}
