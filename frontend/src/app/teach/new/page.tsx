import type { Metadata } from "next";
import { Navbar } from "@/components/layout/navbar";
import { requireTeacher } from "@/lib/auth/guards";
import { NewCourseForm } from "./new-course-form";

export const metadata: Metadata = { title: "Nuevo curso" };

export default async function NewCoursePage() {
  await requireTeacher("/teach/new");
  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-2xl space-y-6 px-6 py-10">
        <h1 className="text-3xl font-bold tracking-tight">Nuevo curso</h1>
        <NewCourseForm />
      </main>
    </>
  );
}
