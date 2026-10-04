"use client";

import { LogOut } from "lucide-react";
import { logoutAction } from "@/lib/auth/actions";

/** Boton de cierre de sesion. Dispara la Server Action que limpia la cookie. */
export function LogoutButton() {
  return (
    <form action={logoutAction}>
      <button
        type="submit"
        title="Cerrar sesión"
        className="flex items-center gap-1 border-2 border-ink bg-danger px-3 py-1.5 text-sm font-bold uppercase text-white brutal-press"
      >
        <LogOut className="h-4 w-4" />
        <span className="hidden sm:inline">Salir</span>
      </button>
    </form>
  );
}
