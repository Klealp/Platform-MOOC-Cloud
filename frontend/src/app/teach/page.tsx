import type { Metadata } from "next";
import Link from "next/link";
import { Plus, Users } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { CourseFilters } from "@/components/teach/course-filters";
import { requireTeacher } from "@/lib/auth/guards";
import { listMyCourses } from "@/lib/api/authoring";
import { Card, CardBody, CardFooter } from "@/components/ui/card";
import { Tag, statusTone } from "@/components/ui/badge";
import { ButtonLink } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { formatDate } from "@/lib/utils";

export const metadata: Metadata = { title: "Mis cursos (autoría)" };

// Minusculas y sin tildes, para que "programacion" encuentre "Programación".
function normalize(s: string | undefined) {
  return (s ?? "").normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase().trim();
}

export default async function TeachHomePage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | undefined>>;
}) {
  await requireTeacher("/teach");
  const sp = await searchParams;
  const all = await listMyCourses();

  // GET /courses no acepta filtros: devuelve todo lo visible para el usuario
  // (un admin ve todos los cursos) y aqui se filtra en el servidor de Next.
  const q = normalize(sp.q);
  const category = normalize(sp.category);
  const status = sp.status ?? "";
  const courses = all.filter(
    (c) =>
      (!q || normalize(`${c.title} ${c.slug} ${c.category ?? ""}`).includes(q)) &&
      (!category || normalize(c.category).includes(category)) &&
      (!status || c.status === status),
  );

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-5xl space-y-6 px-6 py-10">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-3xl font-bold tracking-tight">Mis cursos</h1>
          <ButtonLink href="/teach/new">
              <Plus className="h-5 w-5" /> Nuevo curso
            </ButtonLink>
        </div>

        {all.length > 0 && <CourseFilters />}

        {all.length === 0 ? (
          <Alert variant="info">
            Aún no has creado cursos. Empieza con “Nuevo curso”.
          </Alert>
        ) : courses.length === 0 ? (
          <Alert variant="info">No hay cursos que coincidan con tu búsqueda.</Alert>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2">
            {courses.map((c) => (
              <Link key={c.id} href={`/teach/${c.id}`}>
                <Card interactive className="flex h-full flex-col">
                  <CardBody className="flex-1 space-y-2">
                    <Tag tone={statusTone(c.status)}>{c.status}</Tag>
                    <h2 className="text-xl font-semibold leading-tight">
                      {c.title}
                    </h2>
                    {c.category && (
                      <p className="text-sm font-medium text-[var(--color-text-muted)]">{c.category}</p>
                    )}
                  </CardBody>
                  <CardFooter className="flex items-center justify-between text-sm font-medium">
                    <span className="flex items-center gap-1">
                      <Users className="h-4 w-4" aria-hidden /> {c.active_enrollments ?? 0}
                      <span className="sr-only">inscritos activos</span>
                    </span>
                    <span className="text-[var(--color-text-muted)]">{formatDate(c.updated_at)}</span>
                  </CardFooter>
                </Card>
              </Link>
            ))}
          </div>
        )}
      </main>
    </>
  );
}
