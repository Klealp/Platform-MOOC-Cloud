import { Navbar } from "@/components/layout/navbar";
import { requireAdmin } from "@/lib/auth/guards";
import { listAudit } from "@/lib/api/admin";
import { Tag } from "@/components/ui/badge";
import { Alert } from "@/components/ui/alert";

export default async function AdminAuditPage() {
  await requireAdmin("/admin/audit");
  const entries = await listAudit();

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-6xl space-y-6 px-4 py-10">
        <h1 className="text-4xl font-extrabold uppercase">Bitácora de auditoría</h1>

        {entries.length === 0 ? (
          <Alert variant="info">No hay registros de auditoría todavía.</Alert>
        ) : (
          <div className="space-y-2">
            {entries.map((e) => (
              <div
                key={e.id}
                className="brutal-border bg-[var(--card)] p-3 text-sm"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Tag tone="purple">{e.action}</Tag>
                  <span className="font-bold">{e.entity_type}</span>
                  <span className="font-mono text-xs opacity-60">{e.entity_id}</span>
                  <span className="ml-auto font-mono text-xs opacity-60">
                    {new Date(e.created_at).toLocaleString("es-CO")}
                  </span>
                </div>
                <div className="mt-1 flex flex-wrap gap-x-4 text-xs font-medium opacity-70">
                  <span>actor: {e.actor_id || "—"}</span>
                  <span>ip: {e.ip}</span>
                </div>
                {e.metadata && Object.keys(e.metadata).length > 0 && (
                  <pre className="mt-2 overflow-x-auto border-2 border-ink bg-ink p-2 font-mono text-xs text-brand-300">
                    {JSON.stringify(e.metadata, null, 2)}
                  </pre>
                )}
              </div>
            ))}
          </div>
        )}
      </main>
    </>
  );
}
