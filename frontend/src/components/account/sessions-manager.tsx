"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { Monitor, LogOut } from "lucide-react";

import { Card, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { Tag } from "@/components/ui/badge";
import { formatDateTime } from "@/lib/utils";
import { revokeSessionAction } from "@/lib/account/actions";
import type { ActiveSession } from "@/lib/api/types";

/**
 * Lista las sesiones activas del usuario y permite cerrarlas una a una.
 *
 * La sesión marcada como `current` es la que está en uso: al cerrarla, el
 * guard server-side redirige a /login en la siguiente navegación.
 */
export function SessionsManager({ sessions }: { sessions: ActiveSession[] }) {
  const router = useRouter();
  const [pending, start] = useTransition();
  const [error, setError] = useState<string | null>(null);
  const [revokingId, setRevokingId] = useState<string | null>(null);

  function revoke(session: ActiveSession) {
    setError(null);
    setRevokingId(session.id);
    start(async () => {
      const res = await revokeSessionAction(session.id);
      setRevokingId(null);
      if (res?.error) {
        setError(res.error);
        return;
      }
      // Si cerró su propia sesión, lo llevamos al login.
      if (session.current) {
        router.push("/login");
        return;
      }
      router.refresh();
    });
  }

  if (sessions.length === 0) {
    return <Alert variant="info">No hay sesiones activas.</Alert>;
  }

  return (
    <div className="space-y-4">
      {error && <Alert variant="error">{error}</Alert>}
      <div className="grid gap-3">
        {sessions.map((s) => (
          <Card key={s.id}>
            <CardBody className="flex flex-wrap items-center justify-between gap-3">
              <div className="min-w-0 space-y-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Monitor className="h-4 w-4 shrink-0" />
                  <span className="truncate text-sm font-bold">
                    {s.user_agent || "Dispositivo desconocido"}
                  </span>
                  {s.current && <Tag tone="ok">Sesión actual</Tag>}
                </div>
                <p className="text-xs font-medium text-[var(--color-text-muted)]">
                  IP {s.ip || "—"} · Inició {formatDateTime(s.created_at)} ·
                  Última actividad {formatDateTime(s.last_seen_at)}
                </p>
              </div>
              <Button
                size="sm"
                variant={s.current ? "danger" : "outline"}
                disabled={pending && revokingId === s.id}
                onClick={() => revoke(s)}
                aria-label={
                  s.current
                    ? "Cerrar esta sesión"
                    : `Cerrar la sesión de ${s.user_agent || "dispositivo desconocido"}`
                }
              >
                <LogOut className="h-4 w-4" />
                {pending && revokingId === s.id
                  ? "Cerrando…"
                  : s.current
                    ? "Cerrar esta sesión"
                    : "Cerrar"}
              </Button>
            </CardBody>
          </Card>
        ))}
      </div>
    </div>
  );
}
