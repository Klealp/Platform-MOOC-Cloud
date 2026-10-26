"use client";

import * as React from "react";
import { cn } from "@/lib/utils";

const base =
  "w-full rounded-[var(--radius-sm)] border border-[var(--color-border-strong)] " +
  "bg-[var(--color-surface)] text-[var(--color-text)] px-3.5 py-2.5 text-base " +
  "transition-colors placeholder:text-[var(--color-text-muted)] " +
  "focus-visible:outline-none focus-visible:border-[var(--color-primary)] " +
  "disabled:opacity-50 disabled:cursor-not-allowed";

export const Input = React.forwardRef<
  HTMLInputElement,
  React.InputHTMLAttributes<HTMLInputElement>
>(({ className, ...props }, ref) => (
  <input ref={ref} className={cn(base, className)} {...props} />
));
Input.displayName = "Input";

export const Textarea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement>
>(({ className, ...props }, ref) => (
  <textarea ref={ref} className={cn(base, "min-h-28 resize-y", className)} {...props} />
));
Textarea.displayName = "Textarea";

export const Select = React.forwardRef<
  HTMLSelectElement,
  React.SelectHTMLAttributes<HTMLSelectElement>
>(({ className, ...props }, ref) => (
  <select ref={ref} className={cn(base, "cursor-pointer", className)} {...props} />
));
Select.displayName = "Select";

export function Label({
  className,
  ...props
}: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn(
        "mb-1.5 block text-sm font-medium text-[var(--color-text)]",
        className,
      )}
      {...props}
    />
  );
}

/**
 * Etiqueta + control + ayuda o error, asociados para tecnologias de apoyo:
 * la etiqueta apunta al id del control (el de `htmlFor` o uno generado) y la
 * ayuda o el error se enlazan con aria-describedby (y aria-invalid si hay
 * error). Asi un campo nunca queda sin nombre accesible.
 */
export function Field({
  label,
  htmlFor,
  hint,
  error,
  children,
}: {
  label: string;
  htmlFor?: string;
  hint?: string;
  error?: string;
  children: React.ReactNode;
}) {
  const generated = React.useId();
  const id = htmlFor ?? generated;
  const message = error ?? hint;
  const messageId = message ? `${id}-ayuda` : undefined;

  let control = children;
  if (React.isValidElement<Record<string, unknown>>(children)) {
    control = React.cloneElement(children, {
      id,
      "aria-describedby": messageId,
      "aria-invalid": error ? true : undefined,
    });
  }

  return (
    <div>
      <Label htmlFor={id}>{label}</Label>
      {control}
      {message && (
        <p
          id={messageId}
          className={cn(
            "mt-1 text-xs",
            error ? "font-medium text-[var(--color-danger)]" : "text-[var(--color-text-muted)]",
          )}
        >
          {message}
        </p>
      )}
    </div>
  );
}
