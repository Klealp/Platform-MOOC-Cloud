import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { listActiveSessions } from "@/lib/api/account";
import { SessionsManager } from "@/components/account/sessions-manager";

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
      <main className="mx-auto max-w-3xl space-y-6 px-4 py-10">
        <div className="space-y-1">
          <h1 className="text-4xl font-extrabold uppercase">Mis sesiones</h1>
          <p className="font-medium opacity-80">
            Estas son las sesiones activas de tu cuenta. Cierra las que no
            reconozcas.
          </p>
        </div>
        <SessionsManager sessions={sessions} />
      </main>
    </>
  );
}
