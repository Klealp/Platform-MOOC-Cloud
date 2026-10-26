import { ArrowRight, Award, BookOpen, Users } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { ButtonLink } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";

export default function HomePage() {
  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-[1200px] px-6 py-16">
        <section className="grid items-center gap-12 md:grid-cols-2">
          <div className="space-y-6">
            <h1 className="text-4xl font-bold leading-tight tracking-tight md:text-5xl">
              Aprende sin{" "}
              <span className="text-[var(--color-primary)]">límites</span>
            </h1>
            <p className="max-w-md text-lg text-[var(--color-text-muted)]">
              Cursos masivos abiertos en línea. Videos, lecturas, quizzes e
              insignias verificables.
            </p>
            <div className="flex flex-wrap gap-3">
              <ButtonLink href="/catalog" size="lg">
                  Ver catálogo <ArrowRight className="h-5 w-5" />
                </ButtonLink>
              <ButtonLink href="/register" size="lg" variant="secondary">
                  Crear cuenta
                </ButtonLink>
            </div>
          </div>

          <div className="grid gap-4">
            {[
              { Icon: BookOpen, t: "Contenido estructurado", d: "Módulos, unidades y recursos con progreso real." },
              { Icon: Award, t: "Insignias verificables", d: "Certifica tu avance con un código público." },
              { Icon: Users, t: "Para todos los roles", d: "Estudiantes, profesores y administradores." },
            ].map(({ Icon, t, d }, i) => (
              <Card key={i}>
                <CardBody className="flex items-start gap-4">
                  <span className="flex h-11 w-11 shrink-0 items-center justify-center rounded-md bg-[color-mix(in_srgb,var(--color-primary)_12%,transparent)] text-[var(--color-primary)]">
                    <Icon className="h-6 w-6" />
                  </span>
                  <div>
                    <h2 className="font-semibold">{t}</h2>
                    <p className="mt-1 text-sm text-[var(--color-text-muted)]">{d}</p>
                  </div>
                </CardBody>
              </Card>
            ))}
          </div>
        </section>
      </main>
    </>
  );
}
