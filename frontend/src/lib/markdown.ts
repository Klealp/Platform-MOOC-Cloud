import DOMPurify from "isomorphic-dompurify";
import { marked } from "marked";

/**
 * Convierte Markdown a HTML y lo SANEA.
 *
 * El backend guarda el Markdown crudo y advierte explicitamente que el saneo
 * ocurre al renderizar en el frontend. Nunca inyectamos HTML sin pasar por
 * DOMPurify: asi un contenido malicioso (p. ej. <script> o un onerror en una
 * imagen) no se ejecuta. Esta es la defensa contra XSS almacenado.
 */
export function renderMarkdown(md: string): string {
  const rawHtml = marked.parse(md ?? "", { async: false }) as string;
  return DOMPurify.sanitize(rawHtml, {
    ALLOWED_TAGS: [
      "h1", "h2", "h3", "h4", "h5", "h6",
      "p", "br", "hr", "blockquote",
      "ul", "ol", "li",
      "strong", "em", "del", "code", "pre",
      "a", "img", "table", "thead", "tbody", "tr", "th", "td",
    ],
    ALLOWED_ATTR: ["href", "title", "src", "alt", "target", "rel"],
    // Enlaces externos seguros.
    ADD_ATTR: ["target", "rel"],
  });
}
