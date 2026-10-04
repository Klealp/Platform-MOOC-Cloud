"use client";

import { useActionState } from "react";

import { createCourseAction, type AuthoringResult } from "@/lib/authoring/actions";
import { Card, CardBody } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { Alert } from "@/components/ui/alert";

export function NewCourseForm() {
  const [state, action, pending] = useActionState<AuthoringResult, FormData>(
    createCourseAction,
    {},
  );

  return (
    <Card>
      <CardBody className="space-y-4">
        {state.error && <Alert variant="error">{state.error}</Alert>}
        <form action={action} className="space-y-4">
          <Field label="Título" htmlFor="title">
            <Input id="title" name="title" placeholder="Fundamentos de la nube" required />
          </Field>
          <Field label="Resumen" htmlFor="summary">
            <Textarea id="summary" name="summary" placeholder="¿De qué trata el curso?" />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Categoría" htmlFor="category">
              <Input id="category" name="category" placeholder="cloud" />
            </Field>
            <Field label="Nivel" htmlFor="level">
              <Select id="level" name="level" defaultValue="beginner">
                <option value="beginner">Principiante</option>
                <option value="intermediate">Intermedio</option>
                <option value="advanced">Avanzado</option>
              </Select>
            </Field>
          </div>
          <Button type="submit" disabled={pending}>
            {pending ? "Creando…" : "Crear curso"}
          </Button>
        </form>
      </CardBody>
    </Card>
  );
}
