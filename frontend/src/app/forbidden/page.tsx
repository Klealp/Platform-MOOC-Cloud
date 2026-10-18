import Link from "next/link";
import { ShieldX } from "lucide-react";
import { Button } from "@/components/ui/button";

export default function ForbiddenPage() {
  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-[var(--color-bg)] p-4 text-center">
      <span className="flex h-20 w-20 items-center justify-center rounded-full bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] text-[var(--color-danger)]">
        <ShieldX className="h-10 w-10" />
      </span>
      <div className="space-y-2">
        <h1 className="text-3xl font-bold tracking-tight">403 — Sin permiso</h1>
        <p className="max-w-md text-[var(--color-text-muted)]">
          Tu rol no permite acceder a esta sección. Esta comprobación la realiza
          el servidor.
        </p>
      </div>
      <Link href="/dashboard">
        <Button variant="secondary">Volver al panel</Button>
      </Link>
    </main>
  );
}
