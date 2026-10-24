"use client";

import { useState, useTransition } from "react";
import { Rocket } from "lucide-react";

import { Button, ButtonLink } from "@/components/ui/button";
import { Alert } from "@/components/ui/alert";
import { enrollAction } from "@/lib/learning/actions";

export function EnrollButton({
  slug,
  authenticated,
}: {
  slug: string;
  authenticated: boolean;
}) {
  const [pending, start] = useTransition();
  const [error, setError] = useState<string | null>(null);

  if (!authenticated) {
    return (
      <ButtonLink href={`/login?from=/catalog/${slug}`} size="lg" className="w-full">
        Inicia sesión para inscribirte
      </ButtonLink>
    );
  }

  return (
    <div className="space-y-2">
      {error && <Alert variant="error">{error}</Alert>}
      <Button
        size="lg"
        className="w-full"
        disabled={pending}
        onClick={() =>
          start(async () => {
            const res = await enrollAction(slug);
            if (res?.error) setError(res.error);
          })
        }
      >
        <Rocket className="h-5 w-5" aria-hidden />
        {pending ? "Inscribiendo…" : "Inscribirme"}
      </Button>
    </div>
  );
}
