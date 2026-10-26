import type { Metadata } from "next";
import Link from "next/link";
import { BookOpen, PencilRuler, ShieldCheck, Award, Layers, Monitor } from "lucide-react";

import { Navbar } from "@/components/layout/navbar";
import { requireUser } from "@/lib/auth/guards";
import { Card, CardBody } from "@/components/ui/card";
import { Tag } from "@/components/ui/badge";

export const metadata: Metadata = { title: "Panel" };

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

  // Disponible para cualquier rol: gestión de las sesiones propias.
  const accountCards = [
    {
      href: "/account/sessions",
      Icon: Monitor,
      t: "Mis sesiones",
      d: "Revisa y cierra sesiones activas.",
    },
  ];

  const cards =
    user.role === "admin"
      ? [...adminCards, ...teacherCards, ...accountCards]
      : user.role === "teacher"
        ? [...teacherCards, ...accountCards]
        : [...studentCards, ...accountCards];

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-[1200px] px-6 py-10">
        <div className="mb-8 flex flex-wrap items-center gap-3">
          <h1 className="text-3xl font-bold tracking-tight">
            Hola, {user.full_name.split(" ")[0]}
          </h1>
          <Tag tone="brand">{user.role}</Tag>
        </div>

        <div className="grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {cards.map(({ href, Icon, t, d }) => (
            <Link key={href} href={href}>
              <Card interactive className="h-full">
                <CardBody className="space-y-3">
                  <span className="flex h-11 w-11 items-center justify-center rounded-md bg-[color-mix(in_srgb,var(--color-primary)_12%,transparent)] text-[var(--color-primary)]">
                    <Icon className="h-6 w-6" />
                  </span>
                  <h2 className="text-lg font-semibold">{t}</h2>
                  <p className="text-sm text-[var(--color-text-muted)]">{d}</p>
                </CardBody>
              </Card>
            </Link>
          ))}
        </div>
      </main>
    </>
  );
}
