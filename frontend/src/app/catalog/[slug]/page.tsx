import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { FileText, Film, HelpCircle, Music, Image as ImageIcon, Link2, FileDown } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Tag } from "@/components/ui/badge";
import { EnrollButton } from "@/components/catalog/enroll-button";
import { ApprovedBanner } from "@/components/catalog/approved-banner";
import { ButtonLink } from "@/components/ui/button";
import { getCatalogDetail } from "@/lib/api/catalog";
import { listEnrollments, listMyBadges } from "@/lib/api/learning";
import { getCurrentUser } from "@/lib/auth/session";
import { ApiRequestError } from "@/lib/api/server-client";
import type { Badge, Enrollment, ResourceType } from "@/lib/api/types";

export const metadata: Metadata = { title: "Curso" };

const typeIcon: Record<ResourceType, typeof FileText> = {
  rich_text: FileText,
  video: Film,
  audio: Music,
  image: ImageIcon,
  pdf: FileText,
  download: FileDown,
  external_link: Link2,
  quiz: HelpCircle,
};

export default async function CourseDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;

  let detail;
  try {
    detail = await getCatalogDetail(slug);
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 404) notFound();
    throw err;
  }

  const user = await getCurrentUser();

  // Con sesion, se busca la inscripcion del usuario en este curso para saber
  // si ya lo aprobo. Si falla, la ficha se muestra igual, sin felicitacion.
  let enrollment: Enrollment | undefined;
  let badge: Badge | undefined;
  if (user) {
    try {
      const [enrollments, badges] = await Promise.all([listEnrollments(), listMyBadges()]);
      enrollment = enrollments.find((e) => e.slug === detail.slug);
      badge = badges.find((b) => b.course_slug === detail.slug);
    } catch {
      enrollment = undefined;
    }
  }
  const approved = !!enrollment && (enrollment.state === "approved" || !!enrollment.approved_at);

  const totalResources = detail.outline.reduce(
    (acc, m) => acc + m.units.reduce((a, u) => a + u.resources.length, 0),
    0,
  );

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-[1200px] px-6 py-10">
        {approved && <ApprovedBanner approvedAt={enrollment!.approved_at} badge={badge} />}
        <div className="grid gap-8 lg:grid-cols-[1fr_340px]">
          <div className="space-y-6">
            <div className="flex flex-wrap gap-2">
              {detail.category && <Tag tone="blue">{detail.category}</Tag>}
              {detail.level && <Tag tone="yellow">{detail.level}</Tag>}
              <Tag tone="neutral">{detail.language}</Tag>
            </div>
            <h1 className="text-4xl font-bold leading-tight tracking-tight">
              {detail.title}
            </h1>
            <p className="text-lg text-[var(--color-text-muted)]">{detail.summary}</p>
            <p className="text-sm font-medium text-[var(--color-text-muted)]">
              Por {detail.teacher}
            </p>

            <section className="space-y-4">
              <h2 className="text-2xl font-semibold">Contenido</h2>
              {detail.outline.map((mod, i) => (
                <Card key={mod.id}>
                  <CardHeader>
                    <CardTitle>
                      {i + 1}. {mod.title}
                    </CardTitle>
                  </CardHeader>
                  <CardBody className="space-y-4">
                    {mod.units.map((unit) => (
                      <div key={unit.id}>
                        <p className="mb-2 font-medium">{unit.title}</p>
                        <ul className="space-y-1">
                          {unit.resources.map((r) => {
                            const Icon = typeIcon[r.type] ?? FileText;
                            return (
                              <li
                                key={r.id}
                                className="flex items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--color-border)] bg-[var(--color-surface-alt)] px-3 py-1.5 text-sm font-medium"
                              >
                                <Icon className="h-4 w-4 shrink-0" />
                                <span className="flex-1">{r.title}</span>
                                {r.required && <Tag tone="pink">obligatorio</Tag>}
                              </li>
                            );
                          })}
                        </ul>
                      </div>
                    ))}
                  </CardBody>
                </Card>
              ))}
            </section>
          </div>

          <aside className="lg:sticky lg:top-24 lg:self-start">
            <Card>
              <CardBody className="space-y-4">
                <dl className="space-y-2 text-sm font-medium">
                  <div className="flex justify-between">
                    <dt className="text-[var(--color-text-muted)]">Recursos</dt>
                    <dd>{totalResources}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-[var(--color-text-muted)]">Nota mínima</dt>
                    <dd>{detail.passing_score}%</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="text-[var(--color-text-muted)]">Avance exigido</dt>
                    <dd>{detail.required_completion}%</dd>
                  </div>
                </dl>
                {approved && enrollment!.status === "active" ? (
                  <ButtonLink href={`/learn/${enrollment!.id}`} variant="secondary" className="w-full">
                    Repasar el curso
                  </ButtonLink>
                ) : (
                  <EnrollButton slug={detail.slug} authenticated={!!user} />
                )}
              </CardBody>
            </Card>
          </aside>
        </div>
      </main>
    </>
  );
}
