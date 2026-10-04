import * as React from "react";
import { AlertTriangle, CheckCircle2, Info, XCircle } from "lucide-react";
import { cn } from "@/lib/utils";

type Variant = "info" | "success" | "warning" | "error";

const config: Record<Variant, { cls: string; Icon: typeof Info }> = {
  info: { cls: "bg-accent-blue text-ink", Icon: Info },
  success: { cls: "bg-brand-500 text-ink", Icon: CheckCircle2 },
  warning: { cls: "bg-accent-yellow text-ink", Icon: AlertTriangle },
  error: { cls: "bg-danger text-white", Icon: XCircle },
};

export function Alert({
  variant = "info",
  title,
  children,
  className,
}: {
  variant?: Variant;
  title?: string;
  children?: React.ReactNode;
  className?: string;
}) {
  const { cls, Icon } = config[variant];
  return (
    <div
      role="alert"
      className={cn("flex gap-3 brutal-border brutal-shadow-sm p-3", cls, className)}
    >
      <Icon className="mt-0.5 h-5 w-5 shrink-0" aria-hidden />
      <div className="text-sm font-medium">
        {title && <p className="font-extrabold uppercase">{title}</p>}
        {children}
      </div>
    </div>
  );
}
