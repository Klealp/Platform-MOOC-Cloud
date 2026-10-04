import { cn } from "@/lib/utils";

/** Barra de progreso brutalista: bloque solido con borde negro. */
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
  return (
    <div className={cn("w-full", className)}>
      <div
        className="relative h-6 w-full brutal-border bg-[var(--card)] overflow-hidden"
        role="progressbar"
        aria-valuenow={pct}
        aria-valuemin={0}
        aria-valuemax={100}
      >
        <div
          className={cn(
            "h-full border-r-[3px] border-ink transition-[width] duration-500",
            pct >= 100 ? "bg-ok" : "bg-brand-500",
          )}
          style={{ width: `${pct}%` }}
        />
        {showLabel && (
          <span className="absolute inset-0 flex items-center justify-center text-xs font-extrabold">
            {pct}%
          </span>
        )}
      </div>
    </div>
  );
}
