import * as React from "react";
import Link from "next/link";
import { cn } from "@/lib/utils";

type Variant = "primary" | "secondary" | "danger" | "ghost" | "outline";
type Size = "sm" | "md" | "lg";

const variants: Record<Variant, string> = {
  // Acción principal: fondo de marca, texto en --color-on-primary.
  primary:
    "bg-[var(--color-primary)] text-[var(--color-on-primary)] hover:bg-[var(--color-primary-hover)]",
  // Secundario: superficie con borde sutil.
  secondary:
    "bg-[var(--color-surface)] text-[var(--color-text)] border border-[var(--color-border)] hover:bg-[var(--color-surface-alt)]",
  // Alias de secondary para compatibilidad: borde sutil sobre superficie.
  outline:
    "bg-[var(--color-surface)] text-[var(--color-text)] border border-[var(--color-border)] hover:bg-[var(--color-surface-alt)]",
  // Destructivo.
  danger: "bg-[var(--color-danger)] text-[var(--color-on-danger)] hover:brightness-95",
  // Terciario: sin fondo ni borde.
  ghost:
    "bg-transparent text-[var(--color-text)] hover:bg-[var(--color-surface-alt)]",
};

const sizes: Record<Size, string> = {
  sm: "px-3 py-1.5 text-sm gap-1.5",
  md: "px-5 py-2.5 text-base gap-2",
  lg: "px-6 py-3 text-lg gap-2",
};

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
  size?: Size;
}

/** Clases de un boton, para aplicarlas tambien a un enlace. */
export function buttonClasses({
  variant = "primary",
  size = "md",
  className,
}: { variant?: Variant; size?: Size; className?: string } = {}) {
  return cn(
    "inline-flex items-center justify-center rounded-md font-semibold",
    "transition-colors duration-150 ease-out",
    "disabled:cursor-not-allowed disabled:opacity-50",
    variants[variant],
    sizes[size],
    className,
  );
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => (
    <button
      ref={ref}
      className={buttonClasses({ variant, size, className })}
      {...props}
    />
  ),
);
Button.displayName = "Button";

/**
 * Enlace con aspecto de boton. Reemplaza el patron <Link><Button/></Link>,
 * que anida un control dentro de otro (HTML invalido, dos paradas de
 * tabulacion y un nombre accesible confuso para los lectores de pantalla).
 */
export function ButtonLink({
  href,
  variant,
  size,
  className,
  ...props
}: React.ComponentProps<typeof Link> & { variant?: Variant; size?: Size }) {
  return (
    <Link href={href} className={buttonClasses({ variant, size, className })} {...props} />
  );
}
