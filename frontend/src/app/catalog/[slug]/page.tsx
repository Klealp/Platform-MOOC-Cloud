import { notFound } from "next/navigation";
import { FileText, Film, HelpCircle, Music, Image as ImageIcon, Link2, FileDown } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Tag } from "@/components/ui/badge";
import { EnrollButton } from "@/components/catalog/enroll-button";
import { getCatalogDetail } from "@/lib/api/catalog";
import { getCurrentUser } from "@/lib/auth/session";
import { ApiRequestError } from "@/lib/api/server-client";
import type { ResourceType } from "@/lib/api/types";

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

  const totalResources = detail.outline.reduce(
    (acc, m) => acc + m.units.reduce((a, u) => a + u.resources.length, 0),
    0,
  );

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-6xl px-4 py-10">
        <div className="grid gap-8 lg:grid-cols-[1fr_340px]">
          <div className="space-y-6">
            <div className="flex flex-wrap gap-2">
              {detail.category && <Tag tone="blue">{detail.category}</Tag>}
              {detail.level && <Tag tone="yellow">{detail.level}</Tag>}
              <Tag tone="neutral">{detail.language}</Tag>
            </div>
            <h1 className="text-4xl font-extrabold uppercase leading-none">
              {detail.title}
            </h1>
            <p className="text-lg font-medium">{detail.summary}</p>
            <p className="text-sm font-bold opacity-70">
              Por {detail.teacher}
            </p>

            <section className="space-y-4">
              <h2 className="text-2xl font-extrabold uppercase">Contenido</h2>
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
                        <p className="mb-2 font-bold">{unit.title}</p>
                        <ul className="space-y-1">
                          {unit.resources.map((r) => {
                            const Icon = typeIcon[r.type] ?? FileText;
                            return (
                              <li
                                key={r.id}
                                className="flex items-center gap-2 border-2 border-ink bg-brand-50 px-3 py-1.5 text-sm font-medium"
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
                <dl className="space-y-2 text-sm font-bold">
                  <div className="flex justify-between">
                    <dt className="opacity-60">Recursos</dt>
                    <dd>{totalResources}</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="opacity-60">Nota mínima</dt>
                    <dd>{detail.passing_score}%</dd>
                  </div>
                  <div className="flex justify-between">
                    <dt className="opacity-60">Avance exigido</dt>
                    <dd>{detail.required_completion}%</dd>
                  </div>
                </dl>
                <EnrollButton slug={detail.slug} authenticated={!!user} />
              </CardBody>
            </Card>
          </aside>
        </div>
      </main>
    </>
  );
}
