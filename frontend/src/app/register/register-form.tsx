"use client";

import { useActionState } from "react";

import { registerAction, type FormState } from "@/lib/auth/actions";
import { Card, CardBody } from "@/components/ui/card";
import { Button, ButtonLink } from "@/components/ui/button";
import { Field, Input } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";

export function RegisterForm() {
  const [state, action, pending] = useActionState<FormState, FormData>(
    registerAction,
    {},
  );

  if (state.ok) {
    return (
      <Card>
        <CardBody className="space-y-4">
          <Alert variant="success" title="Casi listo">
            Si el correo no estaba registrado, recibirás un enlace de
            verificación. Verifica tu cuenta y luego inicia sesión.
          </Alert>
          <ButtonLink href="/login" className="w-full">Ir a iniciar sesión</ButtonLink>
        </CardBody>
      </Card>
    );
  }

  return (
    <Card>
      <CardBody className="space-y-4">
        {state.error && <Alert variant="error">{state.error}</Alert>}
        <form action={action} className="space-y-4">
          <Field label="Nombre completo" htmlFor="full_name">
            <Input id="full_name" name="full_name" autoComplete="name" placeholder="Estudiante Demo" required />
          </Field>
          <Field label="Correo" htmlFor="email">
            <Input id="email" name="email" type="email" autoComplete="email" placeholder="estudiante@mooc.local" required />
          </Field>
          <Field
            label="Contraseña"
            htmlFor="password"
            hint="Mínimo 10 caracteres."
          >
            <Input
              id="password"
              name="password"
              type="password"
              minLength={10}
              autoComplete="new-password"
              placeholder="••••••••••"
              required
            />
          </Field>
          <Button type="submit" className="w-full" disabled={pending}>
            {pending ? "Creando…" : "Crear cuenta"}
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
