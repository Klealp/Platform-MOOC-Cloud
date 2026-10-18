import { cn } from "@/lib/utils";

/**
 * Bloque de carga (skeleton). Se usa en lugar de spinners genéricos para
 * comunicar la estructura que está por llegar (guía de estilo, estado "cargando").
 */
export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      className={cn(
        "animate-pulse rounded-[var(--radius-sm)] bg-[var(--color-surface-alt)]",
        className,
      )}
      aria-hidden
    />
  );
}

/** Tarjeta-esqueleto para grillas de catálogo / cursos. */
export function CardSkeleton() {
  return (
    <div className="rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)] p-5 elevate-sm">
      <div className="space-y-3">
        <Skeleton className="h-4 w-20" />
        <Skeleton className="h-5 w-4/5" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-2/3" />
      </div>
    </div>
  );
}
