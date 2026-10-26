import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import "./globals.css";

const sans = Inter({
  subsets: ["latin"],
  variable: "--font-inter",
  display: "swap",
});

const mono = JetBrains_Mono({
  subsets: ["latin"],
  variable: "--font-jetbrains",
  weight: ["400", "600"],
  display: "swap",
});

// Cada pagina declara su propio titulo (WCAG 2.4.2); la plantilla le agrega
// el nombre del sitio.
export const metadata: Metadata = {
  title: { default: "Plataforma MOOC", template: "%s · Plataforma MOOC" },
  description: "Cursos masivos abiertos en linea",
};

/*
  Script anti-parpadeo: aplica la clase .dark ANTES de pintar, leyendo la
  preferencia guardada (o la del sistema). Debe ir inline en <head> para
  ejecutarse de forma síncrona y evitar el "flash" de tema incorrecto.
*/
const themeScript = `
(function () {
  try {
    var stored = localStorage.getItem("theme");
    var prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    if (stored === "dark" || (!stored && prefersDark)) {
      document.documentElement.classList.add("dark");
    }
  } catch (e) {}
})();
`;

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="es" className={`${sans.variable} ${mono.variable}`} suppressHydrationWarning>
      <head>
        <script dangerouslySetInnerHTML={{ __html: themeScript }} />
      </head>
      <body>
        {/* Primer elemento tabulable: permite saltar la barra de navegacion
            (WCAG 2.4.1). Cada pagina marca su <main> con id="contenido". */}
        <a
          href="#contenido"
          className="sr-only rounded-md bg-[var(--color-primary)] px-4 py-2 font-semibold text-[var(--color-on-primary)] focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-50"
        >
          Saltar al contenido
        </a>
        {children}
      </body>
    </html>
  );
}
