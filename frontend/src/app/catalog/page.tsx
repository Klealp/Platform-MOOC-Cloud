import Link from "next/link";

import { Navbar } from "@/components/layout/navbar";
import { CatalogFilters } from "@/components/catalog/catalog-filters";
import { CourseCard } from "@/components/catalog/course-card";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { listCatalog, type CatalogFilters as Filters } from "@/lib/api/catalog";
import { ApiRequestError } from "@/lib/api/server-client";

export default async function CatalogPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | undefined>>;
}) {
  const sp = await searchParams;
  const filters: Filters = {
    q: sp.q,
    category: sp.category,
    level: sp.level,
    language: sp.language,
    tag: sp.tag,
    cursor: sp.cursor,
  };

  let error: string | null = null;
  let items: Awaited<ReturnType<typeof listCatalog>>["data"] = [];
  let nextCursor = "";

  try {
    const res = await listCatalog(filters);
    items = res.data;
    nextCursor = res.next_cursor;
  } catch (err) {
    error =
      err instanceof ApiRequestError
        ? err.message
        : "No se pudo cargar el catálogo. ¿Está el backend en marcha?";
  }

  function cursorHref(cursor: string) {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(filters)) if (v) p.set(k, String(v));
    p.set("cursor", cursor);
    return `/catalog?${p.toString()}`;
  }

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-6xl space-y-6 px-4 py-10">
        <h1 className="text-4xl font-extrabold uppercase">Catálogo</h1>
        <CatalogFilters />

        {error ? (
          <Alert variant="error" title="Error">{error}</Alert>
        ) : items.length === 0 ? (
          <Alert variant="info">No hay cursos que coincidan con tu búsqueda.</Alert>
        ) : (
          <>
            <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {items.map((c) => (
                <CourseCard key={c.id} course={c} />
              ))}
            </div>
            {nextCursor && (
              <div className="flex justify-center pt-4">
                <Link href={cursorHref(nextCursor)}>
                  <Button variant="secondary">Cargar más</Button>
                </Link>
              </div>
            )}
          </>
        )}
      </main>
    </>
  );
}
