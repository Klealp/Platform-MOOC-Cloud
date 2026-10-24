import * as React from "react";
import { AlertTriangle, CheckCircle2, Info, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";

type Variant = "info" | "success" | "warning" | "error";

/**
 * Aviso minimalista: fondo tenue + borde e icono del color semántico. El color
 * nunca es el único portador del significado (siempre hay icono y, si procede,
 * título).
 */
const config: Record<
  Variant,
  { icon: typeof Info; accent: string; tint: string }
> = {
  info: {
    icon: Info,
    accent: "var(--color-accent)",
    tint: "color-mix(in srgb, var(--color-accent) 10%, var(--color-surface))",
  },
  success: {
    icon: CheckCircle2,
    accent: "var(--color-success)",
    tint: "color-mix(in srgb, var(--color-success) 10%, var(--color-surface))",
  },
  warning: {
    icon: AlertTriangle,
    accent: "var(--color-warning)",
    tint: "color-mix(in srgb, var(--color-warning) 12%, var(--color-surface))",
  },
  error: {
    icon: XCircle,
    accent: "var(--color-danger)",
    tint: "color-mix(in srgb, var(--color-danger) 10%, var(--color-surface))",
  },
};

export function Alert({
  variant = "info",
  title,
  children,
  className,
}: {
  variant?: Variant;
  title?: string;
  children?: React.ReactNode;
  className?: string;
}) {
  const { icon: Icon, accent, tint } = config[variant];
  return (
    // Solo un error interrumpe al lector de pantalla (role="alert"); el resto
    // se anuncia con cortesia (role="status") cuando aparece.
    <div
      role={variant === "error" ? "alert" : "status"}
      className={cn(
        "flex gap-3 rounded-[var(--radius-md)] border p-3.5 text-[var(--color-text)]",
        className,
      )}
      style={{ backgroundColor: tint, borderColor: accent }}
    >
      <Icon
        className="mt-0.5 h-5 w-5 shrink-0"
        style={{ color: accent }}
        aria-hidden
      />
      <div className="text-sm leading-relaxed">
        {title && <p className="font-semibold">{title}</p>}
        {children}
      </div>
    </div>
  );
}
