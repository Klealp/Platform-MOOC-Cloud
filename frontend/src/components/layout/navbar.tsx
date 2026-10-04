import Link from "next/link";
import { GraduationCap } from "lucide-react";

import { getCurrentUser } from "@/lib/auth/session";
import { Tag } from "@/components/ui/badge";
import { LogoutButton } from "@/components/auth/logout-button";

/**
 * Barra superior. Server Component: la identidad y el rol se resuelven en el
 * servidor con getCurrentUser(). Lo que ve el usuario depende de datos del
 * backend, no de estado del cliente.
 */
export async function Navbar() {
  const user = await getCurrentUser();

  return (
    <header className="sticky top-0 z-40 border-b-[3px] border-ink bg-brand-500">
      <nav className="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3">
        <Link href="/" className="flex items-center gap-2 font-extrabold uppercase tracking-tight">
          <span className="flex h-9 w-9 items-center justify-center border-[3px] border-ink bg-[var(--card)]">
            <GraduationCap className="h-5 w-5" />
          </span>
          <span className="hidden sm:inline">MOOC</span>
        </Link>

        <div className="flex items-center gap-2">
          <Link
            href="/catalog"
            className="border-2 border-ink bg-[var(--card)] px-3 py-1.5 text-sm font-bold uppercase brutal-press"
          >
            Catálogo
          </Link>

          {user ? (
            <>
              <Link
                href="/dashboard"
                className="border-2 border-ink bg-[var(--card)] px-3 py-1.5 text-sm font-bold uppercase brutal-press"
              >
                Panel
              </Link>
              <span className="hidden items-center gap-2 md:flex">
                <span className="text-sm font-bold">{user.full_name}</span>
                <Tag tone="purple">{user.role}</Tag>
              </span>
              <LogoutButton />
            </>
          ) : (
            <Link
              href="/login"
              className="border-2 border-ink bg-accent-yellow px-3 py-1.5 text-sm font-bold uppercase brutal-press"
            >
              Entrar
            </Link>
          )}
        </div>
      </nav>
    </header>
  );
}
