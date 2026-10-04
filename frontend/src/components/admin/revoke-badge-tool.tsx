"use client";

import { useState, useTransition } from "react";
import { ShieldX } from "lucide-react";

import { Card, CardBody, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";
import { revokeBadgeAction } from "@/lib/admin/actions";

export function RevokeBadgeTool() {
  const [badgeId, setBadgeId] = useState("");
  const [message, setMessage] = useState<{ ok: boolean; text: string } | null>(null);
  const [pending, start] = useTransition();

  function revoke() {
    if (!badgeId.trim()) {
      setMessage({ ok: false, text: "Ingresa el ID de la insignia." });
      return;
    }
    setMessage(null);
    start(async () => {
      const res = await revokeBadgeAction(badgeId.trim());
      if (res.error) setMessage({ ok: false, text: res.error });
      else setMessage({ ok: true, text: "Insignia revocada correctamente." });
    });
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Revocar insignia</CardTitle>
      </CardHeader>
      <CardBody className="space-y-4">
        {message && (
          <Alert variant={message.ok ? "success" : "error"}>{message.text}</Alert>
        )}
        <Field label="ID de la insignia (UUID)" hint="Lo encuentras en la bitácora o en los datos del estudiante.">
          <Input
            value={badgeId}
            onChange={(e) => setBadgeId(e.target.value)}
            placeholder="00000000-0000-0000-0000-000000000000"
          />
        </Field>
        <Button variant="danger" onClick={revoke} disabled={pending}>
          <ShieldX className="h-5 w-5" />
          {pending ? "Revocando…" : "Revocar insignia"}
        </Button>
      </CardBody>
    </Card>
  );
}
