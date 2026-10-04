import Link from "next/link";
import { redirect } from "next/navigation";

import { getCurrentUser } from "@/lib/auth/session";
import { LoginForm } from "./login-form";

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
    <main className="flex min-h-screen items-center justify-center bg-brand-100 p-4">
      <div className="w-full max-w-md">
        <div className="mb-6 text-center">
          <h1 className="inline-block border-[3px] border-ink bg-accent-yellow px-4 py-2 text-3xl font-extrabold uppercase brutal-shadow">
            Iniciar sesión
          </h1>
        </div>
        <LoginForm from={from} />
        <p className="mt-4 text-center text-sm font-medium">
          ¿No tienes cuenta?{" "}
          <Link href="/register" className="font-bold underline decoration-[3px]">
            Regístrate
          </Link>
        </p>
      </div>
    </main>
  );
}
