"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useTransition } from "react";
import { X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";

// Espejo de las constantes Action* de internal/audit/audit.go. Si se agrega
// una accion en el backend, agregarla aqui para poder filtrar por ella.
const ACTION_GROUPS: { g: string; items: { v: string; l: string }[] }[] = [
  {
    g: "Usuarios",
    items: [
      { v: "user.registered", l: "Registro" },
      { v: "user.email_verified", l: "Correo verificado" },
      { v: "user.login", l: "Inicio de sesión" },
      { v: "user.login_failed", l: "Inicio de sesión fallido" },
      { v: "user.logout", l: "Cierre de sesión" },
      { v: "user.password_reset", l: "Cambio de contraseña" },
    ],
  },
  {
    g: "Administración",
    items: [
      { v: "admin.role_changed", l: "Cambio de rol" },
      { v: "admin.status_changed", l: "Cambio de estado" },
      { v: "admin.teacher_created", l: "Profesor creado" },
      { v: "admin.sessions_revoked", l: "Sesiones revocadas" },
    ],
  },
  {
    g: "Cursos",
    items: [
      { v: "course.published", l: "Curso publicado" },
      { v: "course.unpublished", l: "Curso despublicado" },
      { v: "course.version_created", l: "Versión creada" },
    ],
  },
  {
    g: "Archivos",
    items: [
      { v: "asset.upload_initiated", l: "Subida iniciada" },
      { v: "asset.upload_completed", l: "Subida completada" },
      { v: "asset.ready", l: "Archivo listo" },
      { v: "asset.infected", l: "Archivo infectado" },
    ],
  },
  {
    g: "Aprendizaje",
    items: [
      { v: "enrollment.created", l: "Inscripción" },
      { v: "enrollment.withdrawn", l: "Retiro" },
      { v: "progress.rejected", l: "Progreso rechazado" },
      { v: "quiz.submitted", l: "Quiz enviado" },
      { v: "badge.issued", l: "Insignia emitida" },
      { v: "badge.revoked", l: "Insignia revocada" },
    ],
  },
  {
    g: "Sistema",
    items: [{ v: "job.dead_lettered", l: "Trabajo agotó reintentos" }],
  },
];

/**
 * Filtros de la bitacora: tipo de accion y rango de fechas (YYYY-MM-DD).
 * Escribe los parametros en la URL; la pagina hace el filtrado en el servidor.
 *
 * Los desplegables no navegan al cambiar: con teclado, cada flecha dispararia
 * una recarga (WCAG 3.2.2). Todo se aplica con el boton.
 */
export function AuditFilters() {
  const router = useRouter();
  const params = useSearchParams();
  const [pending, startTransition] = useTransition();

  const [action, setAction] = useState(params.get("action") ?? "");
  const [from, setFrom] = useState(params.get("from") ?? "");
  const [to, setTo] = useState(params.get("to") ?? "");

  function apply(next: Partial<Record<string, string>>) {
    const sp = new URLSearchParams();
    const merged = { action, from, to, ...next };
    for (const [k, v] of Object.entries(merged)) {
      if (v) sp.set(k, v);
    }
    startTransition(() => router.push(`/admin/audit?${sp.toString()}`));
  }

  function clear() {
    setAction("");
    setFrom("");
    setTo("");
    startTransition(() => router.push("/admin/audit"));
  }

  return (
    <form
      role="search"
      aria-label="Filtrar la bitácora"
      className="grid items-end gap-3 rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 elevate-sm md:grid-cols-[1fr_auto_auto_auto]"
      onSubmit={(e) => {
        e.preventDefault();
        apply({});
      }}
    >
      <label className="text-sm font-medium">
        <span className="mb-1.5 block">Tipo</span>
        <Select value={action} onChange={(e) => setAction(e.target.value)}>
          <option value="">Todos los tipos</option>
          {ACTION_GROUPS.map((group) => (
            <optgroup key={group.g} label={group.g}>
              {group.items.map((o) => (
                <option key={o.v} value={o.v}>
                  {o.l} ({o.v})
                </option>
              ))}
            </optgroup>
          ))}
        </Select>
      </label>
      <label className="text-sm font-medium">
        <span className="mb-1.5 block">Desde</span>
        <Input type="date" value={from} max={to || undefined} onChange={(e) => setFrom(e.target.value)} />
      </label>
      <label className="text-sm font-medium">
        <span className="mb-1.5 block">Hasta</span>
        <Input type="date" value={to} min={from || undefined} onChange={(e) => setTo(e.target.value)} />
      </label>
      <div className="flex gap-2 pb-0.5">
        <Button type="submit" size="sm" disabled={pending}>
          Filtrar
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={clear} title="Limpiar filtros" aria-label="Limpiar filtros">
          <X className="h-4 w-4" aria-hidden />
        </Button>
      </div>
    </form>
  );
}
