"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import {
  ArrowRight,
  CheckCircle2,
  Circle,
  Clock,
  FileText,
  Film,
  HelpCircle,
  PartyPopper,
} from "lucide-react";

import { ProgressBar } from "@/components/ui/progress-bar";
import { Tag } from "@/components/ui/badge";
import { Alert } from "@/components/ui/alert";
import { Button, ButtonLink } from "@/components/ui/button";
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
 *
 * Cuando el servidor confirma en esta visita que el curso quedo aprobado y al
 * 100%, a los 2 segundos lleva al estudiante a la ficha del curso, que muestra
 * la felicitacion y el acceso a su insignia.
 */

const REDIRECT_MS = 2000;

export function CoursePlayer({
  initial,
  courseSlug,
}: {
  initial: EnrollmentOutline;
  courseSlug: string;
}) {
  const router = useRouter();
  const [progressPct, setProgressPct] = useState(initial.progress.progress_pct);
  const [courseState, setCourseState] = useState(initial.progress.state);
  // Un curso que ya estaba terminado al entrar no redirige: el estudiante
  // vuelve a repasar y no debe ser expulsado del reproductor.
  const [finishedAtStart] = useState(
    () => isFinished(initial.progress.state, initial.progress.progress_pct),
  );
  const justFinished = !finishedAtStart && isFinished(courseState, progressPct);
  const resultHref = courseSlug
    ? `/catalog/${encodeURIComponent(courseSlug)}`
    : "/learn/badges";
  const headingRef = useRef<HTMLHeadingElement>(null);
  const [completedMap, setCompletedMap] = useState<Record<string, boolean>>(
    () =>
      Object.fromEntries(
        Object.entries(initial.resource_progress).map(([k, v]) => [k, v.completed]),
      ),
  );

  // Lista plana en el orden del curso, con la unidad y el modulo de cada
  // recurso para rotular el boton "Siguiente".
  const flatResources = useMemo(() => {
    const list: { resource: OutlineResource; unitId: string; moduleId: string }[] = [];
    for (const m of initial.modules)
      for (const u of m.units)
        for (const r of u.resources) list.push({ resource: r, unitId: u.id, moduleId: m.id });
    return list;
  }, [initial.modules]);

  const [activeStableId, setActiveStableId] = useState(
    flatResources[0]?.resource.stable_id ?? "",
  );
  const activeIndex = flatResources.findIndex((x) => x.resource.stable_id === activeStableId);
  const active = flatResources[activeIndex]?.resource;
  const next = activeIndex >= 0 ? flatResources[activeIndex + 1] : undefined;
  const nextLabel = !next
    ? ""
    : next.unitId === flatResources[activeIndex].unitId
      ? "Siguiente recurso"
      : next.moduleId === flatResources[activeIndex].moduleId
        ? "Siguiente unidad"
        : "Siguiente módulo";

  useEffect(() => {
    if (!justFinished) return;
    const timer = setTimeout(() => router.push(resultHref), REDIRECT_MS);
    return () => clearTimeout(timer);
  }, [justFinished, resultHref, router]);

  function markCompleted(stableId: string, completed: boolean, pct?: number, state?: string) {
    setCompletedMap((prev) => ({ ...prev, [stableId]: completed }));
    if (pct !== undefined) setProgressPct(pct);
    if (state) setCourseState(state);
  }

  function goTo(stableId: string) {
    setActiveStableId(stableId);
    // El foco pasa al titulo del recurso nuevo: quien usa teclado o lector de
    // pantalla sigue leyendo desde ahi y no queda al final de la pagina.
    requestAnimationFrame(() => {
      headingRef.current?.focus();
      headingRef.current?.scrollIntoView({ block: "start" });
    });
  }

  return (
    <main id="contenido" tabIndex={-1} className="mx-auto grid max-w-6xl gap-6 px-4 py-8 lg:grid-cols-[320px_1fr]">
      {/* Sidebar: arbol + progreso */}
      <aside className="space-y-4 lg:sticky lg:top-24 lg:self-start">
        <div className="elevate-sm rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4">
          <p className="mb-2 text-sm font-medium">Tu progreso</p>
          <ProgressBar value={progressPct} label="Progreso del curso" />
          <p className="mt-2 text-xs font-medium text-[var(--color-text-muted)]">
            {courseState}
          </p>
        </div>

        <nav aria-label="Contenido del curso" className="space-y-3">
          {initial.modules.map((mod, i) => (
            <div key={mod.id} className="rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)]">
              <h2 className="border-b border-[var(--color-border)] bg-[var(--color-surface-alt)] px-3 py-2 text-sm font-semibold">
                {i + 1}. {mod.title}
              </h2>
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
                        {/* El estado (activo, completado, obligatorio, en
                            proceso) se dice tambien en texto: los iconos y el
                            color solos no llegan a un lector de pantalla. */}
                        <button
                          type="button"
                          aria-current={isActive ? "true" : undefined}
                          onClick={() => goTo(r.stable_id)}
                          className={cn(
                            "flex w-full items-center gap-2 border-b border-[var(--color-border)] px-3 py-2 text-left text-sm font-medium transition-colors last:border-b-0",
                            isActive
                              ? "border-l-2 border-l-[var(--color-primary)] bg-[color-mix(in_srgb,var(--color-primary)_10%,transparent)]"
                              : "hover:bg-[var(--color-surface-alt)]",
                          )}
                        >
                          {done ? (
                            <CheckCircle2 aria-hidden className="h-4 w-4 shrink-0 text-[var(--color-success)]" />
                          ) : (
                            <Circle aria-hidden className="h-4 w-4 shrink-0 text-[var(--color-text-muted)]" />
                          )}
                          <Icon aria-hidden className="h-4 w-4 shrink-0" />
                          <span className="flex-1 truncate">{r.title}</span>
                          <span className="sr-only">
                            {done ? ", completado" : ", pendiente"}
                            {r.required ? ", obligatorio" : ""}
                            {processing ? ", en procesamiento" : ""}
                          </span>
                          {processing && (
                            <Clock
                              aria-hidden
                              className="h-3.5 w-3.5 shrink-0 text-[var(--color-warning)]"
                            />
                          )}
                          {r.required && (
                            <span
                              aria-hidden
                              className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]"
                              title="Obligatorio"
                            />
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
        <p className="flex items-center gap-2 text-xs text-[var(--color-text-muted)]" aria-hidden>
          <span className="h-2 w-2 rounded-full bg-[var(--color-accent)]" /> Obligatorio
        </p>
      </aside>

      {/* Panel del recurso activo */}
      <section className="min-w-0 space-y-4">
        {active ? (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <h1 ref={headingRef} tabIndex={-1} className="text-2xl font-semibold">
                {active.title}
              </h1>
              <Tag tone="blue">{active.type}</Tag>
              {completedMap[active.stable_id] && <Tag tone="ok">completado</Tag>}
            </div>
            <ResourceViewer
              key={active.stable_id}
              resource={active}
              enrollmentId={initial.enrollment_id}
              completed={!!completedMap[active.stable_id]}
              onProgress={(done, pct, state) => markCompleted(active.stable_id, done, pct, state)}
            />

            {justFinished && (
              <Alert variant="success" title="¡Completaste el curso!">
                <span className="flex flex-wrap items-center gap-2">
                  <PartyPopper className="h-4 w-4" aria-hidden />
                  Aprobaste con el 100% del avance. En un momento te llevamos a tu
                  resultado…
                  <a href={resultHref} className="font-medium underline underline-offset-2">
                    Ir ahora
                  </a>
                </span>
              </Alert>
            )}

            <div className="flex flex-wrap items-center justify-end gap-3 border-t border-[var(--color-border)] pt-4">
              {next ? (
                <Button variant="secondary" onClick={() => goTo(next.resource.stable_id)}>
                  <span className="text-left">
                    <span className="block">{nextLabel}</span>
                    <span className="block max-w-[16rem] truncate text-xs font-normal text-[var(--color-text-muted)]">
                      {next.resource.title}
                    </span>
                  </span>
                  <ArrowRight className="h-5 w-5" aria-hidden />
                </Button>
              ) : isFinished(courseState, progressPct) ? (
                <ButtonLink href={resultHref}>
                  Ver mi resultado <ArrowRight className="h-5 w-5" aria-hidden />
                </ButtonLink>
              ) : (
                <p className="text-sm text-[var(--color-text-muted)]">
                  Este es el último recurso del curso.
                </p>
              )}
            </div>
          </>
        ) : (
          <p className="font-bold">Este curso aún no tiene contenido.</p>
        )}
      </section>
    </main>
  );
}

/** Aprobado y con todo el avance: el porcentaje llega como numero real. */
function isFinished(state: string, pct: number) {
  return state === "approved" && Math.round(pct) >= 100;
}
