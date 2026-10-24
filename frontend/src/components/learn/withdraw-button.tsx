"use client";

import { useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { LogOut } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { withdrawAction } from "@/lib/learning/actions";

/**
 * Permite al estudiante retirarse de un curso (DELETE /enrollments/:id).
 *
 * El retiro NO borra el progreso: el backend solo cambia el estado a
 * `withdrawn`; al reinscribirse se reactiva la misma fila. Pedimos
 * confirmación porque es una acción destructiva desde el punto de vista del
 * usuario (deja de ver el curso en "Mis cursos").
 */
export function WithdrawButton({
  enrollmentId,
  size = "sm",
}: {
  enrollmentId: string;
  size?: "sm" | "md" | "lg";
}) {
  const router = useRouter();
  const [pending, start] = useTransition();
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function confirm() {
    setError(null);
    start(async () => {
      const res = await withdrawAction(enrollmentId);
      if (res?.error) {
        setError(res.error);
        setConfirming(false);
      } else {
        router.refresh();
      }
    });
  }

  if (confirming) {
    return (
      <div className="space-y-2">
        {error && <Alert variant="error">{error}</Alert>}
        <p className="text-sm font-medium">
          ¿Seguro que quieres retirarte? Tu progreso se conserva si vuelves a
          inscribirte.
        </p>
        <div className="flex gap-2">
          <Button size={size} variant="danger" disabled={pending} onClick={confirm}>
            {pending ? "Retirando…" : "Sí, retirarme"}
          </Button>
          {/* El boton "Retirarme" desaparece al confirmar: el foco pasa a
              "Cancelar" (la opcion segura) para no perderse (WCAG 2.4.3). */}
          <Button
            size={size}
            variant="outline"
            disabled={pending}
            autoFocus
            onClick={() => setConfirming(false)}
          >
            Cancelar
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {error && <Alert variant="error">{error}</Alert>}
      <Button
        size={size}
        variant="outline"
        disabled={pending}
        onClick={() => setConfirming(true)}
      >
        <LogOut className="h-4 w-4" /> Retirarme
      </Button>
    </div>
  );
}
