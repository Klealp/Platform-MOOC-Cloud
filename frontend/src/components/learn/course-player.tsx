"use client";

import { useMemo, useState } from "react";
import { CheckCircle2, Circle, Clock, FileText, Film, HelpCircle } from "lucide-react";

import { ProgressBar } from "@/components/ui/progress-bar";
import { Tag } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type {
  EnrollmentOutline,
  OutlineResource,
} from "@/lib/api/types";
import { ResourceViewer } from "./resource-viewer";

/**
 * Reproductor de curso. Mantiene en el cliente:
 *  - el recurso activo (navegacion por el arbol),
 *  - el progreso por recurso y el porcentaje global, que se actualizan en vivo
 *    cuando el servidor confirma una senal (via onProgress del viewer).
 */
export function CoursePlayer({ initial }: { initial: EnrollmentOutline }) {
  const [progressPct, setProgressPct] = useState(initial.progress.progress_pct);
  const [completedMap, setCompletedMap] = useState<Record<string, boolean>>(
    () =>
      Object.fromEntries(
        Object.entries(initial.resource_progress).map(([k, v]) => [k, v.completed]),
      ),
  );

  const flatResources = useMemo(() => {
    const list: OutlineResource[] = [];
    for (const m of initial.modules)
      for (const u of m.units) for (const r of u.resources) list.push(r);
    return list;
  }, [initial.modules]);

  const [activeStableId, setActiveStableId] = useState(
    flatResources[0]?.stable_id ?? "",
  );
  const active = flatResources.find((r) => r.stable_id === activeStableId);

  function markCompleted(stableId: string, completed: boolean, pct?: number) {
    setCompletedMap((prev) => ({ ...prev, [stableId]: completed }));
    if (pct !== undefined) setProgressPct(pct);
  }

  return (
    <main className="mx-auto grid max-w-6xl gap-6 px-4 py-8 lg:grid-cols-[320px_1fr]">
      {/* Sidebar: arbol + progreso */}
      <aside className="space-y-4 lg:sticky lg:top-24 lg:self-start">
        <div className="elevate-sm rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
          <p className="mb-2 text-sm font-medium">Tu progreso</p>
          <ProgressBar value={progressPct} />
          <p className="mt-2 text-xs font-medium text-[var(--color-text-muted)]">
            {initial.progress.state}
          </p>
        </div>

        <nav className="space-y-3">
          {initial.modules.map((mod, i) => (
            <div key={mod.id} className="rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)]">
              <p className="border-b border-[var(--color-border)] bg-[var(--color-surface-alt)] px-3 py-2 text-sm font-semibold">
                {i + 1}. {mod.title}
              </p>
              <ul>
                {mod.units.map((u) =>
                  u.resources.map((r) => {
                    const done = completedMap[r.stable_id];
                    const isActive = r.stable_id === activeStableId;
                    // Un recurso con archivo adjunto que aún no está `ready`
                    // está en procesamiento y no debe presentarse como listo.
                    const processing =
                      !!r.asset_id && !!r.asset_status && r.asset_status !== "ready";
                    const Icon =
                      r.type === "video"
                        ? Film
                        : r.type === "quiz"
                          ? HelpCircle
                          : FileText;
                    return (
                      <li key={r.id}>
                        <button
                          onClick={() => setActiveStableId(r.stable_id)}
                          className={cn(
                            "flex w-full items-center gap-2 border-b border-[var(--color-border)] px-3 py-2 text-left text-sm font-medium transition-colors last:border-b-0",
                            isActive
                              ? "border-l-2 border-l-[var(--color-primary)] bg-[color-mix(in_srgb,var(--color-primary)_10%,transparent)]"
                              : "hover:bg-[var(--color-surface-alt)]",
                          )}
                        >
                          {done ? (
                            <CheckCircle2 className="h-4 w-4 shrink-0 text-[var(--color-success)]" />
                          ) : (
                            <Circle className="h-4 w-4 shrink-0 text-[var(--color-text-muted)]" />
                          )}
                          <Icon className="h-4 w-4 shrink-0" />
                          <span className="flex-1 truncate">{r.title}</span>
                          {processing && (
                            <Clock
                              className="h-3.5 w-3.5 shrink-0 text-[var(--color-warning)]"
                              aria-label="En procesamiento"
                            />
                          )}
                          {r.required && (
                            <span className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]" title="Obligatorio" />
                          )}
                        </button>
                      </li>
                    );
                  }),
                )}
              </ul>
            </div>
          ))}
        </nav>
      </aside>

      {/* Panel del recurso activo */}
      <section className="min-w-0 space-y-4">
        {active ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <h1 className="text-2xl font-semibold">{active.title}</h1>
              <Tag tone="blue">{active.type}</Tag>
              {completedMap[active.stable_id] && <Tag tone="ok">completado</Tag>}
            </div>
            <ResourceViewer
              key={active.stable_id}
              resource={active}
              enrollmentId={initial.enrollment_id}
              completed={!!completedMap[active.stable_id]}
              onProgress={(done, pct) => markCompleted(active.stable_id, done, pct)}
            />
          </>
        ) : (
          <p className="font-bold">Este curso aún no tiene contenido.</p>
        )}
      </section>
    </main>
  );
}
