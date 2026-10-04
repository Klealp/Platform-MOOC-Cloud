import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireTeacher } from "@/lib/auth/guards";
import { apiFetch, ApiRequestError } from "@/lib/api/server-client";
import type { ResourceContent } from "@/lib/api/types";
import { ContentEditor } from "@/components/teach/content-editor";

export default async function ResourceContentPage({
  params,
}: {
  params: Promise<{ courseId: string; resourceId: string }>;
}) {
  const { courseId, resourceId } = await params;
  await requireTeacher(`/teach/${courseId}/resource/${resourceId}`);

  let content: ResourceContent & { title?: string };
  try {
    content = await apiFetch<ResourceContent & { title?: string }>(
      `/resources/${encodeURIComponent(resourceId)}/content`,
    );
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 404) notFound();
    throw err;
  }

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-6xl space-y-6 px-4 py-10">
        <Link
          href={`/teach/${courseId}`}
          className="inline-flex items-center gap-1 text-sm font-bold uppercase underline decoration-2"
        >
          <ArrowLeft className="h-4 w-4" /> Volver al curso
        </Link>
        <ContentEditor
          resourceId={resourceId}
          initialContent={content.content_md ?? ""}
          initialRevision={content.revision ?? 0}
          title={content.title ?? "Editar contenido"}
        />
      </main>
    </>
  );
}
