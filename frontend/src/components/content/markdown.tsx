import { renderMarkdown } from "@/lib/markdown";
import { cn } from "@/lib/utils";

/**
 * Renderiza Markdown saneado. El HTML pasa por DOMPurify en el servidor antes
 * de inyectarse, de modo que dangerouslySetInnerHTML aqui es seguro.
 */
export function Markdown({ content, className }: { content: string; className?: string }) {
  const html = renderMarkdown(content);
  return (
    <div
      className={cn(
        "max-w-none space-y-3 leading-relaxed text-[var(--color-text)]",
        "[&_h1]:text-2xl [&_h1]:font-semibold",
        "[&_h2]:text-xl [&_h2]:font-semibold",
        "[&_h3]:text-lg [&_h3]:font-medium",
        "[&_a]:text-[var(--color-primary)] [&_a]:underline [&_a]:underline-offset-2",
        "[&_ul]:list-disc [&_ul]:pl-6 [&_ol]:list-decimal [&_ol]:pl-6",
        "[&_code]:bg-[var(--color-surface-alt)] [&_code]:px-1 [&_code]:font-mono [&_code]:text-sm [&_code]:rounded-[var(--radius-sm)]",
        "[&_pre]:border [&_pre]:border-[var(--color-border)] [&_pre]:bg-[var(--color-surface-alt)] [&_pre]:p-3 [&_pre]:text-[var(--color-text)] [&_pre]:overflow-x-auto [&_pre]:rounded-[var(--radius-md)]",
        "[&_blockquote]:border-l [&_blockquote]:border-[var(--color-border)] [&_blockquote]:pl-4 [&_blockquote]:italic",
        "[&_img]:rounded-[var(--radius-md)] [&_table]:w-full [&_th]:border [&_th]:border-[var(--color-border)] [&_td]:border [&_td]:border-[var(--color-border)] [&_th]:p-2 [&_td]:p-2",
        className,
      )}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
