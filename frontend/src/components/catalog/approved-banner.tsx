import { Award, PartyPopper } from "lucide-react";

import { ButtonLink } from "@/components/ui/button";
import { formatDate } from "@/lib/utils";
import type { Badge } from "@/lib/api/types";

/**
 * Felicitacion en la ficha del curso para un estudiante que ya lo aprobo.
 *
 * La insignia la emite un worker de forma asincrona justo despues de la
 * aprobacion, asi que al llegar desde el reproductor puede no existir todavia:
 * en ese caso se manda a "Mis insignias", donde aparecera en segundos.
 */
export function ApprovedBanner({
  approvedAt,
  badge,
}: {
  approvedAt: string | null;
  badge?: Badge;
}) {
  const liveBadge = badge && !badge.revoked_at ? badge : undefined;

  return (
    <section
      aria-labelledby="curso-aprobado"
      className="mb-8 flex flex-col items-center gap-4 rounded-[var(--radius-lg)] border border-[var(--color-success)] bg-[color-mix(in_srgb,var(--color-success)_8%,var(--color-surface))] p-8 text-center elevate-sm"
    >
      <span className="flex h-16 w-16 items-center justify-center rounded-full bg-[color-mix(in_srgb,var(--color-success)_16%,transparent)] text-[var(--color-success)]">
        <PartyPopper className="h-8 w-8" aria-hidden />
      </span>
      <div className="space-y-1">
        <h2 id="curso-aprobado" className="text-2xl font-bold tracking-tight">
          ¡Felicitaciones, aprobaste este curso!
        </h2>
        <p className="text-[var(--color-text-muted)]">
          Completaste todo el contenido
          {approvedAt ? ` y quedaste aprobado el ${formatDate(approvedAt)}` : ""}.
        </p>
      </div>
      <div className="flex flex-wrap justify-center gap-3">
        {liveBadge ? (
          <>
            <ButtonLink href={`/badges/verify/${encodeURIComponent(liveBadge.public_code)}`}>
              <Award className="h-5 w-5" aria-hidden /> Ver mi insignia
            </ButtonLink>
            <ButtonLink href="/learn/badges" variant="secondary">
              Mis insignias
            </ButtonLink>
          </>
        ) : (
          <ButtonLink href="/learn/badges">
            <Award className="h-5 w-5" aria-hidden /> Ver mis insignias
          </ButtonLink>
        )}
      </div>
      {!badge && (
        <p className="text-sm text-[var(--color-text-muted)]">
          Tu insignia se está emitiendo; si aún no aparece, recarga “Mis insignias” en unos
          segundos.
        </p>
      )}
    </section>
  );
}
