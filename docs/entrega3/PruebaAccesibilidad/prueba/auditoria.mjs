// Auditoria automatica de accesibilidad (WCAG 2.2 AA) del frontend.
//
// Recorre las paginas principales por rol (anonimo, estudiante, profesor,
// administrador), en tema claro y oscuro, y ejecuta axe-core sobre cada una.
// Escribe resultados/REPORTE.md y resultados/detalle.json. Termina con
// codigo 1 si alguna pagina tiene incumplimientos criticos o graves.
//
// Solo lee paginas: no inscribe, no inicia quizzes ni edita nada. El unico
// efecto en el sistema es una sesion nueva (y su linea user.login en la
// bitacora) por cada rol con credenciales.

import net from "node:net";
import { mkdir, writeFile } from "node:fs/promises";
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";

const DESTINO = process.env.DESTINO ?? "web:3000";
const SALIDA = process.env.SALIDA ?? "/resultados";
const PUERTO_LOCAL = 3100;
const BASE = `http://localhost:${PUERTO_LOCAL}`;
const ETIQUETAS = ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"];
const BLOQUEANTES = new Set(["critical", "serious"]);

const CREDENCIALES = {
  estudiante: [process.env.ESTUDIANTE_EMAIL, process.env.ESTUDIANTE_PASSWORD],
  profesor: [process.env.PROFESOR_EMAIL, process.env.PROFESOR_PASSWORD],
  admin: [process.env.ADMIN_EMAIL, process.env.ADMIN_PASSWORD],
};

// El frontend local corre con NODE_ENV=production y marca la cookie de sesion
// como Secure. Chromium solo guarda una cookie Secure sobre http si el origen
// es localhost, asi que no se navega a http://web:3000 sino a un puerto local
// que reenvia hacia alla. Asi no hay que cambiar la configuracion del frontend.
function abrirTunel() {
  const [host, puerto] = DESTINO.split(":");
  const servidor = net.createServer((cliente) => {
    const remoto = net.connect(Number(puerto), host);
    cliente.pipe(remoto).pipe(cliente);
    const cerrar = () => {
      cliente.destroy();
      remoto.destroy();
    };
    cliente.on("error", cerrar);
    remoto.on("error", cerrar);
  });
  return new Promise((resolve, reject) => {
    servidor.once("error", reject);
    servidor.listen(PUERTO_LOCAL, "127.0.0.1", () => resolve(servidor));
  });
}

// Primer enlace de la pagina actual que empiece por `prefijo` y no este en
// `excluir`. Sirve para descubrir ids (curso, inscripcion) sin conocerlos.
async function primerEnlace(page, prefijo, excluir = []) {
  const hrefs = await page.$$eval("a[href]", (as) => as.map((a) => a.getAttribute("href")));
  return hrefs.find(
    (h) => h && h.startsWith(prefijo) && !excluir.some((x) => h.startsWith(x)) && !h.includes("?"),
  );
}

async function iniciarSesion(browser, rol) {
  const [email, password] = CREDENCIALES[rol];
  if (!email || !password) return { omitida: `sin credenciales de ${rol}` };
  const context = await browser.newContext();
  const page = await context.newPage();
  await page.goto(`${BASE}/login`, { waitUntil: "networkidle" });
  await page.fill("#email", email);
  await page.fill("#password", password);
  await Promise.all([
    page.waitForURL((u) => !u.pathname.startsWith("/login"), { timeout: 15000 }).catch(() => {}),
    page.getByRole("button", { name: "Entrar" }).click(),
  ]);
  if (new URL(page.url()).pathname.startsWith("/login")) {
    await context.close();
    return { omitida: `no se pudo iniciar sesion como ${rol} (${email})` };
  }
  const estado = await context.storageState();
  await context.close();
  return { estado, email };
}

async function auditar(page, url) {
  await page.goto(`${BASE}${url}`, { waitUntil: "networkidle", timeout: 30000 });
  // Margen para el contenido que se carga en el cliente (Markdown, archivos).
  await page.waitForTimeout(800);
  const res = await new AxeBuilder({ page }).withTags(ETIQUETAS).analyze();
  return res.violations;
}

