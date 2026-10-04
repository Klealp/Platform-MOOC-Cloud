import Link from "next/link";
import { ArrowRight } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { listEnrollments } from "@/lib/api/learning";
import { Card, CardBody, CardFooter } from "@/components/ui/card";
import { Tag, statusTone } from "@/components/ui/badge";
import { ProgressBar } from "@/components/ui/progress-bar";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";

export default async function MyLearningPage() {
  await requireUser("/learn");
  const enrollments = await listEnrollments();

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-5xl space-y-6 px-4 py-10">
        <h1 className="text-4xl font-extrabold uppercase">Mis cursos</h1>

        {enrollments.length === 0 ? (
          <Alert variant="info" title="Aún no te has inscrito">
            Explora el{" "}
            <Link href="/catalog" className="underline decoration-2">catálogo</Link>{" "}
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
                  <h2 className="text-xl font-extrabold uppercase leading-tight">
                    {e.title}
                  </h2>
                  <ProgressBar value={e.progress_pct} />
                </CardBody>
                <CardFooter>
                  {e.status === "active" ? (
                    <Link href={`/learn/${e.id}`}>
                      <Button className="w-full">
                        Continuar <ArrowRight className="h-4 w-4" />
                      </Button>
                    </Link>
                  ) : (
                    <Link href={`/catalog/${e.slug}`}>
                      <Button variant="outline" className="w-full">
                        Reinscribirme
                      </Button>
                    </Link>
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
