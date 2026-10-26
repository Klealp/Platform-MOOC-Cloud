import { Navbar } from "@/components/layout/navbar";
import { CardSkeleton, Skeleton } from "@/components/ui/skeleton";

/** Estado de carga del catálogo (skeletons mientras llegan los cursos). */
export default function CatalogLoading() {
  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1}
        className="mx-auto max-w-[1200px] space-y-8 px-6 py-10"
        aria-busy="true"
        aria-live="polite"
      >
        <span className="sr-only">Cargando el catálogo…</span>
        <Skeleton className="h-9 w-48" />
        <Skeleton className="h-14 w-full" />
        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      </main>
    </>
  );
}
