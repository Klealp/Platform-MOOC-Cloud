import * as React from "react";
import { cn } from "@/lib/utils";

type Tone = "brand" | "yellow" | "pink" | "blue" | "purple" | "ok" | "warn" | "danger" | "neutral";

const tones: Record<Tone, string> = {
  brand: "bg-brand-500 text-ink",
  yellow: "bg-accent-yellow text-ink",
  pink: "bg-accent-pink text-ink",
  blue: "bg-accent-blue text-ink",
  purple: "bg-accent-purple text-white",
  ok: "bg-ok text-white",
  warn: "bg-warn text-ink",
  danger: "bg-danger text-white",
  neutral: "bg-neutral-200 text-ink",
};

export function Tag({
  tone = "neutral",
  className,
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & { tone?: Tone }) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 border-2 border-ink px-2 py-0.5 text-xs font-bold uppercase tracking-wide",
        tones[tone],
        className,
      )}
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
