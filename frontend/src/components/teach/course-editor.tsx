"use client";

import { useRouter } from "next/navigation";
import { useState, useTransition } from "react";
import { Plus, Rocket, Trash2, GitBranch } from "lucide-react";

import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { Tag, statusTone } from "@/components/ui/badge";
import {
  createDraftVersionAction,
  createModuleAction,
  createUnitAction,
  deleteModuleAction,
  deleteResourceAction,
  deleteUnitAction,
  publishVersionAction,
} from "@/lib/authoring/actions";
import type { Course, CourseVersion } from "@/lib/api/types";
import type { VersionOutline } from "@/lib/api/authoring";
import { AddResourceDialog } from "./add-resource-dialog";

/**
 * Editor de curso del profesor.
 *
 * - Versionado: solo la version 'draft' es editable (el backend rechaza con 409
 *   editar una publicada). Si no hay borrador, se ofrece crear uno clonando la
 *   vigente, lo que conserva el progreso de los estudiantes.
 * - Arbol modulos -> unidades -> recursos con crear/eliminar.
 * - Publicar: si la version no cumple, el backend devuelve la lista EXHAUSTIVA
 *   de problemas, que mostramos tal cual.
 */
export function CourseEditor({
  course,
  versions,
  activeVersion,
  outline,
}: {
  course: Course;
  versions: CourseVersion[];
  activeVersion: CourseVersion | null;
  outline: VersionOutline | null;
}) {
  const router = useRouter();
  const [pending, start] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [problems, setProblems] = useState<VersionOutline["problems"]>([]);
  const [newModule, setNewModule] = useState("");
  const [unitDrafts, setUnitDrafts] = useState<Record<string, string>>({});

  const isDraft = activeVersion?.status === "draft";

  function run(fn: () => Promise<{ error?: string }>) {
    setError(null);
    start(async () => {
      const res = await fn();
      if (res?.error) setError(res.error);
      else router.refresh();
    });
  }

  async function publish() {
    setError(null);
    setProblems([]);
    start(async () => {
      const res = await publishVersionAction(activeVersion!.id);
      if (res.problems?.length) {
        setProblems(res.problems);
        setError(res.error ?? "La versión no cumple los requisitos.");
      } else if (res.error) {
        setError(res.error);
      } else {
        router.refresh();
      }
    });
  }

  return (
    <main className="mx-auto max-w-5xl space-y-6 px-4 py-10">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <Tag tone={statusTone(course.status)}>{course.status}</Tag>
            {activeVersion && (
              <Tag tone="blue">
                v{activeVersion.version_number} · {activeVersion.status}
              </Tag>
            )}
          </div>
          <h1 className="text-3xl font-extrabold uppercase">{course.title}</h1>
        </div>
        <div className="flex gap-2">
          {!isDraft && (
            <Button
              variant="secondary"
              disabled={pending}
              onClick={() => run(() => createDraftVersionAction(course.id))}
            >
              <GitBranch className="h-5 w-5" /> Nuevo borrador
            </Button>
          )}
          {isDraft && (
            <Button disabled={pending} onClick={publish}>
              <Rocket className="h-5 w-5" /> Publicar
            </Button>
          )}
        </div>
      </div>

      {error && (
        <Alert variant="error" title="No se pudo completar">
          {error}
        </Alert>
      )}

      {problems.length > 0 && (
        <Alert variant="warning" title="Corrige estos problemas antes de publicar">
          <ul className="mt-1 list-disc space-y-1 pl-5">
            {problems.map((p, i) => (
              <li key={i}>
                <span className="font-mono text-xs opacity-70">{p.path}</span> —{" "}
                {p.message}
              </li>
            ))}
          </ul>
        </Alert>
      )}

      {!isDraft && (
        <Alert variant="info">
          Esta versión está publicada y es inmutable. Crea un borrador para
          editarla; el progreso de los estudiantes se conserva.
        </Alert>
      )}

      {/* Arbol de contenido */}
      <section className="space-y-4">
        {outline?.modules.map((mod, i) => (
          <Card key={mod.id}>
            <CardHeader className="flex items-center justify-between gap-2">
              <CardTitle>
                {i + 1}. {mod.title}
              </CardTitle>
              {isDraft && (
                <button
                  title="Eliminar módulo"
                  disabled={pending}
                  onClick={() => run(() => deleteModuleAction(mod.id))}
                  className="border-2 border-ink bg-danger p-1.5 text-white brutal-press"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              )}
            </CardHeader>
            <CardBody className="space-y-4">
              {mod.units.map((unit) => (
                <div key={unit.id} className="brutal-border bg-brand-50 p-3">
                  <div className="mb-2 flex items-center justify-between gap-2">
                    <p className="font-bold">{unit.title}</p>
                    {isDraft && (
                      <button
                        title="Eliminar unidad"
                        disabled={pending}
                        onClick={() => run(() => deleteUnitAction(unit.id))}
                        className="border-2 border-ink bg-danger p-1 text-white brutal-press"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                  <ul className="space-y-1">
                    {unit.resources.map((r) => (
                      <li
                        key={r.id}
                        className="flex items-center gap-2 border-2 border-ink bg-[var(--card)] px-2 py-1 text-sm font-medium"
                      >
                        <Tag tone="neutral">{r.type}</Tag>
                        {r.type === "rich_text" && isDraft ? (
                          <a
                            href={`/teach/${course.id}/resource/${r.id}`}
                            className="flex-1 truncate underline decoration-2"
                          >
                            {r.title}
                          </a>
                        ) : r.type === "quiz" && r.quiz_id && isDraft ? (
                          <a
                            href={`/teach/${course.id}/quiz/${r.quiz_id}`}
                            className="flex-1 truncate underline decoration-2"
                          >
                            {r.title}
                          </a>
                        ) : (
                          <span className="flex-1 truncate">{r.title}</span>
                        )}
                        {!r.visible && <Tag tone="warn">oculto</Tag>}
                        {isDraft && (
                          <button
                            title="Eliminar recurso"
                            disabled={pending}
                            onClick={() => run(() => deleteResourceAction(r.id))}
                            className="border-2 border-ink bg-danger p-1 text-white brutal-press"
                          >
                            <Trash2 className="h-3 w-3" />
                          </button>
                        )}
                      </li>
                    ))}
                  </ul>
                  {isDraft && (
                    <div className="mt-2">
                      <AddResourceDialog unitId={unit.id} onDone={() => router.refresh()} />
                    </div>
                  )}
                </div>
              ))}

              {isDraft && (
                <form
                  className="flex gap-2"
                  onSubmit={(e) => {
                    e.preventDefault();
                    const title = unitDrafts[mod.id]?.trim();
                    if (!title) return;
                    run(async () => {
                      const res = await createUnitAction(mod.id, title);
                      if (!res.error) setUnitDrafts((p) => ({ ...p, [mod.id]: "" }));
                      return res;
                    });
                  }}
                >
                  <Input
                    placeholder="Nueva unidad…"
                    value={unitDrafts[mod.id] ?? ""}
                    onChange={(e) =>
                      setUnitDrafts((p) => ({ ...p, [mod.id]: e.target.value }))
                    }
                  />
                  <Button type="submit" size="sm" variant="outline" disabled={pending}>
                    <Plus className="h-4 w-4" />
                  </Button>
                </form>
              )}
            </CardBody>
          </Card>
        ))}

        {outline && outline.modules.length === 0 && (
          <Alert variant="info">Esta versión aún no tiene módulos.</Alert>
        )}

        {isDraft && (
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              const title = newModule.trim();
              if (!title) return;
              run(async () => {
                const res = await createModuleAction(activeVersion!.id, title);
                if (!res.error) setNewModule("");
                return res;
              });
            }}
          >
            <Input
              placeholder="Nuevo módulo…"
              value={newModule}
              onChange={(e) => setNewModule(e.target.value)}
            />
            <Button type="submit" variant="outline" disabled={pending}>
              <Plus className="h-5 w-5" /> Módulo
            </Button>
          </form>
        )}
      </section>
    </main>
  );
}
