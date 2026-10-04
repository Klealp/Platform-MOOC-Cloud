import { Navbar } from "@/components/layout/navbar";
import { requireTeacher } from "@/lib/auth/guards";
import { NewCourseForm } from "./new-course-form";

export default async function NewCoursePage() {
  await requireTeacher("/teach/new");
  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-2xl space-y-6 px-4 py-10">
        <h1 className="text-4xl font-extrabold uppercase">Nuevo curso</h1>
        <NewCourseForm />
      </main>
    </>
  );
}
