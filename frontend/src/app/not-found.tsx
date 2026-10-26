import { SearchX } from "lucide-react";

import { ButtonLink } from "@/components/ui/button";

/**
 * Página 404 global. Next.js la muestra cuando un Server Component llama a
 * notFound(). En esta plataforma también cubre el acceso a contenido ajeno:
 * el backend responde 404 (no 403) a un curso/recurso de otro propietario para
 * no confirmar su existencia, y ese 404 se traduce aquí.
 */
export default function NotFound() {
  return (
    <main id="contenido" tabIndex={-1} className="flex min-h-screen flex-col items-center justify-center gap-6 bg-[var(--color-bg)] p-4 text-center">
      <span className="flex h-20 w-20 items-center justify-center rounded-full bg-[var(--color-surface-alt)] text-[var(--color-text-muted)]">
        <SearchX className="h-10 w-10" />
      </span>
      <div className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight">404 — No encontrado</h1>
        <p className="max-w-md text-[var(--color-text-muted)]">
          La página o el recurso que buscas no existe o no está disponible para
          tu cuenta.
        </p>
      </div>
      <div className="flex gap-3">
        <ButtonLink href="/dashboard" variant="secondary">Ir al panel</ButtonLink>
        <ButtonLink href="/catalog" variant="ghost">Ver catálogo</ButtonLink>
      </div>
    </main>
  );
}
