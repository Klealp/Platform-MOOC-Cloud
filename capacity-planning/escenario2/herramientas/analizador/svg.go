package main

// Graficas SVG estaticas para el informe (se ven en GitHub y en el navegador).
// Reglas que se siguen a proposito:
//   - un solo eje Y por grafica (nunca dos escalas);
//   - colores categoricos en orden fijo, asignados a la entidad (no al rango);
//   - leyenda siempre que haya 2 o mas series y etiqueta directa al final de
//     la linea cuando son 4 o menos, para no depender solo del color;
//   - lineas de 2 px, marcadores de 8 px con forma distinta por serie en los
//     diagramas de dispersion; rejilla y ejes en gris tenue;
//   - fondo claro explicito, para que se lea igual en el tema oscuro de GitHub.

import (
	"fmt"
	"html"
	"math"
	"os"
	"sort"
	"strings"
)

var paleta = []string{"#2a78d6", "#eb6834", "#1baf7a", "#eda100", "#e87ba4", "#008300", "#4a3aa7", "#e34948"}

const (
	colorFondo   = "#fcfcfb"
	colorTexto   = "#0b0b0b"
	colorTexto2  = "#52514e"
	colorRejilla = "#e4e3df"
	colorEje     = "#b9b8b2"
	colorRef     = "#8a8983"
)

type Serie struct {
	Nombre string
	Puntos [][2]float64
}

type Grafica struct {
	Titulo, EjeX, EjeY string
	Series             []Serie
	Dispersion         bool     // marcadores en vez de lineas
	Referencias        []Ref    // lineas horizontales punteadas (umbral, limite)
	YMin               *float64 // por defecto 0
}

type Ref struct {
	Y     float64
	Texto string
}

func escribirSVG(path string, g Grafica) error {
	return os.WriteFile(path, []byte(g.svg()), 0o644)
}

