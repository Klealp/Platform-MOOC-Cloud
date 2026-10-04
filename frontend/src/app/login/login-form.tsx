"use client";

import { useActionState } from "react";

import { loginAction, type FormState } from "@/lib/auth/actions";
import { Card, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";

export function LoginForm({ from }: { from?: string }) {
  const [state, action, pending] = useActionState<FormState, FormData>(
    loginAction,
    {},
  );

  return (
    <Card>
      <CardBody className="space-y-4">
        {state.error && <Alert variant="error">{state.error}</Alert>}
        <form action={action} className="space-y-4">
          <input type="hidden" name="from" value={from ?? ""} />
          <Field label="Correo" htmlFor="email">
            <Input
              id="email"
              name="email"
              type="email"
              autoComplete="email"
              placeholder="admin@mooc.local"
              required
            />
          </Field>
          <Field label="Contraseña" htmlFor="password">
            <Input
              id="password"
              name="password"
              type="password"
              autoComplete="current-password"
              placeholder="••••••••••"
              required
            />
          </Field>
          <Button type="submit" className="w-full" disabled={pending}>
            {pending ? "Entrando…" : "Entrar"}
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
