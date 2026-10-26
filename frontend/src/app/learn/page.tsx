import type { Metadata } from "next";
import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { listEnrollments } from "@/lib/api/learning";
import { Card, CardBody, CardFooter } from "@/components/ui/card";
import { Tag, statusTone } from "@/components/ui/badge";
import { ProgressBar } from "@/components/ui/progress-bar";
import { ButtonLink } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { WithdrawButton } from "@/components/learn/withdraw-button";

export const metadata: Metadata = { title: "Mis cursos" };

export default async function MyLearningPage() {
  await requireUser("/learn");
  const enrollments = await listEnrollments();

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-5xl space-y-6 px-6 py-10">
        <h1 className="text-3xl font-bold tracking-tight">Mis cursos</h1>

        {enrollments.length === 0 ? (
          <Alert variant="info" title="Aún no te has inscrito">
            Explora el{" "}
            <Link href="/catalog" className="font-medium text-[var(--color-primary)] underline underline-offset-2">catálogo</Link>{" "}
            y empieza tu primer curso.
          </Alert>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2">
            {enrollments.map((e) => (
              <Card key={e.id} className="flex flex-col">
                <CardBody className="flex-1 space-y-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <Tag tone={statusTone(e.status)}>{e.status}</Tag>
                    {e.approved_at && <Tag tone="ok">aprobado</Tag>}
                  </div>
                  <h2 className="text-lg font-semibold leading-snug">
                    {e.title}
                  </h2>
                  <ProgressBar value={e.progress_pct} label={`Progreso en ${e.title}`} />
                </CardBody>
                <CardFooter className="flex flex-col items-stretch gap-2">
                  {e.status === "active" ? (
                    <>
                      <ButtonLink href={`/learn/${e.id}`} className="w-full" aria-label={`Continuar ${e.title}`}>
                        Continuar <ArrowRight className="h-4 w-4" aria-hidden />
                      </ButtonLink>
                      <WithdrawButton enrollmentId={e.id} />
                    </>
                  ) : (
                    <ButtonLink
                      href={`/catalog/${e.slug}`}
                      variant="outline"
                      className="w-full"
                      aria-label={`Reinscribirme en ${e.title}`}
                    >
                      Reinscribirme
                    </ButtonLink>
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
