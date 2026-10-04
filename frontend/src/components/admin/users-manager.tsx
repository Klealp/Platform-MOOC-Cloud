"use client";

import { useRouter } from "next/navigation";
import { useActionState, useState, useTransition } from "react";
import { Plus, ShieldOff } from "lucide-react";

import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, Input, Select } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { Tag, statusTone } from "@/components/ui/badge";
import {
  createUserAction,
  revokeUserSessionsAction,
  updateUserAction,
  type AdminResult,
} from "@/lib/admin/actions";
import type { AdminUser, Role } from "@/lib/api/types";
import { formatDate } from "@/lib/utils";

export function UsersManager({ initialUsers }: { initialUsers: AdminUser[] }) {
  const router = useRouter();
  const [rowError, setRowError] = useState<string | null>(null);
  const [pending, start] = useTransition();

  const [createState, createAction, creating] = useActionState<AdminResult, FormData>(
    createUserAction,
    {},
  );

  function changeRole(user: AdminUser, role: Role) {
    setRowError(null);
    start(async () => {
      const res = await updateUserAction(user.id, { role });
      if (res.error) setRowError(res.error);
      else router.refresh();
    });
  }

  function changeStatus(user: AdminUser, status: string) {
    setRowError(null);
    start(async () => {
      const res = await updateUserAction(user.id, { status });
      if (res.error) setRowError(res.error);
      else router.refresh();
    });
  }

  function revoke(user: AdminUser) {
    setRowError(null);
    start(async () => {
      const res = await revokeUserSessionsAction(user.id);
      if (res.error) setRowError(res.error);
    });
  }

  return (
    <div className="space-y-6">
      {/* Crear usuario */}
      <Card>
        <CardHeader>
          <CardTitle>Crear usuario</CardTitle>
        </CardHeader>
        <CardBody>
          {createState.error && <Alert variant="error" className="mb-3">{createState.error}</Alert>}
          {createState.ok && <Alert variant="success" className="mb-3">Usuario creado.</Alert>}
          <form action={createAction} className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Field label="Nombre"><Input name="full_name" required /></Field>
            <Field label="Correo"><Input name="email" type="email" required /></Field>
            <Field label="Contraseña" hint="mín. 10">
              <Input name="password" type="password" minLength={10} required />
            </Field>
            <Field label="Rol">
              <Select name="role" defaultValue="teacher">
                <option value="teacher">Profesor</option>
                <option value="admin">Administrador</option>
                <option value="student">Estudiante</option>
              </Select>
            </Field>
            <div className="sm:col-span-2 lg:col-span-4">
              <Button type="submit" disabled={creating}>
                <Plus className="h-5 w-5" /> {creating ? "Creando…" : "Crear"}
              </Button>
            </div>
          </form>
        </CardBody>
      </Card>

      {rowError && <Alert variant="error">{rowError}</Alert>}

      {/* Tabla de usuarios */}
      <div className="overflow-x-auto brutal-border brutal-shadow bg-[var(--card)]">
        <table className="w-full text-sm">
          <thead className="border-b-[3px] border-ink bg-brand-100">
            <tr className="text-left uppercase">
              <th className="p-3">Usuario</th>
              <th className="p-3">Rol</th>
              <th className="p-3">Estado</th>
              <th className="p-3">Creado</th>
              <th className="p-3">Acciones</th>
            </tr>
          </thead>
          <tbody>
            {initialUsers.map((u) => (
              <tr key={u.id} className="border-b-2 border-ink last:border-b-0">
                <td className="p-3">
                  <p className="font-bold">{u.full_name}</p>
                  <p className="opacity-60">{u.email}</p>
                  {!u.email_verified && <Tag tone="warn">sin verificar</Tag>}
                </td>
                <td className="p-3">
                  <Select
                    value={u.role}
                    disabled={pending}
                    onChange={(e) => changeRole(u, e.target.value as Role)}
                    className="w-32"
                  >
                    <option value="student">student</option>
                    <option value="teacher">teacher</option>
                    <option value="admin">admin</option>
                  </Select>
                </td>
                <td className="p-3">
                  <Select
                    value={u.status}
                    disabled={pending}
                    onChange={(e) => changeStatus(u, e.target.value)}
                    className="w-40"
                  >
                    <option value="active">active</option>
                    <option value="suspended">suspended</option>
                    <option value="pending_verification">pending</option>
                  </Select>
                  <span className="ml-1"><Tag tone={statusTone(u.status)}>{u.status}</Tag></span>
                </td>
                <td className="p-3 font-medium opacity-70">{formatDate(u.created_at)}</td>
                <td className="p-3">
                  <button
                    title="Revocar sesiones"
                    disabled={pending}
                    onClick={() => revoke(u)}
                    className="flex items-center gap-1 border-2 border-ink bg-accent-yellow px-2 py-1 text-xs font-bold uppercase brutal-press"
                  >
                    <ShieldOff className="h-3.5 w-3.5" /> Sesiones
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
