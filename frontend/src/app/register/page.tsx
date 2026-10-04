import Link from "next/link";
import { redirect } from "next/navigation";

import { getCurrentUser } from "@/lib/auth/session";
import { RegisterForm } from "./register-form";

export default async function RegisterPage() {
  const user = await getCurrentUser();
  if (user) redirect("/dashboard");

  return (
    <main className="flex min-h-screen items-center justify-center bg-accent-pink p-4">
      <div className="w-full max-w-md">
        <div className="mb-6 text-center">
          <h1 className="inline-block border-[3px] border-ink bg-brand-500 px-4 py-2 text-3xl font-extrabold uppercase brutal-shadow">
            Crear cuenta
          </h1>
          <p className="mt-2 text-sm font-bold">Te registras como estudiante.</p>
        </div>
        <RegisterForm />
        <p className="mt-4 text-center text-sm font-medium">
          ¿Ya tienes cuenta?{" "}
          <Link href="/login" className="font-bold underline decoration-[3px]">
            Inicia sesión
          </Link>
        </p>
      </div>
    </main>
  );
}
