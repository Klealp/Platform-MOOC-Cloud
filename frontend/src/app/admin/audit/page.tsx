import type { Metadata } from "next";
import { Navbar } from "@/components/layout/navbar";
import { requireAdmin } from "@/lib/auth/guards";
import { AUDIT_MAX_LIMIT, listAudit } from "@/lib/api/admin";
import { AuditFilters } from "@/components/admin/audit-filters";
import { Tag } from "@/components/ui/badge";
import { Alert } from "@/components/ui/alert";

export const metadata: Metadata = { title: "Bitácora de auditoría" };

const DAY = /^\d{4}-\d{2}-\d{2}$/;

// Dia (YYYY-MM-DD) en la misma zona horaria con que se muestra la hora de
// cada linea, para que el filtro coincida con lo que el admin ve en pantalla.
function dayKey(iso: string) {
  return new Date(iso).toLocaleDateString("en-CA");
}

export default async function AdminAuditPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | undefined>>;
}) {
  await requireAdmin("/admin/audit");
  const sp = await searchParams;
  const action = sp.action ?? "";
  const from = sp.from && DAY.test(sp.from) ? sp.from : "";
  const to = sp.to && DAY.test(sp.to) ? sp.to : "";
  const byDate = Boolean(from || to);

  // El tipo lo filtra el backend (?action=). La fecha no: GET /admin/audit no
  // la acepta, asi que se piden las ultimas AUDIT_MAX_LIMIT lineas y se
  // recortan aqui.
  const raw = await listAudit({ action, limit: byDate ? AUDIT_MAX_LIMIT : undefined });
  const entries = byDate
    ? raw.filter((e) => {
        const d = dayKey(e.created_at);
        return (!from || d >= from) && (!to || d <= to);
      })
    : raw;
  // Si llego el tope, puede haber lineas mas antiguas que no se revisaron.
  const truncated = byDate && raw.length === AUDIT_MAX_LIMIT;
  const oldest = raw.length > 0 ? raw[raw.length - 1].created_at : "";
  const filtered = Boolean(action || byDate);

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-[1200px] space-y-6 px-6 py-10">
        <h1 className="text-3xl font-bold tracking-tight">Bitácora de auditoría</h1>
        <AuditFilters />

        {truncated && (
          <Alert variant="info">
            El filtro de fecha solo revisa los últimos {AUDIT_MAX_LIMIT} registros
            {action ? " de este tipo" : ""}, que llegan hasta el{" "}
            {new Date(oldest).toLocaleString("es-CO")}. Los anteriores no aparecen.
          </Alert>
        )}

        {entries.length === 0 ? (
          <Alert variant="info">
            {filtered
              ? "No hay registros que coincidan con los filtros."
              : "No hay registros de auditoría todavía."}
          </Alert>
        ) : (
          <div className="space-y-2">
            <p className="text-sm font-medium text-[var(--color-text-muted)]">
              {entries.length} {entries.length === 1 ? "registro" : "registros"}
            </p>
            {entries.map((e) => (
              <div
                key={e.id}
                className="rounded-[var(--radius-md)] border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-sm elevate-sm"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Tag tone="purple">{e.action}</Tag>
                  <span className="font-medium">{e.entity_type}</span>
                  <span className="font-mono text-xs text-[var(--color-text-muted)]">{e.entity_id}</span>
                  <span className="ml-auto font-mono text-xs text-[var(--color-text-muted)]">
                    {new Date(e.created_at).toLocaleString("es-CO")}
                  </span>
                </div>
                <div className="mt-1 flex flex-wrap gap-x-4 text-xs font-medium text-[var(--color-text-muted)]">
                  <span>actor: {e.actor_id || "—"}</span>
                  <span>ip: {e.ip}</span>
                </div>
                {e.metadata && Object.keys(e.metadata).length > 0 && (
                  <pre className="mt-2 overflow-x-auto rounded-[var(--radius-sm)] border border-[var(--color-border)] bg-[var(--color-surface-alt)] p-2 font-mono text-xs text-[var(--color-text)]">
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
