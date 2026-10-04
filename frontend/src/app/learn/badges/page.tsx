import { Award, ExternalLink } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { listMyBadges } from "@/lib/api/learning";
import { Card, CardBody, CardFooter } from "@/components/ui/card";
import { Tag } from "@/components/ui/badge";
import { Alert } from "@/components/ui/alert";
import { formatDate } from "@/lib/utils";

export default async function MyBadgesPage() {
  await requireUser("/learn/badges");
  const badges = await listMyBadges();

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-5xl space-y-6 px-4 py-10">
        <h1 className="text-4xl font-extrabold uppercase">Mis insignias</h1>

        {badges.length === 0 ? (
          <Alert variant="info">
            Aún no tienes insignias. Aprueba un curso para obtener la primera.
          </Alert>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {badges.map((b) => (
              <Card key={b.id} className={b.revoked_at ? "opacity-60" : ""}>
                <CardBody className="space-y-3 text-center">
                  <span className="mx-auto flex h-20 w-20 items-center justify-center border-[3px] border-ink bg-accent-yellow">
                    <Award className="h-10 w-10" />
                  </span>
                  <h2 className="text-lg font-extrabold uppercase leading-tight">
                    {b.course_title}
                  </h2>
                  <p className="text-xs font-bold opacity-70">
                    Emitida {formatDate(b.issued_at)}
                  </p>
                  {b.revoked_at && <Tag tone="danger">revocada</Tag>}
                </CardBody>
                <CardFooter className="text-center">
                  <a
                    href={b.verify_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 font-bold uppercase underline decoration-2"
                  >
                    Verificar <ExternalLink className="h-4 w-4" />
                  </a>
                </CardFooter>
              </Card>
            ))}
          </div>
        )}
      </main>
    </>
  );
}
