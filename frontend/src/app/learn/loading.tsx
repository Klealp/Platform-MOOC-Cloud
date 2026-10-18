import { Navbar } from "@/components/layout/navbar";
import { CardSkeleton, Skeleton } from "@/components/ui/skeleton";

/** Estado de carga de "Mis cursos". */
export default function LearnLoading() {
  return (
    <>
      <Navbar />
      <main
        className="mx-auto max-w-5xl space-y-6 px-6 py-10"
        aria-busy="true"
        aria-live="polite"
      >
        <span className="sr-only">Cargando tus cursos…</span>
        <Skeleton className="h-9 w-48" />
        <div className="grid gap-5 sm:grid-cols-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <CardSkeleton key={i} />
          ))}
        </div>
      </main>
    </>
  );
}
