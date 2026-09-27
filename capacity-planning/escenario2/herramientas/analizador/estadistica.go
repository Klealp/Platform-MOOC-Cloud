package main

import (
	"math"
	"sort"
)

// Resumen de una distribucion. Los percentiles usan interpolacion lineal
// entre rangos (el mismo criterio que k6), asi las cifras coinciden con las
// del resumen de k6 cuando se calcula sobre el mismo conjunto.
type Resumen struct {
	N                             int
	Min, Prom, P50, P95, P99, Max float64
}

func resumir(v []float64) Resumen {
	if len(v) == 0 {
		return Resumen{}
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	suma := 0.0
	for _, x := range s {
		suma += x
	}
	return Resumen{N: len(s), Min: s[0], Prom: suma / float64(len(s)), P50: percentil(s, 50),
		P95: percentil(s, 95), P99: percentil(s, 99), Max: s[len(s)-1]}
}

// percentil sobre un slice YA ordenado.
func percentil(s []float64, p float64) float64 {
	if len(s) == 0 {
		return math.NaN()
	}
	if len(s) == 1 {
		return s[0]
	}
	pos := p / 100 * float64(len(s)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return s[lo] + (s[hi]-s[lo])*(pos-float64(lo))
}

// filtrar devuelve los valores de los puntos que cumplen todas las etiquetas.
// Una etiqueta con valor "*" solo exige que exista.
func filtrar(ps []Punto, tags map[string]string) []float64 {
	var out []float64
	for _, p := range ps {
		if cumple(p, tags) {
			out = append(out, p.V)
		}
	}
	return out
}

func cumple(p Punto, tags map[string]string) bool {
	for k, v := range tags {
		pv, ok := p.Tags[k]
		if !ok || (v != "*" && pv != v) {
			return false
		}
	}
	return true
}

// tasa: fraccion de puntos con valor 1 (para metricas Rate como http_req_failed).
func tasa(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// serieVentanas agrupa puntos en ventanas de 'ventMs' y aplica 'f' a cada una.
// Devuelve x en minutos desde t0.
func serieVentanas(ps []Punto, tags map[string]string, t0, ventMs int64, f func([]float64) float64) [][2]float64 {
	grupos := map[int64][]float64{}
	for _, p := range ps {
		if cumple(p, tags) {
			k := (p.T - t0) / ventMs
			grupos[k] = append(grupos[k], p.V)
		}
	}
	claves := make([]int64, 0, len(grupos))
	for k := range grupos {
		claves = append(claves, k)
	}
	sort.Slice(claves, func(i, j int) bool { return claves[i] < claves[j] })
	out := make([][2]float64, 0, len(claves))
	for _, k := range claves {
		x := float64(k*ventMs+ventMs/2) / 60000
		out = append(out, [2]float64{x, f(grupos[k])})
	}
	return out
}

func p95(v []float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return percentil(s, 95)
}
