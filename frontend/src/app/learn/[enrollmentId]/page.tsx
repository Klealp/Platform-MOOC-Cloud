import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { getEnrollmentOutline, listEnrollments } from "@/lib/api/learning";
import { ApiRequestError } from "@/lib/api/server-client";
import { CoursePlayer } from "@/components/learn/course-player";

export const metadata: Metadata = { title: "Curso" };

export default async function CoursePlayerPage({
  params,
}: {
  params: Promise<{ enrollmentId: string }>;
}) {
  const { enrollmentId } = await params;
  await requireUser(`/learn/${enrollmentId}`);

  let outline;
  try {
    outline = await getEnrollmentOutline(enrollmentId);
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 404) notFound();
    throw err;
  }
  // El outline no trae el slug del curso, que hace falta para llevar al
  // estudiante a la ficha del curso cuando lo aprueba.
  const enrollments = await listEnrollments();
  const courseSlug = enrollments.find((e) => e.id === enrollmentId)?.slug ?? "";

  return (
    <>
      <Navbar />
      <CoursePlayer initial={outline} courseSlug={courseSlug} />
    </>
  );
}
