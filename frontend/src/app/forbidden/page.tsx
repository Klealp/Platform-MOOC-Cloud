import Link from "next/link";
import { ShieldX } from "lucide-react";
import { Button } from "@/components/ui/button";

export default function ForbiddenPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-danger p-4 text-center">
      <ShieldX className="h-20 w-20 text-white" />
      <h1 className="border-[3px] border-ink bg-[var(--card)] px-6 py-3 text-4xl font-extrabold uppercase brutal-shadow-lg">
        403 — Sin permiso
      </h1>
      <p className="max-w-md font-bold text-white">
        Tu rol no permite acceder a esta sección. Esta comprobación la realiza
        el servidor.
      </p>
      <Link href="/dashboard">
        <Button variant="secondary">Volver al panel</Button>
      </Link>
    </main>
  );
}
