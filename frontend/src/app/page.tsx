import Link from "next/link";
import { ArrowRight, Award, BookOpen, Users } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { Button } from "@/components/ui/button";
import { Card, CardBody } from "@/components/ui/card";

export default function HomePage() {
  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-6xl px-4 py-12">
        <section className="grid items-center gap-8 md:grid-cols-2">
          <div className="space-y-6">
            <h1 className="text-5xl font-extrabold uppercase leading-none tracking-tight md:text-6xl">
              Aprende sin{" "}
              <span className="inline-block -rotate-1 border-[3px] border-ink bg-brand-500 px-2 brutal-shadow">
                límites
              </span>
            </h1>
            <p className="max-w-md text-lg font-medium">
              Cursos masivos abiertos en línea. Videos, lecturas, quizzes e
              insignias verificables.
            </p>
            <div className="flex flex-wrap gap-3">
              <Link href="/catalog">
                <Button size="lg">
                  Ver catálogo <ArrowRight className="h-5 w-5" />
                </Button>
              </Link>
              <Link href="/register">
                <Button size="lg" variant="secondary">
                  Crear cuenta
                </Button>
              </Link>
            </div>
          </div>

          <div className="grid gap-4">
            {[
              { Icon: BookOpen, t: "Contenido estructurado", d: "Módulos, unidades y recursos con progreso real." },
              { Icon: Award, t: "Insignias verificables", d: "Certifica tu avance con un código público." },
              { Icon: Users, t: "Para todos los roles", d: "Estudiantes, profesores y administradores." },
            ].map(({ Icon, t, d }, i) => (
              <Card key={i} className={i % 2 ? "rotate-1" : "-rotate-1"}>
                <CardBody className="flex items-start gap-4">
                  <span className="flex h-12 w-12 shrink-0 items-center justify-center border-[3px] border-ink bg-accent-yellow">
                    <Icon className="h-6 w-6" />
                  </span>
                  <div>
                    <h3 className="font-extrabold uppercase">{t}</h3>
                    <p className="text-sm font-medium opacity-80">{d}</p>
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