const paginas = [
  // rol, nombre, ruta fija o funcion que la descubre desde otra pagina
  { rol: "anonimo", nombre: "Inicio", ruta: "/" },
  { rol: "anonimo", nombre: "Catalogo", ruta: "/catalog" },
  {
    rol: "anonimo",
    nombre: "Ficha de curso",
    descubrir: async (page) => {
      await page.goto(`${BASE}/catalog`, { waitUntil: "networkidle" });
      return primerEnlace(page, "/catalog/");
    },
  },
  { rol: "anonimo", nombre: "Iniciar sesion", ruta: "/login" },
  { rol: "anonimo", nombre: "Crear cuenta", ruta: "/register" },
  { rol: "anonimo", nombre: "Pagina 404", ruta: "/pagina-que-no-existe" },
  { rol: "estudiante", nombre: "Panel", ruta: "/dashboard" },
  { rol: "estudiante", nombre: "Mis cursos", ruta: "/learn" },
  {
    rol: "estudiante",
    nombre: "Reproductor del curso",
    descubrir: async (page) => {
      await page.goto(`${BASE}/learn`, { waitUntil: "networkidle" });
      return primerEnlace(page, "/learn/", ["/learn/badges"]);
    },
  },
  { rol: "estudiante", nombre: "Mis insignias", ruta: "/learn/badges" },
  {
    rol: "estudiante",
    nombre: "Verificacion publica de insignia",
    descubrir: async (page) => {
      await page.goto(`${BASE}/learn/badges`, { waitUntil: "networkidle" });
      return primerEnlace(page, "/badges/verify/");
    },
  },
  { rol: "estudiante", nombre: "Mis sesiones", ruta: "/account/sessions" },
  { rol: "profesor", nombre: "Mis cursos (autoria)", ruta: "/teach" },
  { rol: "profesor", nombre: "Nuevo curso", ruta: "/teach/new" },
  {
    rol: "profesor",
    nombre: "Editor de curso",
    descubrir: async (page) => {
      await page.goto(`${BASE}/teach`, { waitUntil: "networkidle" });
      return primerEnlace(page, "/teach/", ["/teach/new"]);
    },
  },
  { rol: "admin", nombre: "Usuarios y roles", ruta: "/admin/users" },
  { rol: "admin", nombre: "Bitacora de auditoria", ruta: "/admin/audit" },
  { rol: "admin", nombre: "Insignias (administracion)", ruta: "/admin/badges" },
  { rol: "admin", nombre: "Todos los cursos", ruta: "/teach" },
];

function escaparTabla(texto) {
  return String(texto).replace(/\|/g, "\\|").replace(/\n/g, " ");
}

function reporte({ filas, omitidas, cuentas, aprobada, inicio }) {
  const l = [];
  l.push("# Auditoría automática de accesibilidad — WCAG 2.2 AA", "");
  l.push(`- **Resultado:** ${aprobada ? "✅ APROBADA" : "❌ NO APROBADA"}`);
  l.push(`- **Fecha:** ${inicio.toISOString().replace("T", " ").slice(0, 19)} UTC`);
  l.push(`- **Sitio auditado:** \`http://${DESTINO}\` (frontend en Docker Compose)`);
  l.push("- **Herramientas:** Playwright 1.49.1 (Chromium) + axe-core vía @axe-core/playwright 4.10.1");
  l.push(`- **Reglas:** ${ETIQUETAS.join(", ")}`);
  l.push(
    "- **Criterio de aprobación:** ninguna página con incumplimientos de impacto " +
      "`critical` o `serious`. Los `moderate` y `minor` se reportan pero no bloquean.",
  );
  l.push(`- **Usuarios:** ${cuentas.join(" · ") || "solo anónimo"}`, "");
  l.push(
    "> axe detecta de forma automática alrededor de un tercio de los criterios WCAG. " +
      "Lo que no puede juzgar (calidad del texto alternativo, orden lógico de lectura, " +
      "subtítulos de los videos) requiere revisión manual.",
    "",
  );

  l.push("## Resumen", "");
  l.push("| Página | Rol | Tema | Ruta | Críticas | Graves | Moderadas | Menores | Estado |");
  l.push("|---|---|---|---|---:|---:|---:|---:|---|");
  for (const f of filas) {
    const c = f.conteo;
    const ok = c.critical + c.serious === 0;
    l.push(
      `| ${f.nombre} | ${f.rol} | ${f.tema} | \`${f.ruta}\` | ${c.critical} | ${c.serious} | ${c.moderate} | ${c.minor} | ${ok ? "✅" : "❌"} |`,
    );
  }
  l.push("");

  if (omitidas.length) {
    l.push("## Páginas no auditadas", "");
    for (const o of omitidas) l.push(`- **${o.nombre}** (${o.rol}): ${o.motivo}`);
    l.push("");
  }

  const conFallas = filas.filter((f) => f.violaciones.length);
  l.push("## Detalle de incumplimientos", "");
  if (!conFallas.length) l.push("No se encontraron incumplimientos.", "");
  for (const f of conFallas) {
    l.push(`### ${f.nombre} — ${f.rol}, tema ${f.tema} (\`${f.ruta}\`)`, "");
    for (const v of f.violaciones) {
      const wcag = v.tags.filter((t) => t.startsWith("wcag")).join(", ");
      l.push(`- **${v.id}** · impacto \`${v.impact}\` · ${wcag}`);
      l.push(`  - ${escaparTabla(v.help)} ([guía](${v.helpUrl}))`);
      for (const n of v.nodes.slice(0, 5)) {
        l.push(`  - Elemento: \`${escaparTabla(n.target.join(" "))}\``);
      }
      if (v.nodes.length > 5) l.push(`  - … y ${v.nodes.length - 5} elementos más`);
    }
    l.push("");
  }
  return l.join("\n");
}

