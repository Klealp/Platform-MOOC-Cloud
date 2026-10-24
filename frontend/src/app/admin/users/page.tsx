import type { Metadata } from "next";
import { Navbar } from "@/components/layout/navbar";
import { requireAdmin } from "@/lib/auth/guards";
import { listUsers } from "@/lib/api/admin";
import { UsersManager } from "@/components/admin/users-manager";

export const metadata: Metadata = { title: "Usuarios y roles" };

export default async function AdminUsersPage() {
  // requireAdmin() se ejecuta en el SERVIDOR: un no-admin es redirigido antes
  // de renderizar. El backend Go también exige rol admin (doble barrera).
  await requireAdmin("/admin/users");
  const users = await listUsers();

  return (
    <>
      <Navbar />
      <main id="contenido" tabIndex={-1} className="mx-auto max-w-[1200px] space-y-6 px-6 py-10">
        <h1 className="text-3xl font-bold tracking-tight">Usuarios y roles</h1>
        <UsersManager initialUsers={users} />
      </main>
    </>
  );
}
