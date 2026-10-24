import type { Metadata } from "next";
import { Navbar } from "@/components/layout/navbar";
import { requireAdmin } from "@/lib/auth/guards";
import { RevokeBadgeTool } from "@/components/admin/revoke-badge-tool";
import { Alert } from "@/components/ui/alert";

export const metadata: Metadata = { title: "Insignias (administración)" };

export default async function AdminBadgesPage() {
  await requireAdmin("/admin/badges");

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-2xl space-y-6 px-6 py-10">
        <h1 className="text-3xl font-bold tracking-tight">Insignias</h1>
        <Alert variant="info">
          El backend permite revocar una insignia por su ID. Puedes verificar
          una insignia por su código público antes de revocarla.
        </Alert>
        <RevokeBadgeTool />
      </main>
    </>
  );
}
