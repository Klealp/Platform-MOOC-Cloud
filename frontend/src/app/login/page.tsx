import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { getCurrentUser } from "@/lib/auth/session";
import { LoginForm } from "./login-form";

export const metadata: Metadata = { title: "Iniciar sesión" };

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ from?: string }>;
}) {
  // Si ya hay sesion, no tiene sentido mostrar el login.
  const user = await getCurrentUser();
  if (user) redirect("/dashboard");

  const { from } = await searchParams;

  return (
    <main id="contenido" tabIndex={-1} className="flex min-h-screen items-center justify-center bg-[var(--color-bg)] p-4">
      <div className="w-full max-w-md">
        <div className="mb-8 text-center">
          <h1 className="text-3xl font-bold tracking-tight">Iniciar sesión</h1>
          <p className="mt-2 text-[var(--color-text-muted)]">
            Accede para continuar tu aprendizaje.
          </p>
        </div>
        <LoginForm from={from} />
        <p className="mt-6 text-center text-sm text-[var(--color-text-muted)]">
          ¿No tienes cuenta?{" "}
          <Link
            href="/register"
            className="font-medium text-[var(--color-primary)] underline underline-offset-2"
          >
            Regístrate
          </Link>
        </p>
      </div>
    </main>
  );
}