// ticks "bonitos" (1, 2, 5 x 10^k) entre lo y hi.
func ticks(lo, hi float64, n int) []float64 {
	if hi <= lo {
		hi = lo + 1
	}
	paso := (hi - lo) / float64(n)
	mag := math.Pow(10, math.Floor(math.Log10(paso)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if paso <= m*mag {
			paso = m * mag
			break
		}
	}
	var out []float64
	for v := math.Ceil(lo/paso) * paso; v <= hi+paso*1e-9; v += paso {
		out = append(out, v)
	}
	return out
}

func fmtNum(v float64) string {
	a := math.Abs(v)
	if a < 1e-9 {
		return "0"
	}
	switch {
	case a >= 10000:
		return fmt.Sprintf("%.0fk", v/1000)
	case a >= 100 || v == math.Trunc(v):
		return fmt.Sprintf("%.0f", v)
	case a >= 10:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

func (g Grafica) svg() string {
	const W, H = 860.0, 420.0
	const ml, mr, mt, mb = 70.0, 150.0, 70.0, 56.0
	pw, ph := W-ml-mr, H-mt-mb

	xmin, xmax := math.Inf(1), math.Inf(-1)
	ymin, ymax := 0.0, math.Inf(-1)
	for _, s := range g.Series {
		for _, p := range s.Puntos {
			if math.IsNaN(p[1]) || math.IsInf(p[1], 0) {
				continue
			}
			xmin, xmax = math.Min(xmin, p[0]), math.Max(xmax, p[0])
			ymax = math.Max(ymax, p[1])
		}
	}
	for _, r := range g.Referencias {
		ymax = math.Max(ymax, r.Y)
	}
	if g.YMin != nil {
		ymin = *g.YMin
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" font-family="system-ui,-apple-system,Segoe UI,Roboto,sans-serif" role="img" aria-label="%s">`,
		W, H, W, H, html.EscapeString(g.Titulo))
	fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="%s" rx="8"/>`, colorFondo)
	fmt.Fprintf(&b, `<text x="%.0f" y="28" font-size="16" font-weight="600" fill="%s">%s</text>`, ml, colorTexto, html.EscapeString(g.Titulo))

	if math.IsInf(xmin, 1) {
		fmt.Fprintf(&b, `<text x="%.0f" y="%.0f" font-size="14" fill="%s">Sin datos para esta grafica</text></svg>`, ml, H/2, colorTexto2)
		return b.String()
	}
	if xmax == xmin {
		xmax = xmin + 1
	}
	ymax *= 1.08
	if ymax <= ymin {
		ymax = ymin + 1
	}
	X := func(x float64) float64 { return ml + (x-xmin)/(xmax-xmin)*pw }
	Y := func(y float64) float64 { return mt + ph - (y-ymin)/(ymax-ymin)*ph }

	// Rejilla y ejes
	for _, t := range ticks(ymin, ymax, 5) {
		fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, ml, ml+pw, Y(t), Y(t), colorRejilla)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s" text-anchor="end">%s</text>`, ml-8, Y(t)+4, colorTexto2, fmtNum(t))
	}
	for _, t := range ticks(xmin, xmax, 8) {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s" text-anchor="middle">%s</text>`, X(t), mt+ph+18, colorTexto2, fmtNum(t))
	}
	fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" stroke="%s"/>`, ml, ml+pw, mt+ph, mt+ph, colorEje)
	fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="12" fill="%s" text-anchor="middle">%s</text>`, ml+pw/2, H-14, colorTexto2, html.EscapeString(g.EjeX))
	fmt.Fprintf(&b, `<text transform="translate(18 %.1f) rotate(-90)" font-size="12" fill="%s" text-anchor="middle">%s</text>`, mt+ph/2, colorTexto2, html.EscapeString(g.EjeY))

	for _, r := range g.Referencias {
		fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5" stroke-dasharray="5 4"/>`, ml, ml+pw, Y(r.Y), Y(r.Y), colorRef)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s">%s</text>`, ml+4, Y(r.Y)-5, colorTexto2, html.EscapeString(r.Texto))
	}

	// Series
	type etiqueta struct {
		y     float64
		texto string
		color string
	}
	var finales []etiqueta
	for i, s := range g.Series {
		col := paleta[i%len(paleta)]
		pts := make([][2]float64, 0, len(s.Puntos))
		for _, p := range s.Puntos {
			if !math.IsNaN(p[1]) && !math.IsInf(p[1], 0) {
				pts = append(pts, p)
			}
		}
		if len(pts) == 0 {
			continue
		}
		sort.Slice(pts, func(a, c int) bool { return pts[a][0] < pts[c][0] })
		if g.Dispersion {
			for _, p := range pts {
				b.WriteString(marcador(i, X(p[0]), Y(p[1]), col,
					fmt.Sprintf("%s: x=%s, y=%s", s.Nombre, fmtNum(p[0]), fmtNum(p[1]))))
			}
		} else {
			var d strings.Builder
			for j, p := range pts {
				if j == 0 {
					fmt.Fprintf(&d, "M%.1f %.1f", X(p[0]), Y(p[1]))
				} else {
					fmt.Fprintf(&d, " L%.1f %.1f", X(p[0]), Y(p[1]))
				}
			}
			fmt.Fprintf(&b, `<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"><title>%s</title></path>`,
				d.String(), col, html.EscapeString(s.Nombre))
			// Con pocos puntos (p. ej. la comparativa de niveles) se marca cada medicion.
			if len(pts) <= 12 {
				for _, p := range pts {
					fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="4" fill="%s" stroke="%s" stroke-width="2"><title>%s: x=%s, y=%s</title></circle>`,
						X(p[0]), Y(p[1]), col, colorFondo, html.EscapeString(s.Nombre), fmtNum(p[0]), fmtNum(p[1]))
				}
			}
			ult := pts[len(pts)-1]
			finales = append(finales, etiqueta{Y(ult[1]), s.Nombre, col})
		}
	}

	// Etiquetas directas al final de cada linea (<= 4 series), separadas >= 13 px.
	if !g.Dispersion && len(finales) <= 4 {
		sort.Slice(finales, func(a, c int) bool { return finales[a].y < finales[c].y })
		for i := 1; i < len(finales); i++ {
			if finales[i].y-finales[i-1].y < 13 {
				finales[i].y = finales[i-1].y + 13
			}
		}
		for _, e := range finales {
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="3" fill="%s"/>`, ml+pw+8, e.y, e.color)
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s">%s</text>`, ml+pw+15, e.y+4, colorTexto, html.EscapeString(recortar(e.texto, 20)))
		}
	}

	// Leyenda arriba (siempre con 2 o mas series)
	if len(g.Series) >= 2 {
		x := ml
		for i, s := range g.Series {
			col := paleta[i%len(paleta)]
			if g.Dispersion {
				b.WriteString(marcador(i, x+5, 48, col, s.Nombre))
			} else {
				fmt.Fprintf(&b, `<line x1="%.1f" x2="%.1f" y1="48" y2="48" stroke="%s" stroke-width="3" stroke-linecap="round"/>`, x, x+14, col)
			}
			nombre := recortar(s.Nombre, 26)
			fmt.Fprintf(&b, `<text x="%.1f" y="52" font-size="12" fill="%s">%s</text>`, x+20, colorTexto, html.EscapeString(nombre))
			x += 32 + float64(len(nombre))*6.6
			if x > W-120 {
				break
			}
		}
	}
	b.WriteString(`</svg>`)
	return b.String()
}

// marcador dibuja formas distintas por serie (circulo, cuadrado, triangulo,
// rombo...) para que la identidad no dependa solo del color.
func marcador(i int, x, y float64, col, titulo string) string {
	t := "<title>" + html.EscapeString(titulo) + "</title>"
	borde := fmt.Sprintf(`stroke="%s" stroke-width="1.5"`, colorFondo)
	switch i % 4 {
	case 0:
		return fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4.5" fill="%s" %s>%s</circle>`, x, y, col, borde, t)
	case 1:
		return fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="8" height="8" fill="%s" %s>%s</rect>`, x-4, y-4, col, borde, t)
	case 2:
		return fmt.Sprintf(`<path d="M%.1f %.1f L%.1f %.1f L%.1f %.1f Z" fill="%s" %s>%s</path>`, x, y-5, x+5, y+4, x-5, y+4, col, borde, t)
	default:
		return fmt.Sprintf(`<path d="M%.1f %.1f L%.1f %.1f L%.1f %.1f L%.1f %.1f Z" fill="%s" %s>%s</path>`, x, y-5.5, x+5.5, y, x, y+5.5, x-5.5, y, col, borde, t)
	}
}

func recortar(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
