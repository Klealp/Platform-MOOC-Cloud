import Link from "next/link";
import { Plus, Users } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireTeacher } from "@/lib/auth/guards";
import { listMyCourses } from "@/lib/api/authoring";
import { Card, CardBody, CardFooter } from "@/components/ui/card";
import { Tag, statusTone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { formatDate } from "@/lib/utils";

export default async function TeachHomePage() {
  await requireTeacher("/teach");
  const courses = await listMyCourses();

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-5xl space-y-6 px-4 py-10">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-4xl font-extrabold uppercase">Mis cursos</h1>
          <Link href="/teach/new">
            <Button>
              <Plus className="h-5 w-5" /> Nuevo curso
            </Button>
          </Link>
        </div>

        {courses.length === 0 ? (
          <Alert variant="info">
            Aún no has creado cursos. Empieza con “Nuevo curso”.
          </Alert>
        ) : (
          <div className="grid gap-5 sm:grid-cols-2">
            {courses.map((c) => (
              <Link key={c.id} href={`/teach/${c.id}`}>
                <Card interactive className="flex h-full flex-col">
                  <CardBody className="flex-1 space-y-2">
                    <Tag tone={statusTone(c.status)}>{c.status}</Tag>
                    <h2 className="text-xl font-extrabold uppercase leading-tight">
                      {c.title}
                    </h2>
                    {c.category && (
                      <p className="text-sm font-bold opacity-60">{c.category}</p>
                    )}
                  </CardBody>
                  <CardFooter className="flex items-center justify-between text-sm font-bold">
                    <span className="flex items-center gap-1">
                      <Users className="h-4 w-4" /> {c.active_enrollments ?? 0}
                    </span>
                    <span className="opacity-60">{formatDate(c.updated_at)}</span>
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
