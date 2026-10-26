"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useTransition } from "react";
import { Search, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";

const STATUSES = [
  { v: "", l: "Todos los estados" },
  { v: "draft", l: "Borrador" },
  { v: "published", l: "Publicado" },
  { v: "unpublished", l: "Despublicado" },
  { v: "archived", l: "Archivado" },
];

/**
 * Filtros de "Mis cursos" (/teach), con la misma forma que los del catalogo.
 * Escribe los parametros en la URL y la pagina filtra en el servidor.
 *
 * Los desplegables no navegan al cambiar: con teclado, cada flecha dispararia
 * una recarga (WCAG 3.2.2). Todo se aplica con el boton.
 *
 * GET /courses no devuelve nivel ni idioma, asi que aqui no se ofrecen esos
 * filtros; en su lugar esta el estado, que el catalogo no necesita.
 */
export function CourseFilters() {
  const router = useRouter();
  const params = useSearchParams();
  const [pending, startTransition] = useTransition();

  const [q, setQ] = useState(params.get("q") ?? "");
  const [category, setCategory] = useState(params.get("category") ?? "");
  const [status, setStatus] = useState(params.get("status") ?? "");

  function apply(next: Partial<Record<string, string>>) {
    const sp = new URLSearchParams();
    const merged = { q, category, status, ...next };
    for (const [k, v] of Object.entries(merged)) {
      if (v) sp.set(k, v);
    }
    startTransition(() => router.push(`/teach?${sp.toString()}`));
  }

  function clear() {
    setQ("");
    setCategory("");
    setStatus("");
    startTransition(() => router.push("/teach"));
  }

  return (
    <form
      role="search"
      aria-label="Filtrar cursos"
      className="grid gap-3 rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)] p-4 elevate-sm md:grid-cols-[1fr_auto_auto_auto]"
      onSubmit={(e) => {
        e.preventDefault();
        apply({});
      }}
    >
      <div className="relative">
        <Search aria-hidden className="pointer-events-none absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-[var(--color-text-muted)]" />
        <Input
          className="pl-10"
          placeholder="Buscar cursos…"
          aria-label="Buscar cursos"
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          name="q"
        />
      </div>
      <Input
        placeholder="Categoría"
        aria-label="Categoría"
        value={category}
        onChange={(e) => setCategory(e.target.value)}
        className="md:w-36"
      />
      <Select aria-label="Estado" value={status} onChange={(e) => setStatus(e.target.value)}>
        {STATUSES.map((o) => (
          <option key={o.v} value={o.v}>{o.l}</option>
        ))}
      </Select>
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={pending}>
          Buscar
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={clear} title="Limpiar filtros" aria-label="Limpiar filtros">
          <X className="h-4 w-4" aria-hidden />
        </Button>
      </div>
    </form>
  );
}
