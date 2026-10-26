import Link from "next/link";
import { GraduationCap } from "lucide-react";

import { getCurrentUser } from "@/lib/auth/session";
import { Tag } from "@/components/ui/badge";
import { LogoutButton } from "@/components/auth/logout-button";
import { ThemeToggle } from "@/components/layout/theme-toggle";

/**
 * Barra superior. Server Component: la identidad y el rol se resuelven en el
 * servidor con getCurrentUser(). Lo que ve el usuario depende de datos del
 * backend, no de estado del cliente.
 */
export async function Navbar() {
  const user = await getCurrentUser();

  const linkCls =
    "rounded-md px-3 py-1.5 text-sm font-medium text-[var(--color-text-muted)] transition-colors hover:bg-[var(--color-surface-alt)] hover:text-[var(--color-text)]";

  return (
    <header className="sticky top-0 z-40 border-b border-[var(--color-border)] bg-[var(--color-surface)]/90 backdrop-blur">
      <nav aria-label="Principal" className="mx-auto flex max-w-[1200px] items-center justify-between gap-4 px-6 py-3">
        <Link
          href="/"
          aria-label="MOOC, inicio"
          className="flex items-center gap-2 font-semibold tracking-tight text-[var(--color-text)]"
        >
          <span className="flex h-8 w-8 items-center justify-center rounded-md bg-[var(--color-primary)] text-[var(--color-on-primary)]">
            <GraduationCap className="h-5 w-5" aria-hidden />
          </span>
          <span className="hidden sm:inline">MOOC</span>
        </Link>

        <div className="flex items-center gap-1.5">
          <Link href="/catalog" className={linkCls}>
            Catálogo
          </Link>

          {user ? (
            <>
              <Link href="/dashboard" className={linkCls}>
                Panel
              </Link>
              <span className="hidden items-center gap-2 pl-2 md:flex">
                <span className="text-sm font-medium text-[var(--color-text)]">
                  {user.full_name}
                </span>
                <Tag tone="brand">{user.role}</Tag>
              </span>
              <ThemeToggle />
              <LogoutButton />
            </>
          ) : (
            <>
              <ThemeToggle />
              <Link
                href="/login"
                className="rounded-md bg-[var(--color-primary)] px-4 py-1.5 text-sm font-semibold text-[var(--color-on-primary)] transition-colors hover:bg-[var(--color-primary-hover)]"
              >
                Entrar
              </Link>
            </>
          )}
        </div>
      </nav>
    </header>
  );
}
