import type { Metadata } from "next";
import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { listActiveSessions } from "@/lib/api/account";
import { SessionsManager } from "@/components/account/sessions-manager";

export const metadata: Metadata = { title: "Mis sesiones" };

/**
 * Gestión de sesiones activas del usuario: ver dónde tiene sesión abierta y
 * cerrarlas de forma remota. Punto de control de seguridad del enunciado.
 */
export default async function SessionsPage() {
  await requireUser("/account/sessions");
  const sessions = await listActiveSessions();

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-3xl space-y-6 px-6 py-10">
        <div className="space-y-1">
          <h1 className="text-3xl font-bold tracking-tight">Mis sesiones</h1>
          <p className="text-[var(--color-text-muted)]">
            Estas son las sesiones activas de tu cuenta. Cierra las que no
            reconozcas.
          </p>
        </div>
        <SessionsManager sessions={sessions} />
      </main>
    </>
  );
}
