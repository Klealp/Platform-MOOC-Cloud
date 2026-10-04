import { NextResponse, type NextRequest } from "next/server";

/**
 * Middleware edge: primera linea de defensa.
 *
 * Solo comprueba la PRESENCIA de la cookie de sesion para redirigir pronto a
 * /login y evitar renderizar areas privadas sin sesion. NO decide roles aqui:
 * la autorizacion fina (admin/teacher/student) la hacen los guards
 * server-side (lib/auth/guards.ts) consultando /auth/me en el backend, que es
 * la fuente de verdad. El middleware no puede leer el valor HttpOnly con
 * seguridad para tomar decisiones de rol, y no debe.
 */

const SESSION_COOKIE = process.env.SESSION_COOKIE_NAME ?? "mooc_session";

// Prefijos que exigen haber iniciado sesion.
const PROTECTED_PREFIXES = ["/dashboard", "/learn", "/teach", "/admin", "/account"];

export function middleware(req: NextRequest) {
  const { pathname } = req.nextUrl;

  const isProtected = PROTECTED_PREFIXES.some(
    (p) => pathname === p || pathname.startsWith(`${p}/`),
  );
  if (!isProtected) return NextResponse.next();

  const hasSession = req.cookies.has(SESSION_COOKIE);
  if (!hasSession) {
    const url = req.nextUrl.clone();
    url.pathname = "/login";
    url.searchParams.set("from", pathname);
    return NextResponse.redirect(url);
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*", "/learn/:path*", "/teach/:path*", "/admin/:path*", "/account/:path*"],
};
