import type { Metadata } from "next";
import { Award, CheckCircle2, XCircle } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { Card, CardBody } from "@/components/ui/card";
import { Alert } from "@/components/ui/alert";
import { Tag } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button";
import { verifyBadge } from "@/lib/api/public";
import { ApiRequestError } from "@/lib/api/server-client";
import { formatDate } from "@/lib/utils";

export const metadata: Metadata = { title: "Verificar insignia" };

/**
 * Verificación PÚBLICA de una insignia. No requiere sesión: cualquiera con el
 * código puede comprobar la autenticidad de un logro, sin ver el correo del
 * titular. Es el destino legible del `verify_url` de cada insignia.
 */
export default async function BadgeVerifyPage({
  params,
}: {
  params: Promise<{ code: string }>;
}) {
  const { code } = await params;

  let badge;
  let notFound = false;
  try {
    badge = await verifyBadge(code);
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 404) {
      notFound = true;
    } else {
      throw err;
    }
  }

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-2xl space-y-6 px-6 py-12">
        <h1 className="text-center text-3xl font-bold tracking-tight">
          Verificación de insignia
        </h1>

        {notFound || !badge ? (
          <Alert variant="error" title="Código inválido">
            No existe una insignia con el código{" "}
            <span className="font-mono">{code}</span>.
          </Alert>
        ) : (
          <Card>
            <CardBody className="space-y-4 text-center">
              <span
                className="mx-auto flex h-24 w-24 items-center justify-center rounded-full"
                style={{
                  backgroundColor: badge.valid
                    ? "color-mix(in srgb, var(--color-warning) 14%, transparent)"
                    : "var(--color-surface-alt)",
                  color: badge.valid
                    ? "var(--color-warning)"
                    : "var(--color-text-muted)",
                }}
              >
                <Award className="h-12 w-12" />
              </span>

              {badge.valid ? (
                <div className="flex items-center justify-center gap-2 font-semibold text-[var(--color-success)]">
                  <CheckCircle2 className="h-5 w-5" /> Insignia válida
                </div>
              ) : (
                <div className="flex items-center justify-center gap-2 font-semibold text-[var(--color-danger)]">
                  <XCircle className="h-5 w-5" /> Insignia revocada
                </div>
              )}

              <dl className="space-y-2 text-left text-sm">
                <div className="flex justify-between gap-3">
                  <dt className="opacity-60">Titular</dt>
                  <dd className="text-right">{badge.recipient}</dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="opacity-60">Curso</dt>
                  <dd className="text-right">{badge.course}</dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="opacity-60">Emitida por</dt>
                  <dd className="text-right">{badge.issued_by}</dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="opacity-60">Fecha de emisión</dt>
                  <dd className="text-right">{formatDate(badge.issued_at)}</dd>
                </div>
                <div className="flex justify-between gap-3">
                  <dt className="opacity-60">Código</dt>
                  <dd className="text-right font-mono text-xs">
                    {badge.public_code}
                  </dd>
                </div>
              </dl>

              {!badge.valid && (
                <Alert variant="warning" title="Revocada">
                  {badge.revoke_reason
                    ? `Motivo: ${badge.revoke_reason}`
                    : "Esta insignia fue revocada y ya no es válida."}
                  {badge.revoked_at && (
                    <span className="mt-1 block">
                      <Tag tone="danger">{formatDate(badge.revoked_at)}</Tag>
                    </span>
                  )}
                </Alert>
              )}
            </CardBody>
          </Card>
        )}

        <div className="text-center">
          <ButtonLink href="/catalog" variant="outline">Ver catálogo de cursos</ButtonLink>
        </div>
      </main>
    </>
  );
}
