"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState, useTransition } from "react";
import { Search, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";

const LEVELS = [
  { v: "", l: "Todos los niveles" },
  { v: "beginner", l: "Principiante" },
  { v: "intermediate", l: "Intermedio" },
  { v: "advanced", l: "Avanzado" },
];

const LANGUAGES = [
  { v: "", l: "Todos los idiomas" },
  { v: "es", l: "Español" },
  { v: "en", l: "Inglés" },
  { v: "pt", l: "Portugués" },
];

/**
 * Filtros del catalogo. Escribe los parametros en la URL (?q=&level=...), de
 * modo que la pagina (Server Component) vuelve a pedir los datos filtrados al
 * backend. El filtrado real lo hace el servidor; esto solo navega.
 */
export function CatalogFilters() {
  const router = useRouter();
  const params = useSearchParams();
  const [pending, startTransition] = useTransition();

  const [q, setQ] = useState(params.get("q") ?? "");
  const [category, setCategory] = useState(params.get("category") ?? "");
  const [level, setLevel] = useState(params.get("level") ?? "");
  const [language, setLanguage] = useState(params.get("language") ?? "");

  function apply(next: Partial<Record<string, string>>) {
    const sp = new URLSearchParams();
    const merged = { q, category, level, language, ...next };
    for (const [k, v] of Object.entries(merged)) {
      if (v) sp.set(k, v);
    }
    startTransition(() => router.push(`/catalog?${sp.toString()}`));
  }

  function clear() {
    setQ("");
    setCategory("");
    setLevel("");
    setLanguage("");
    startTransition(() => router.push("/catalog"));
  }

  return (
    <form
      className="grid gap-3 brutal-border brutal-shadow-sm bg-[var(--card)] p-4 md:grid-cols-[1fr_auto_auto_auto_auto]"
      onSubmit={(e) => {
        e.preventDefault();
        apply({});
      }}
    >
      <div className="relative">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 opacity-50" />
        <Input
          className="pl-10"
          placeholder="Buscar cursos…"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          name="q"
        />
      </div>
      <Input
        placeholder="Categoría"
        value={category}
        onChange={(e) => setCategory(e.target.value)}
        className="md:w-36"
      />
      <Select value={level} onChange={(e) => { setLevel(e.target.value); apply({ level: e.target.value }); }}>
        {LEVELS.map((o) => (
          <option key={o.v} value={o.v}>{o.l}</option>
        ))}
      </Select>
      <Select value={language} onChange={(e) => { setLanguage(e.target.value); apply({ language: e.target.value }); }}>
        {LANGUAGES.map((o) => (
          <option key={o.v} value={o.v}>{o.l}</option>
        ))}
      </Select>
      <div className="flex gap-2">
        <Button type="submit" size="sm" disabled={pending}>
          Buscar
        </Button>
        <Button type="button" size="sm" variant="outline" onClick={clear} title="Limpiar">
          <X className="h-4 w-4" />
        </Button>
      </div>
    </form>
  );
}
