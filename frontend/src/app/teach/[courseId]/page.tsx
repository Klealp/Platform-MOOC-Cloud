import { notFound } from "next/navigation";

import { Navbar } from "@/components/layout/navbar";
import { requireTeacher } from "@/lib/auth/guards";
import {
  getCourse,
  getVersionOutline,
  listVersions,
  type VersionOutline,
} from "@/lib/api/authoring";
import { ApiRequestError } from "@/lib/api/server-client";
import { CourseEditor } from "@/components/teach/course-editor";

export default async function CourseEditorPage({
  params,
}: {
  params: Promise<{ courseId: string }>;
}) {
  const { courseId } = await params;
  await requireTeacher(`/teach/${courseId}`);

  let course;
  try {
    course = await getCourse(courseId);
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 404) notFound();
    throw err;
  }

  const versions = await listVersions(courseId);
  const draft = versions.find((v) => v.status === "draft");
  const current = versions.find((v) => v.status === "published");

  // Preferimos editar el borrador; si no hay, mostramos la version vigente.
  const editable = draft ?? current ?? versions[0];
  let outline: VersionOutline | null = null;
  if (editable) {
    outline = await getVersionOutline(editable.id);
  }

  return (
    <>
      <Navbar />
      <CourseEditor
        course={course}
        versions={versions}
        activeVersion={editable ?? null}
        outline={outline}
      />
    </>
  );
}
