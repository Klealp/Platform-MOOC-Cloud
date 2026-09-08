// Package badge genera la imagen de la insignia.
//
// Se produce un SVG escrito a mano en vez de usar una libreria de imagenes.
// Motivos: el SVG es texto (se versiona y se inspecciona facilmente), no
// necesita fuentes instaladas en el contenedor y no agrega dependencias.
// Si mas adelante se adopta Open Badges 3.0, la imagen se acompanara de los
// metadatos firmados que exige ese estandar.
package badge

import (
	"fmt"
	"strings"
	"time"
)

// RenderSVG devuelve el contenido del archivo de la insignia.
func RenderSVG(courseTitle, recipient, publicCode string, issuedAt time.Time) []byte {
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="360" viewBox="0 0 600 360">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%%" stop-color="#1e3a8a"/>
      <stop offset="100%%" stop-color="#0f766e"/>
    </linearGradient>
  </defs>
  <rect width="600" height="360" rx="18" fill="url(#bg)"/>
  <circle cx="300" cy="96" r="46" fill="none" stroke="#fbbf24" stroke-width="6"/>
  <text x="300" y="112" text-anchor="middle" font-family="sans-serif" font-size="42" fill="#fbbf24">%s</text>
  <text x="300" y="182" text-anchor="middle" font-family="sans-serif" font-size="16" fill="#cbd5e1"
        letter-spacing="3">CERTIFICADO DE APROBACION</text>
  <text x="300" y="222" text-anchor="middle" font-family="sans-serif" font-size="26" font-weight="bold"
        fill="#ffffff">%s</text>
  <text x="300" y="258" text-anchor="middle" font-family="sans-serif" font-size="17" fill="#e2e8f0">%s</text>
  <text x="300" y="304" text-anchor="middle" font-family="sans-serif" font-size="13" fill="#94a3b8">%s</text>
  <text x="300" y="326" text-anchor="middle" font-family="monospace" font-size="12" fill="#94a3b8">%s</text>
</svg>`,
		escapeXML(initials(recipient)),
		escapeXML(truncate(recipient, 34)),
		escapeXML(truncate(courseTitle, 44)),
		issuedAt.Format("02 de enero de 2006"),
		escapeXML(publicCode),
	)
	return []byte(svg)
}

func initials(name string) string {
	parts := strings.Fields(name)
	out := ""
	for _, p := range parts {
		if len(out) >= 2 {
			break
		}
		out += strings.ToUpper(string([]rune(p)[0]))
	}
	if out == "" {
		out = "?"
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}