async function main() {
  const inicio = new Date();
  const tunel = await abrirTunel();
  const browser = await chromium.launch();

  const sesiones = { anonimo: { estado: undefined } };
  const cuentas = [];
  for (const rol of ["estudiante", "profesor", "admin"]) {
    sesiones[rol] = await iniciarSesion(browser, rol);
    if (sesiones[rol].email) cuentas.push(`${rol}: ${sesiones[rol].email}`);
  }

  const filas = [];
  const omitidas = [];
  for (const p of paginas) {
    const sesion = sesiones[p.rol];
    if (sesion.omitida) {
      omitidas.push({ ...p, motivo: sesion.omitida });
      continue;
    }
    let ruta = p.ruta;
    if (!ruta) {
      const ctx = await browser.newContext({ storageState: sesion.estado });
      ruta = await p.descubrir(await ctx.newPage());
      await ctx.close();
      if (!ruta) {
        omitidas.push({ ...p, motivo: "no hay datos para llegar a esta página (p. ej. sin cursos, inscripciones o insignias)" });
        continue;
      }
    }
    for (const tema of ["claro", "oscuro"]) {
      const ctx = await browser.newContext({
        storageState: sesion.estado,
        colorScheme: tema === "oscuro" ? "dark" : "light",
      });
      const page = await ctx.newPage();
      try {
        const violaciones = await auditar(page, ruta);
        const conteo = { critical: 0, serious: 0, moderate: 0, minor: 0 };
        for (const v of violaciones) conteo[v.impact] = (conteo[v.impact] ?? 0) + 1;
        filas.push({ ...p, ruta, tema, violaciones, conteo });
        const bloqueantes = conteo.critical + conteo.serious;
        console.log(`${bloqueantes ? "FALLA" : "ok   "}  ${p.rol.padEnd(10)} ${tema.padEnd(6)} ${ruta}`);
      } catch (err) {
        omitidas.push({ ...p, motivo: `error al cargar ${ruta} en tema ${tema}: ${err.message.split("\n")[0]}` });
      } finally {
        await ctx.close();
      }
    }
  }

  await browser.close();
  tunel.close();

  const aprobada =
    filas.length > 0 && filas.every((f) => !f.violaciones.some((v) => BLOQUEANTES.has(v.impact)));
  await mkdir(SALIDA, { recursive: true });
  await writeFile(`${SALIDA}/REPORTE.md`, reporte({ filas, omitidas, cuentas, aprobada, inicio }));
  await writeFile(
    `${SALIDA}/detalle.json`,
    JSON.stringify(
      filas.map(({ nombre, rol, tema, ruta, violaciones }) => ({ nombre, rol, tema, ruta, violaciones })),
      null,
      2,
    ),
  );
  console.log(`\nResultado: ${aprobada ? "APROBADA" : "NO APROBADA"} (${filas.length} auditorias, ${omitidas.length} omitidas)`);
  process.exit(aprobada ? 0 : 1);
}

main().catch((err) => {
  console.error(err);
  process.exit(2);
});
