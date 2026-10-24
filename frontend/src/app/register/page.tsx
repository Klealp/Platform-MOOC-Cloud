import type { Metadata } from "next";
import Link from "next/link";
import { redirect } from "next/navigation";

import { getCurrentUser } from "@/lib/auth/session";
import { RegisterForm } from "./register-form";

export const metadata: Metadata = { title: "Crear cuenta" };

export default async function RegisterPage() {
  const user = await getCurrentUser();
  if (user) redirect("/dashboard");

  return (
    <main id="contenido" tabIndex={-1} className="flex min-h-screen items-center justify-center bg-[var(--color-bg)] p-4">
      <div className="w-full max-w-md">
        <div className="mb-8 text-center">
          <h1 className="text-3xl font-bold tracking-tight">Crear cuenta</h1>
          <p className="mt-2 text-[var(--color-text-muted)]">
            Te registras como estudiante.
          </p>
        </div>
        <RegisterForm />
        <p className="mt-6 text-center text-sm text-[var(--color-text-muted)]">
          ¿Ya tienes cuenta?{" "}
          <Link
            href="/login"
            className="font-medium text-[var(--color-primary)] underline underline-offset-2"
          >
            Inicia sesión
          </Link>
        </p>
      </div>
    </main>
  );
}
