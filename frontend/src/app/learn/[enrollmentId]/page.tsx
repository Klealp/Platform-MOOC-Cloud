import { notFound } from "next/navigation";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { getEnrollmentOutline } from "@/lib/api/learning";
import { ApiRequestError } from "@/lib/api/server-client";
import { CoursePlayer } from "@/components/learn/course-player";

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

  return (
    <>
      <Navbar />
      <CoursePlayer initial={outline} />
    </>
  );
}
