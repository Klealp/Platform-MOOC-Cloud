import Link from "next/link";
import { BookOpen, PencilRuler, ShieldCheck, Award, Layers } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { Card, CardBody } from "@/components/ui/card";
import { Tag } from "@/components/ui/badge";

/**
 * Panel principal. requireUser() se ejecuta en el servidor: si no hay sesion
 * valida, redirige a /login antes de renderizar nada. Las tarjetas por rol se
 * deciden tambien en el servidor segun user.role.
 */
export default async function DashboardPage() {
  const user = await requireUser("/dashboard");

  const studentCards = [
    { href: "/learn", Icon: BookOpen, t: "Mis cursos", d: "Continúa donde lo dejaste." },
    { href: "/catalog", Icon: Layers, t: "Explorar catálogo", d: "Inscríbete en nuevos cursos." },
    { href: "/learn/badges", Icon: Award, t: "Mis insignias", d: "Tus logros verificables." },
  ];
  const teacherCards = [
    { href: "/teach", Icon: PencilRuler, t: "Mis cursos", d: "Edita, versiona y publica." },
  ];
  const adminCards = [
    { href: "/admin/users", Icon: ShieldCheck, t: "Usuarios y roles", d: "Gestiona cuentas." },
    { href: "/admin/audit", Icon: Layers, t: "Auditoría", d: "Bitácora del sistema." },
    { href: "/admin/badges", Icon: Award, t: "Insignias", d: "Revocar insignias." },
  ];

  const cards =
    user.role === "admin"
      ? [...adminCards, ...teacherCards]
      : user.role === "teacher"
        ? teacherCards
        : studentCards;

  return (
    <>
      <Navbar />
      <main className="mx-auto max-w-6xl px-4 py-10">
        <div className="mb-8 flex flex-wrap items-center gap-3">
          <h1 className="text-3xl font-extrabold uppercase">
            Hola, {user.full_name.split(" ")[0]}
          </h1>
          <Tag tone="purple">{user.role}</Tag>
        </div>

        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {cards.map(({ href, Icon, t, d }) => (
            <Link key={href} href={href}>
              <Card interactive className="h-full">
                <CardBody className="space-y-3">
                  <span className="flex h-12 w-12 items-center justify-center border-[3px] border-ink bg-brand-500">
                    <Icon className="h-6 w-6" />
                  </span>
                  <h2 className="text-xl font-extrabold uppercase">{t}</h2>
                  <p className="text-sm font-medium opacity-80">{d}</p>
                </CardBody>
              </Card>
            </Link>
          ))}
        </div>
      </main>
    </>
  );
}
