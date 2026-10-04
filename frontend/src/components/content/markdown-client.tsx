"use client";

import { useMemo } from "react";

import { renderMarkdown } from "@/lib/markdown";
import { cn } from "@/lib/utils";

/**
 * Version cliente del renderizador de Markdown. isomorphic-dompurify funciona
 * tambien en el navegador, asi que el saneo contra XSS se aplica igual aqui
 * antes de inyectar el HTML.
 */
export function MarkdownClient({
  content,
  className,
}: {
  content: string;
  className?: string;
}) {
  const html = useMemo(() => renderMarkdown(content), [content]);
  return (
    <div
      className={cn(
        "max-w-none space-y-3 font-medium",
        "[&_h1]:text-2xl [&_h1]:font-extrabold [&_h1]:uppercase",
        "[&_h2]:text-xl [&_h2]:font-extrabold [&_h2]:uppercase",
        "[&_h3]:text-lg [&_h3]:font-bold",
        "[&_a]:font-bold [&_a]:underline [&_a]:decoration-2",
        "[&_ul]:list-disc [&_ul]:pl-6 [&_ol]:list-decimal [&_ol]:pl-6",
        "[&_code]:bg-brand-100 [&_code]:px-1 [&_code]:font-mono [&_code]:text-sm",
        "[&_pre]:brutal-border [&_pre]:bg-ink [&_pre]:p-3 [&_pre]:text-brand-300 [&_pre]:overflow-x-auto",
        "[&_blockquote]:border-l-[6px] [&_blockquote]:border-ink [&_blockquote]:pl-4 [&_blockquote]:italic",
        "[&_img]:brutal-border [&_table]:w-full [&_th]:brutal-border [&_td]:brutal-border [&_th]:p-2 [&_td]:p-2",
        className,
      )}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
