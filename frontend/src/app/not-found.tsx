import Link from "next/link";
import { SearchX } from "lucide-react";

import { Button } from "@/components/ui/button";

/**
 * Página 404 global. Next.js la muestra cuando un Server Component llama a
 * notFound(). En esta plataforma también cubre el acceso a contenido ajeno:
 * el backend responde 404 (no 403) a un curso/recurso de otro propietario para
 * no confirmar su existencia, y ese 404 se traduce aquí.
 */
export default function NotFound() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-accent-yellow p-4 text-center">
      <SearchX className="h-20 w-20" />
      <h1 className="border-[3px] border-ink bg-[var(--card)] px-6 py-3 text-4xl font-extrabold uppercase brutal-shadow-lg">
        404 — No encontrado
      </h1>
      <p className="max-w-md font-bold">
        La página o el recurso que buscas no existe o no está disponible para tu
        cuenta.
      </p>
      <div className="flex gap-2">
        <Link href="/dashboard">
          <Button variant="secondary">Ir al panel</Button>
        </Link>
        <Link href="/catalog">
          <Button variant="outline">Ver catálogo</Button>
        </Link>
      </div>
    </main>
  );
}
