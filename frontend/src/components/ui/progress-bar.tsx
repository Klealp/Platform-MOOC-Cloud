import { cn } from "@/lib/utils";

/**
 * Barra de progreso minimalista: pista sutil + relleno de marca/éxito, con el
 * porcentaje numérico al lado (no se depende solo del color).
 */
export function ProgressBar({
  value,
  className,
  showLabel = true,
}: {
  value: number;
  className?: string;
  showLabel?: boolean;
}) {
  const pct = Math.max(0, Math.min(100, Math.round(value)));
  const done = pct >= 100;
  return (
    <div className={cn("flex items-center gap-3", className)}>
      <div
        className="h-2 flex-1 overflow-hidden rounded-full bg-[var(--color-surface-alt)]"
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <div
          className="h-full rounded-full transition-[width] duration-300 ease-out"
          style={{
            width: `${pct}%`,
            backgroundColor: done
              ? "var(--color-success)"
              : "var(--color-primary)",
          }}
        />
      </div>
      {showLabel && (
        <span className="w-10 shrink-0 text-right text-sm font-medium tabular-nums text-[var(--color-text-muted)]">
          {pct}%
        </span>
      )}
    </div>
  );
}
