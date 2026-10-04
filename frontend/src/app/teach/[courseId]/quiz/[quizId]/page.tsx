import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireTeacher } from "@/lib/auth/guards";
import { apiFetch, ApiRequestError } from "@/lib/api/server-client";
import { QuizBuilder, type AuthorQuiz } from "@/components/teach/quiz-builder";

export default async function QuizBuilderPage({
  params,
}: {
  params: Promise<{ courseId: string; quizId: string }>;
}) {
  const { courseId, quizId } = await params;
  await requireTeacher(`/teach/${courseId}/quiz/${quizId}`);

  let quiz: AuthorQuiz;
  try {
    quiz = await apiFetch<AuthorQuiz>(`/quizzes/${encodeURIComponent(quizId)}`);
  } catch (err) {
    if (err instanceof ApiRequestError && err.status === 404) notFound();
    throw err;
  }

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-4xl space-y-6 px-4 py-10">
        <Link
          href={`/teach/${courseId}`}
          className="inline-flex items-center gap-1 text-sm font-bold uppercase underline decoration-2"
        >
          <ArrowLeft className="h-4 w-4" /> Volver al curso
        </Link>
        <h1 className="text-3xl font-extrabold uppercase">Constructor de quiz</h1>
        <QuizBuilder quiz={quiz} />
      </main>
    </>
  );
}
