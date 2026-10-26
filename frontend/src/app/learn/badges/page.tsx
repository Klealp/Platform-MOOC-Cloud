import type { Metadata } from "next";
import Link from "next/link";
import { Award, ExternalLink } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { listMyBadges } from "@/lib/api/learning";
import { Card, CardBody, CardFooter } from "@/components/ui/card";
import { Tag } from "@/components/ui/badge";
import { Alert } from "@/components/ui/alert";
import { formatDate } from "@/lib/utils";
import { ShareBadgeLink } from "@/components/learn/share-badge-link";

export const metadata: Metadata = { title: "Mis insignias" };

export default async function MyBadgesPage() {
  await requireUser("/learn/badges");
  const badges = await listMyBadges();

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-5xl space-y-6 px-6 py-10">
        <h1 className="text-3xl font-bold tracking-tight">Mis insignias</h1>

        {badges.length === 0 ? (
          <Alert variant="info">
            Aún no tienes insignias. Aprueba un curso para obtener la primera.
          </Alert>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {badges.map((b) => (
              <Card
                key={b.id}
                // Revocada: borde punteado en vez de opacidad, que bajaria el
                // contraste del texto por debajo de 4.5:1; la etiqueta dice el estado.
                className={b.revoked_at ? "border-dashed" : ""}
              >
                <CardBody className="space-y-3 text-center">
                  <span className="mx-auto flex h-20 w-20 items-center justify-center rounded-full bg-[color-mix(in_srgb,var(--color-warning)_14%,transparent)] text-[var(--color-warning)]">
                    <Award className="h-10 w-10" />
                  </span>
                  <h2 className="text-lg font-semibold leading-snug">
                    {b.course_title}
                  </h2>
                  <p className="text-xs text-[var(--color-text-muted)]">
                    Emitida {formatDate(b.issued_at)}
                  </p>
                  {b.revoked_at && <Tag tone="danger">revocada</Tag>}
                </CardBody>
                <CardFooter className="text-center">
                  {b.revoked_at ? (
                    // Una insignia revocada no se ofrece para compartir; su
                    // pagina publica sigue indicando que fue revocada.
                    <Link
                      href={`/badges/verify/${b.public_code}`}
                      className="inline-flex items-center gap-1 text-sm font-medium text-[var(--color-primary)] underline underline-offset-2"
                    >
                      Ver estado <ExternalLink className="h-4 w-4" />
                    </Link>
                  ) : (
                    <ShareBadgeLink code={b.public_code} courseTitle={b.course_title} />
                  )}
                </CardFooter>
              </Card>
            ))}
          </div>
        )}
      </main>
    </>
  );
}
