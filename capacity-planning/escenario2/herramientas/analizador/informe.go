package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Nombres de operacion: deben coincidir con OP en k6/escenario2.js.
const (
	opInit     = "POST /uploads (autorizar y firmar)"
	opParte    = "PUT parte (almacenamiento)"
	opComplete = "POST /uploads/:id/complete (confirmar)"
	opRecurso  = "POST /units/:id/resources"
	opEstado   = "GET /assets/:id (consultar estado)"
	opMaster   = "GET playlist maestro"
	opVariante = "GET playlist variante"
	opSegmento = "GET segmento (almacenamiento)"
)

var nan = math.NaN()

type Umbral struct {
	Metrica, Expr string
	OK            bool
}

type FilaRecurso struct {
	Host, Contenedor             string
	CPUProm, CPUP95, CPUMax      float64
	MemMaxMB, MemMaxPct          float64
	RedRxMB, RedTxMB, DiscoEscMB float64
}

// Indicadores: las cifras de una corrida que van a las tablas.
type Indicadores struct {
	c                                 *Corrida
	Nombre, Nivel, Modo               string
	CargasPorMin, Espectadores        float64
	CargasTotal, CargasOK             int
	FallosPorMotivo                   map[string]int
	Firma, Confirma, Estado, Recurso  Resumen
	Parte                             Resumen
	ErrControl, ErrTransf, ErrConsumo float64
	ErrConsumoAPI                     float64
	Rechazos429                       int
	TransfMs, TransfMbps              map[string]Resumen
	Ready, E2E, ColaEsc, EsperaHLS    Resumen
	ProcHLS                           Resumen
	ReadyPerfil, ProcHLSPerfil        map[string]Resumen
	EsperaHLSPerfil, ColaEscPerfil    map[string]Resumen
	HLSAproximados, Reintentos        int
	Archivados                        int
	TrabajosPorMin                    float64
	ColaMax, EdadMaxS                 map[string]float64
	ColaFinal                         int
	DrenajeS                          float64
	Master, Variante, Segmento        Resumen
	SegmentoRend                      map[string]Resumen
	SegMbps                           Resumen
	RiesgoCorte                       float64
	ReqSegControl, ReqSegConsumo      float64
	Recursos                          []FilaRecurso
	Umbrales                          []Umbral
	Perfiles                          []string
}

func calcular(c *Corrida) Indicadores {
	ind := Indicadores{c: c, Nombre: filepath.Base(c.Dir), Nivel: c.metaStr("nivel"), Modo: c.metaStr("modo"),
		FallosPorMotivo: map[string]int{}, TransfMs: map[string]Resumen{}, TransfMbps: map[string]Resumen{},
		ReadyPerfil: map[string]Resumen{}, ProcHLSPerfil: map[string]Resumen{}, EsperaHLSPerfil: map[string]Resumen{},
		ColaEscPerfil: map[string]Resumen{}, ColaMax: map[string]float64{}, EdadMaxS: map[string]float64{},
		SegmentoRend: map[string]Resumen{}, DrenajeS: nan}
	ind.CargasPorMin = atof(c.metaStr("nivel_efectivo", "cargas_por_min"))
	ind.Espectadores = atof(c.metaStr("nivel_efectivo", "espectadores"))

	dur := c.Puntos["http_req_duration"]
	fail := c.Puntos["http_req_failed"]
	op := func(n string) Resumen { return resumir(filtrar(dur, map[string]string{"name": n})) }
	ind.Firma, ind.Confirma, ind.Estado, ind.Recurso, ind.Parte = op(opInit), op(opComplete), op(opEstado), op(opRecurso), op(opParte)
	med := func(n string) Resumen {
		return resumir(filtrar(dur, map[string]string{"name": n, "fase": "medicion"}))
	}
	ind.Master, ind.Variante, ind.Segmento = med(opMaster), med(opVariante), med(opSegmento)
	ind.ErrControl = tasa(filtrar(fail, map[string]string{"tipo": "control"}))
	ind.ErrTransf = tasa(filtrar(fail, map[string]string{"tipo": "transferencia"}))
	ind.ErrConsumo = tasa(filtrar(fail, map[string]string{"tipo": "consumo", "fase": "medicion"}))
	ind.ErrConsumoAPI = tasa(filtrar(fail, map[string]string{"tipo": "consumo_api", "fase": "medicion"}))
	ind.Rechazos429 = len(c.Puntos["e2_rechazos_429"])
	ind.SegMbps = resumir(filtrar(c.Puntos["e2_segmento_mbps"], map[string]string{"fase": "medicion"}))
	ind.RiesgoCorte = tasa(filtrar(c.Puntos["e2_riesgo_corte"], map[string]string{"fase": "medicion"}))

	rends := map[string]bool{}
	for _, p := range dur {
		if p.Tags["name"] == opSegmento && p.Tags["fase"] == "medicion" {
			rends[p.Tags["rendicion"]] = true
		}
	}
	for r := range rends {
		ind.SegmentoRend[r] = resumir(filtrar(dur, map[string]string{"name": opSegmento, "fase": "medicion", "rendicion": r}))
	}

	// Throughput de peticiones en la ventana de medicion.
	ventana := func(tipo string) float64 {
		var ts []int64
		for _, p := range dur {
			if p.Tags["tipo"] == tipo && p.Tags["fase"] == "medicion" {
				ts = append(ts, p.T)
			}
		}
		if len(ts) < 2 {
			return nan
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
		return float64(len(ts)) / (float64(ts[len(ts)-1]-ts[0]) / 1000)
	}
	ind.ReqSegControl = ventana("control")
	ind.ReqSegConsumo = ventana("consumo")

	// ------------------- Cargas y procesamiento -------------------
	perfiles := map[string]bool{}
	var ready, e2e, colaEsc, espera, proc []float64
	porPerfil := func(m map[string][]float64, k string, v float64) { m[k] = append(m[k], v) }
	rP, pP, eP, cP, tP, mP := map[string][]float64{}, map[string][]float64{}, map[string][]float64{}, map[string][]float64{}, map[string][]float64{}, map[string][]float64{}
	var primerConf, ultimoHLS int64
	var completadosHLS []int64
	for _, cg := range c.Cargas {
		ind.CargasTotal++
		perfiles[cg.Perfil] = true
		ind.Reintentos += cg.Reintentos
		if cg.TransferenciaMs > 0 {
			porPerfil(tP, cg.Perfil, cg.TransferenciaMs)
			porPerfil(mP, cg.Perfil, float64(cg.TamanoBytes)*8/1e6/(cg.TransferenciaMs/1000))
		}
		if cg.EstadoFinal != "ready" {
			ind.FallosPorMotivo[cg.Motivo]++
			continue
		}
		ind.CargasOK++
		ready = append(ready, cg.CargaAReadyMs)
		e2e = append(e2e, cg.ExtremoAExtremoMs)
		porPerfil(rP, cg.Perfil, cg.CargaAReadyMs)
		if primerConf == 0 || cg.TConfirmado < primerConf {
			primerConf = cg.TConfirmado
		}
		if cg.ColaEscaneoMs >= 0 {
			colaEsc = append(colaEsc, cg.ColaEscaneoMs)
			porPerfil(cP, cg.Perfil, cg.ColaEscaneoMs)
		}
		if cg.EsperaColaHLS >= 0 {
			espera = append(espera, cg.EsperaColaHLS)
			proc = append(proc, cg.ProcesarHLSMs)
			porPerfil(eP, cg.Perfil, cg.EsperaColaHLS)
			porPerfil(pP, cg.Perfil, cg.ProcesarHLSMs)
			if cg.HLSAproximado {
				ind.HLSAproximados++
			}
		}
		if h, ok := c.Tareas["hls:"+cg.AssetID]; ok && h.CompletadoMs > 0 {
			completadosHLS = append(completadosHLS, h.CompletadoMs)
			if h.CompletadoMs > ultimoHLS {
				ultimoHLS = h.CompletadoMs
			}
		}
	}
	ind.Ready, ind.E2E, ind.ColaEsc, ind.EsperaHLS, ind.ProcHLS = resumir(ready), resumir(e2e), resumir(colaEsc), resumir(espera), resumir(proc)
	for p := range perfiles {
		ind.Perfiles = append(ind.Perfiles, p)
		ind.ReadyPerfil[p], ind.ProcHLSPerfil[p] = resumir(rP[p]), resumir(pP[p])
		ind.EsperaHLSPerfil[p], ind.ColaEscPerfil[p] = resumir(eP[p]), resumir(cP[p])
		ind.TransfMs[p], ind.TransfMbps[p] = resumir(tP[p]), resumir(mP[p])
	}
	sort.Strings(ind.Perfiles)
	if len(completadosHLS) > 1 && ultimoHLS > primerConf {
		ind.TrabajosPorMin = float64(len(completadosHLS)) / (float64(ultimoHLS-primerConf) / 60000)
	} else {
		ind.TrabajosPorMin = nan
	}

	// ------------------- Cola -------------------
	var ultimaLlegada int64
	for _, cg := range c.Cargas {
		if cg.TInicio > ultimaLlegada {
			ultimaLlegada = cg.TInicio
		}
	}
	prof := map[int64]int{}
	for _, m := range c.Cola {
		d := m.Pend + m.Act + m.Reint + m.Prog
		if float64(d) > ind.ColaMax[m.Cola] {
			ind.ColaMax[m.Cola] = float64(d)
		}
		if s := float64(m.EdadMs) / 1000; s > ind.EdadMaxS[m.Cola] {
			ind.EdadMaxS[m.Cola] = s
		}
		if m.Cola == "default" || m.Cola == "low" {
			prof[m.T] += d
		}
		if m.Arch > ind.Archivados {
			ind.Archivados = m.Arch
		}
	}
	if len(prof) > 0 && ultimaLlegada > 0 {
		ts := make([]int64, 0, len(prof))
		for t := range prof {
			ts = append(ts, t)
		}
		sort.Slice(ts, func(i, j int) bool { return ts[i] < ts[j] })
		ind.ColaFinal = prof[ts[len(ts)-1]]
		if ind.ColaFinal == 0 {
			var ultimoOcupado int64
			for _, t := range ts {
				if prof[t] > 0 {
					ultimoOcupado = t
				}
			}
			for _, t := range ts {
				if t > ultimoOcupado && t >= ultimaLlegada {
					ind.DrenajeS = math.Max(0, float64(t-ultimaLlegada)/1000)
					break
				}
			}
		}
	}

	// ------------------- Recursos -------------------
	tIni, tFin := c.T0, c.TFin
	if len(c.Cola) > 0 && c.Cola[len(c.Cola)-1].T > tFin {
		tFin = c.Cola[len(c.Cola)-1].T
	}
	grupos := map[string][]MuestraRecurso{}
	for _, r := range c.Recursos {
		if tIni > 0 && (r.T < tIni-10000 || r.T > tFin+10000) {
			continue
		}
		k := r.Host + "|" + r.Contenedor
		grupos[k] = append(grupos[k], r)
	}
	for k, rs := range grupos {
		sort.Slice(rs, func(i, j int) bool { return rs[i].T < rs[j].T })
		var cpu []float64
		f := FilaRecurso{Host: strings.Split(k, "|")[0], Contenedor: strings.Split(k, "|")[1]}
		for _, r := range rs {
			cpu = append(cpu, r.CPU)
			f.MemMaxMB = math.Max(f.MemMaxMB, r.MemMB)
			f.MemMaxPct = math.Max(f.MemMaxPct, r.MemPct)
		}
		s := resumir(cpu)
		f.CPUProm, f.CPUP95, f.CPUMax = s.Prom, s.P95, s.Max
		f.RedRxMB = math.Max(0, rs[len(rs)-1].RxMB-rs[0].RxMB)
		f.RedTxMB = math.Max(0, rs[len(rs)-1].TxMB-rs[0].TxMB)
		f.DiscoEscMB = math.Max(0, rs[len(rs)-1].DiscoEscMB-rs[0].DiscoEscMB)
		ind.Recursos = append(ind.Recursos, f)
	}
	sort.Slice(ind.Recursos, func(i, j int) bool {
		a, b := ind.Recursos[i], ind.Recursos[j]
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.CPUMax > b.CPUMax
	})

	// ------------------- Umbrales de k6 -------------------
	if ms, ok := c.Resumen["metrics"].(map[string]any); ok {
		for nombre, v := range ms {
			m, _ := v.(map[string]any)
			th, _ := m["thresholds"].(map[string]any)
			for expr, fallo := range th {
				f, _ := fallo.(bool) // en --summary-export, true = el umbral se cruzo
				ind.Umbrales = append(ind.Umbrales, Umbral{nombre, expr, !f})
			}
		}
		sort.Slice(ind.Umbrales, func(i, j int) bool { return ind.Umbrales[i].Metrica < ind.Umbrales[j].Metrica })
	}
	return ind
}

// Busca un contenedor por fragmento de nombre (api, worker...).
func (ind Indicadores) recurso(fragmento string) (FilaRecurso, bool) {
	for _, r := range ind.Recursos {
		if r.Contenedor != "host" && strings.Contains(r.Contenedor, fragmento) {
			return r, true
		}
	}
	return FilaRecurso{}, false
}

// ------------------------- Formato -------------------------
func ms(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	if v >= 60000 {
		return fmt.Sprintf("%.1f min", v/60000)
	}
	if v >= 10000 {
		return fmt.Sprintf("%.1f s", v/1000)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.2f s", v/1000)
	}
	return fmt.Sprintf("%.0f ms", v)
}
func pct(v float64) string {
	if math.IsNaN(v) {
		return "—"
	}
	return fmt.Sprintf("%.2f %%", v*100)
}
func num(v float64, dec int) string {
	if math.IsNaN(v) {
		return "—"
	}
	return strconv.FormatFloat(v, 'f', dec, 64)
}
func filaRes(nombre string, r Resumen) string {
	if r.N == 0 {
		return fmt.Sprintf("| %s | 0 | — | — | — | — |", nombre)
	}
	return fmt.Sprintf("| %s | %d | %s | %s | %s | %s |", nombre, r.N, ms(r.P50), ms(r.P95), ms(r.P99), ms(r.Max))
}

const cabRes = "| Operacion | n | p50 | p95 | p99 | max |\n|---|---:|---:|---:|---:|---:|"

func escribirAnalisis(c *Corrida, ind Indicadores) error {
	dirG := filepath.Join(c.Dir, "graficas")
	if err := os.MkdirAll(dirG, 0o755); err != nil {
		return err
	}
	graficas := generarGraficas(c, ind, dirG)
	if err := escribirCargasCSV(c); err != nil {
		return err
	}

	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("# Escenario 2 - Analisis de la corrida `%s`", ind.Nombre)
	w("")
	w("> Generado por `herramientas/analizador` el %s a partir de los archivos crudos de esta carpeta. No editar a mano: volver a generar.", time.Now().Format("2006-01-02 15:04"))
	w("")
	w("## Condiciones")
	w("")
	w("| Parametro | Valor |\n|---|---|")
	for _, k := range [][2]string{{"Nivel", "nivel"}, {"Modo (local/nube)", "modo"}, {"Inicio", "inicio"}, {"API", "api_url"},
		{"Version de k6", "k6_version"}, {"Commit evaluado", "git_commit"}, {"Concurrencia de workers", "worker_concurrency"},
		{"Generador de carga", "generador"}} {
		w("| %s | %s |", k[0], strings.ReplaceAll(c.metaStr(k[1]), "|", "/"))
	}
	w("| Cargas por minuto | %s |", c.metaStr("nivel_efectivo", "cargas_por_min"))
	w("| Espectadores concurrentes | %s |", c.metaStr("nivel_efectivo", "espectadores"))
	w("| Calentamiento / duracion | %s / %s |", c.metaStr("nivel_efectivo", "calentamiento"), c.metaStr("nivel_efectivo", "duracion"))
	w("")

	w("## Resumen")
	w("")
	w("| Indicador | Valor |\n|---|---|")
	w("| Cargas iniciadas / llegaron a `ready` y validadas | %d / %d |", ind.CargasTotal, ind.CargasOK)
	if len(ind.FallosPorMotivo) > 0 {
		var fs []string
		for k, v := range ind.FallosPorMotivo {
			fs = append(fs, fmt.Sprintf("%s=%d", k, v))
		}
		sort.Strings(fs)
		w("| Cargas fallidas (motivo) | %s |", strings.Join(fs, ", "))
	}
	w("| API: autorizar y firmar p95 | %s |", ms(ind.Firma.P95))
	w("| API: confirmar carga p95 | %s |", ms(ind.Confirma.P95))
	w("| Carga completa -> `ready` p50 / p95 / max | %s / %s / %s |", ms(ind.Ready.P50), ms(ind.Ready.P95), ms(ind.Ready.Max))
	w("| Espera en cola de transcodificacion p95 / max | %s / %s |", ms(ind.EsperaHLS.P95), ms(ind.EsperaHLS.Max))
	w("| Trabajos HLS completados por minuto | %s |", num(ind.TrabajosPorMin, 2))
	w("| Profundidad maxima de la cola `low` (HLS) / antiguedad maxima | %s / %s |", num(ind.ColaMax["low"], 0), ms(ind.EdadMaxS["low"]*1000))
	if math.IsNaN(ind.DrenajeS) {
		w("| Drenaje de la cola tras la ultima carga | no se observo el drenaje (quedan %d trabajos al final del monitoreo) |", ind.ColaFinal)
	} else {
		w("| Drenaje de la cola tras la ultima carga | %s |", ms(ind.DrenajeS*1000))
	}
	w("| Segmento HLS p50 / p95 / p99 | %s / %s / %s |", ms(ind.Segmento.P50), ms(ind.Segmento.P95), ms(ind.Segmento.P99))
	w("| Riesgo estimado de corte de reproduccion | %s |", pct(ind.RiesgoCorte))
	w("| Errores: control / transferencia / segmentos / manifiestos | %s / %s / %s / %s |", pct(ind.ErrControl), pct(ind.ErrTransf), pct(ind.ErrConsumo), pct(ind.ErrConsumoAPI))
	w("| Rechazos por limite de tasa (429, esperados) | %d |", ind.Rechazos429)
	if r, ok := ind.recurso("worker"); ok {
		w("| CPU contenedor worker prom / max | %s %% / %s %% |", num(r.CPUProm, 1), num(r.CPUMax, 1))
	}
	if r, ok := ind.recurso("api"); ok {
		w("| CPU contenedor api prom / max | %s %% / %s %% |", num(r.CPUProm, 1), num(r.CPUMax, 1))
	}
	w("")
	if len(ind.Umbrales) > 0 {
		w("### Criterios de exito (umbrales declarados en k6)")
		w("")
		w("| Metrica | Umbral | Resultado |\n|---|---|---|")
		for _, u := range ind.Umbrales {
			res := "cumple"
			if !u.OK {
				res = "**NO cumple**"
			}
			w("| `%s` | `%s` | %s |", u.Metrica, u.Expr, res)
		}
		w("")
	}

	w("## 1. Plano de control de la API (trafico que SI pasa por la API)")
	w("")
	w(cabRes)
	w(filaRes(opInit, ind.Firma))
	w(filaRes(opComplete, ind.Confirma))
	w(filaRes(opRecurso, ind.Recurso))
	w(filaRes(opEstado, ind.Estado))
	w(filaRes(opMaster+" (estudiantes)", ind.Master))
	w(filaRes(opVariante+" (estudiantes)", ind.Variante))
	w("")
	w("Peticiones de control por segundo (medicion): %s. Tasa de error de control: %s.", num(ind.ReqSegControl, 2), pct(ind.ErrControl))
	w("")

	w("## 2. Transferencia directa al almacenamiento de objetos (NO pasa por la API)")
	w("")
	w("| Perfil | n | transferencia p50 | p95 | max | Mbps p50 | Mbps min |\n|---|---:|---:|---:|---:|---:|---:|")
	for _, p := range ind.Perfiles {
		t, m := ind.TransfMs[p], ind.TransfMbps[p]
		w("| %s | %d | %s | %s | %s | %s | %s |", p, t.N, ms(t.P50), ms(t.P95), ms(t.Max), num(m.P50, 1), num(m.Min, 1))
	}
	w("")
	w(cabRes)
	w(filaRes(opParte+" (cada parte de 8 MiB)", ind.Parte))
	w("")
	w("Tasa de error de transferencia: %s.", pct(ind.ErrTransf))
	w("")

	w("## 3. Procesamiento asincrono (worker)")
	w("")
	w("Tiempos por carga obtenidos uniendo el registro de k6 con los trabajos de asynq (`scan:<asset>`, `hls:<asset>`). Resolucion: ~2 s (muestreo) y 1 s (`CompletedAt`).")
	w("")
	w("| Perfil | n ok | cola+escaneo p50 | espera cola HLS p50 | p95 | transcodificacion p50 | p95 | carga -> ready p50 | p95 | max |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, p := range ind.Perfiles {
		r, e, pr, ce := ind.ReadyPerfil[p], ind.EsperaHLSPerfil[p], ind.ProcHLSPerfil[p], ind.ColaEscPerfil[p]
		w("| %s | %d | %s | %s | %s | %s | %s | %s | %s | %s |", p, r.N, ms(ce.P50), ms(e.P50), ms(e.P95), ms(pr.P50), ms(pr.P95), ms(r.P50), ms(r.P95), ms(r.Max))
	}
	w("| **Todas** | %d | %s | %s | %s | %s | %s | %s | %s | %s |", ind.Ready.N, ms(ind.ColaEsc.P50), ms(ind.EsperaHLS.P50), ms(ind.EsperaHLS.P95),
		ms(ind.ProcHLS.P50), ms(ind.ProcHLS.P95), ms(ind.Ready.P50), ms(ind.Ready.P95), ms(ind.Ready.Max))
	w("")
	w("- Reintentos de trabajos de estas cargas: %d. Trabajos en DLQ (archivados) al final: %d.", ind.Reintentos, ind.Archivados)
	if ind.HLSAproximados > 0 {
		w("- %d trabajos HLS terminaron entre dos muestras (nunca se vieron activos): su espera se tomo como 0 y todo el tramo como procesamiento.", ind.HLSAproximados)
	}
	w("- Extremo a extremo (inicio de la carga -> `ready`) p50 / p95: %s / %s.", ms(ind.E2E.P50), ms(ind.E2E.P95))
	w("")

	w("## 4. Cola de mensajeria (Redis + asynq)")
	w("")
	w("| Cola | Uso | profundidad max (pend+activos+reint) | antiguedad max del pendiente mas viejo |\n|---|---|---:|---:|")
	usos := map[string]string{"critical": "correos, insignias", "default": "escaneo (media:scan)", "low": "transcodificacion HLS (media:transcode)"}
	for _, q := range []string{"critical", "default", "low"} {
		if _, ok := ind.ColaMax[q]; ok {
			w("| %s | %s | %s | %s |", q, usos[q], num(ind.ColaMax[q], 0), ms(ind.EdadMaxS[q]*1000))
		}
	}
	w("")
	w("Trabajos HLS completados por minuto (desde la primera confirmacion hasta el ultimo `ready`): %s.", num(ind.TrabajosPorMin, 2))
	w("")

	w("## 5. Consumo HLS (estudiantes)")
	w("")
	w(cabRes)
	w(filaRes(opMaster, ind.Master))
	w(filaRes(opVariante, ind.Variante))
	w(filaRes(opSegmento+" (todas)", ind.Segmento))
	var rs []string
	for r := range ind.SegmentoRend {
		rs = append(rs, r)
	}
	sort.Strings(rs)
	for _, r := range rs {
		w(filaRes("  segmento "+r, ind.SegmentoRend[r]))
	}
	w("")
	w("- Tasa de transferencia por segmento p50: %s Mbps. Errores en segmentos: %s; en manifiestos: %s.", num(ind.SegMbps.P50, 1), pct(ind.ErrConsumo), pct(ind.ErrConsumoAPI))
	w("- Riesgo estimado de corte: %s de los segmentos llegaron despues del momento en que la reproduccion simulada los necesitaba. Es una estimacion a partir de peticiones HTTP; NO es tiempo hasta el primer cuadro ni interrupciones medidas con un reproductor.", pct(ind.RiesgoCorte))
	w("- Segmentos por segundo (medicion): %s.", num(ind.ReqSegConsumo, 2))
	w("")

	w("## 6. Recursos de las maquinas y contenedores")
	w("")
	if len(ind.Recursos) == 0 {
		w("Sin datos de recursos (faltan `recursos_*.csv`).")
	} else {
		w("| Maquina | Contenedor | CPU prom %% | CPU p95 %% | CPU max %% | Mem max MB | Mem max %% | Red rx MB | Red tx MB | Disco escrito MB |\n|---|---|---:|---:|---:|---:|---:|---:|---:|---:|")
		for _, r := range ind.Recursos {
			w("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |", r.Host, r.Contenedor, num(r.CPUProm, 1), num(r.CPUP95, 1), num(r.CPUMax, 1),
				num(r.MemMaxMB, 0), num(r.MemMaxPct, 1), num(r.RedRxMB, 1), num(r.RedTxMB, 1), num(r.DiscoEscMB, 1))
		}
		w("")
		w("Nota: en `docker stats` 100 %% = un nucleo completo; un contenedor puede llegar a 200 %% en una maquina de 2 vCPU. La fila `host` es la maquina completa (0-100 %%).")
	}
	w("")

	w("## 7. Graficas")
	w("")
	for _, g := range graficas {
		w("![%s](graficas/%s)", g[1], g[0])
		w("")
	}
	w("## Archivos de esta corrida")
	w("")
	w("`metadata.json` (condiciones), `salida_k6.txt` y `resumen_k6.json` (resumen de k6), `reporte_k6.html` (panel de k6), `crudo_k6.json.gz` (todas las muestras), `consola_k6.log` (una linea por carga), `cola.csv`, `tareas_eventos.csv`, `tareas_final.csv` (cola), `recursos_*.csv` (maquinas) y `cargas.csv` (una fila por carga con todos sus tiempos).")
	if len(c.Faltantes) > 0 {
		w("")
		w("**Datos faltantes en esta corrida:** %s.", strings.Join(c.Faltantes, ", "))
	}
	return os.WriteFile(filepath.Join(c.Dir, "analisis.md"), []byte(b.String()), 0o644)
}

func escribirCargasCSV(c *Corrida) error {
	f, err := os.Create(filepath.Join(c.Dir, "cargas.csv"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"iter", "perfil", "asset_id", "tamano_bytes", "partes", "estado_final", "motivo", "api_firma_ms",
		"transferencia_ms", "mbps", "confirmacion_ms", "cola_y_escaneo_ms", "espera_cola_hls_ms", "transcodificacion_ms",
		"hls_aproximado", "carga_a_ready_ms", "extremo_a_extremo_ms", "reintentos", "t_inicio_ms"})
	f2 := func(v float64) string {
		if v < 0 || math.IsNaN(v) {
			return ""
		}
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	for _, cg := range c.Cargas {
		mbps := ""
		if cg.TransferenciaMs > 0 {
			mbps = strconv.FormatFloat(float64(cg.TamanoBytes)*8/1e6/(cg.TransferenciaMs/1000), 'f', 1, 64)
		}
		_ = w.Write([]string{strconv.Itoa(cg.Iter), cg.Perfil, cg.AssetID, strconv.FormatInt(cg.TamanoBytes, 10), strconv.Itoa(cg.Partes),
			cg.EstadoFinal, cg.Motivo, f2(cg.APIFirmaMs), f2(cg.TransferenciaMs), mbps, f2(cg.ConfirmacionMs), f2(cg.ColaEscaneoMs),
			f2(cg.EsperaColaHLS), f2(cg.ProcesarHLSMs), strconv.FormatBool(cg.HLSAproximado), f2(cg.CargaAReadyMs),
			f2(cg.ExtremoAExtremoMs), strconv.Itoa(cg.Reintentos), strconv.FormatInt(cg.TInicio, 10)})
	}
	w.Flush()
	return w.Error()
}

// generarGraficas devuelve [archivo, descripcion] de cada SVG creado.
func generarGraficas(c *Corrida, ind Indicadores, dir string) [][2]string {
	var out [][2]string
	t0 := c.T0
	if t0 == 0 && len(c.Cola) > 0 {
		t0 = c.Cola[0].T
	}
	minX := func(t int64) float64 { return float64(t-t0) / 60000 }
	guardar := func(archivo string, g Grafica) {
		if escribirSVG(filepath.Join(dir, archivo), g) == nil {
			out = append(out, [2]string{archivo, g.Titulo})
		}
	}

	// Cola: profundidad y antiguedad
	nombres := map[string]string{"default": "escaneo (default)", "low": "transcodificacion (low)", "critical": "correo/insignias (critical)"}
	var sp, se []Serie
	for _, q := range []string{"default", "low", "critical"} {
		var p, e [][2]float64
		for _, m := range c.Cola {
			if m.Cola == q {
				p = append(p, [2]float64{minX(m.T), float64(m.Pend + m.Act + m.Reint + m.Prog)})
				e = append(e, [2]float64{minX(m.T), float64(m.EdadMs) / 1000})
			}
		}
		if len(p) > 0 {
			sp = append(sp, Serie{nombres[q], p})
			se = append(se, Serie{nombres[q], e})
		}
	}
	guardar("cola_profundidad.svg", Grafica{Titulo: "Profundidad de la cola (pendientes + activos + reintentos)", EjeX: "minutos desde el inicio", EjeY: "trabajos", Series: sp})
	guardar("cola_antiguedad.svg", Grafica{Titulo: "Antiguedad del trabajo pendiente mas viejo", EjeX: "minutos desde el inicio", EjeY: "segundos", Series: se})

	// Carga -> ready por carga (dispersion por perfil)
	var sr []Serie
	for _, p := range ind.Perfiles {
		var pts [][2]float64
		for _, cg := range c.Cargas {
			if cg.Perfil == p && cg.EstadoFinal == "ready" {
				pts = append(pts, [2]float64{minX(cg.TInicio), cg.CargaAReadyMs / 1000})
			}
		}
		sr = append(sr, Serie{"Perfil " + p, pts})
	}
	guardar("carga_a_ready.svg", Grafica{Titulo: "Tiempo desde la carga completa hasta ready, por carga", EjeX: "minuto en que empezo la carga", EjeY: "segundos", Series: sr, Dispersion: true})

	// Trabajos HLS completados por minuto
	porMin := map[int64]int{}
	for _, cg := range c.Cargas {
		if h, ok := c.Tareas["hls:"+cg.AssetID]; ok && h.CompletadoMs > 0 {
			porMin[(h.CompletadoMs-t0)/60000]++
		}
	}
	var tm [][2]float64
	if len(porMin) > 0 {
		var maxK int64
		for k := range porMin {
			maxK = max(maxK, k)
		}
		for k := int64(0); k <= maxK; k++ {
			tm = append(tm, [2]float64{float64(k) + 0.5, float64(porMin[k])})
		}
	}
	guardar("trabajos_por_minuto.svg", Grafica{Titulo: "Transcodificaciones completadas por minuto", EjeX: "minutos desde el inicio", EjeY: "trabajos por minuto", Series: []Serie{{"completadas", tm}}})

	// Latencia de control (p95 por ventana de 30 s)
	dur := c.Puntos["http_req_duration"]
	var sc []Serie
	for _, o := range []struct{ op, nombre string }{{opInit, "autorizar y firmar"}, {opComplete, "confirmar"}, {opEstado, "consultar estado"}, {opMaster, "playlist maestro"}} {
		sc = append(sc, Serie{o.nombre, serieVentanas(dur, map[string]string{"name": o.op}, t0, 30000, p95)})
	}
	guardar("latencia_control_p95.svg", Grafica{Titulo: "API: latencia p95 por ventana de 30 s", EjeX: "minutos desde el inicio", EjeY: "milisegundos", Series: sc})

	// Segmentos
	seg := serieVentanas(dur, map[string]string{"name": opSegmento}, t0, 30000, p95)
	guardar("segmentos_p95.svg", Grafica{Titulo: "Segmentos HLS: latencia p95 por ventana de 30 s", EjeX: "minutos desde el inicio", EjeY: "milisegundos",
		Series: []Serie{{"segmento p95", seg}}, Referencias: []Ref{{2000, "umbral p95 < 2000 ms"}}})

	// CPU y memoria: contenedores con mas CPU (max 4) y, aparte, las maquinas completas.
	var conts, hosts []FilaRecurso
	for _, r := range ind.Recursos {
		if r.Contenedor == "host" {
			hosts = append(hosts, r)
		} else {
			conts = append(conts, r)
		}
	}
	sort.Slice(conts, func(i, j int) bool { return conts[i].CPUProm > conts[j].CPUProm })
	if len(conts) > 4 {
		conts = conts[:4]
	}
	serieRec := func(fs []FilaRecurso, campo func(MuestraRecurso) float64, etiqueta func(FilaRecurso) string) []Serie {
		var ss []Serie
		for _, f := range fs {
			var pts [][2]float64
			for _, m := range c.Recursos {
				if m.Host == f.Host && m.Contenedor == f.Contenedor && m.T >= t0-10000 {
					pts = append(pts, [2]float64{minX(m.T), campo(m)})
				}
			}
			ss = append(ss, Serie{etiqueta(f), pts})
		}
		return ss
	}
	nomCont := func(f FilaRecurso) string { return f.Contenedor }
	nomHost := func(f FilaRecurso) string { return f.Host }
	if len(conts) > 0 {
		guardar("cpu_contenedores.svg", Grafica{Titulo: "CPU por contenedor (100 % = 1 nucleo)", EjeX: "minutos desde el inicio", EjeY: "% CPU",
			Series: serieRec(conts, func(m MuestraRecurso) float64 { return m.CPU }, nomCont)})
		guardar("memoria_contenedores.svg", Grafica{Titulo: "Memoria por contenedor", EjeX: "minutos desde el inicio", EjeY: "MB",
			Series: serieRec(conts, func(m MuestraRecurso) float64 { return m.MemMB }, nomCont)})
	}
	if len(hosts) > 0 {
		guardar("cpu_maquinas.svg", Grafica{Titulo: "CPU de cada maquina completa", EjeX: "minutos desde el inicio", EjeY: "% CPU",
			Series: serieRec(hosts, func(m MuestraRecurso) float64 { return m.CPU }, nomHost)})
	}
	return out
}

// ------------------------- Comparativa entre niveles -------------------------
func escribirComparativa(dir string, inds []Indicadores) error {
	var b strings.Builder
	w := func(f string, a ...any) { fmt.Fprintf(&b, f+"\n", a...) }
	w("# Escenario 2 - Comparativa de niveles")
	w("")
	w("> Generado por `herramientas/analizador -comparar` el %s.", time.Now().Format("2006-01-02 15:04"))
	w("")
	w("## Carga y procesamiento")
	w("")
	w("| Corrida | Nivel | Cargas/min | Cargas ok/total | Firma p95 | Confirmar p95 | Estado p95 | Carga->ready p50 | p95 | Espera cola HLS p95 | Cola low max | Antiguedad max | Trabajos/min | Drenaje | CPU worker max %% |")
	w("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, i := range inds {
		cpuW := "—"
		if r, ok := i.recurso("worker"); ok {
			cpuW = num(r.CPUMax, 0)
		}
		dren := "no dreno"
		if !math.IsNaN(i.DrenajeS) {
			dren = ms(i.DrenajeS * 1000)
		}
		w("| %s | %s | %s | %d/%d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |", i.Nombre, i.Nivel, num(i.CargasPorMin, 2), i.CargasOK, i.CargasTotal,
			ms(i.Firma.P95), ms(i.Confirma.P95), ms(i.Estado.P95), ms(i.Ready.P50), ms(i.Ready.P95), ms(i.EsperaHLS.P95),
			num(i.ColaMax["low"], 0), ms(i.EdadMaxS["low"]*1000), num(i.TrabajosPorMin, 2), dren, cpuW)
	}
	w("")
	w("## Consumo")
	w("")
	w("| Corrida | Espectadores | Maestro p95 | Variante p95 | Segmento p50 | p95 | p99 | Errores segmentos | Riesgo de corte | Segmentos/s | CPU api max %% |")
	w("|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	for _, i := range inds {
		cpuA := "—"
		if r, ok := i.recurso("api"); ok {
			cpuA = num(r.CPUMax, 0)
		}
		w("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |", i.Nombre, num(i.Espectadores, 0), ms(i.Master.P95), ms(i.Variante.P95),
			ms(i.Segmento.P50), ms(i.Segmento.P95), ms(i.Segmento.P99), pct(i.ErrConsumo), pct(i.RiesgoCorte), num(i.ReqSegConsumo, 2), cpuA)
	}
	w("")

	// Graficas: una metrica por grafica, x = variable que se incrementa.
	var graf [][2]string
	guardar := func(archivo string, g Grafica) {
		if escribirSVG(filepath.Join(dir, archivo), g) == nil {
			graf = append(graf, [2]string{archivo, g.Titulo})
		}
	}
	pts := func(x func(Indicadores) float64, y func(Indicadores) float64) [][2]float64 {
		var p [][2]float64
		for _, i := range inds {
			p = append(p, [2]float64{x(i), y(i)})
		}
		return p
	}
	xCargas := func(i Indicadores) float64 { return i.CargasPorMin }
	xEsp := func(i Indicadores) float64 { return i.Espectadores }
	guardar("comp_ready.svg", Grafica{Titulo: "Carga completa -> ready segun la tasa de cargas", EjeX: "cargas por minuto", EjeY: "segundos", Series: []Serie{
		{"p50", pts(xCargas, func(i Indicadores) float64 { return i.Ready.P50 / 1000 })},
		{"p95", pts(xCargas, func(i Indicadores) float64 { return i.Ready.P95 / 1000 })}}})
	guardar("comp_cola.svg", Grafica{Titulo: "Profundidad maxima de la cola HLS segun la tasa de cargas", EjeX: "cargas por minuto", EjeY: "trabajos", Series: []Serie{
		{"cola low max", pts(xCargas, func(i Indicadores) float64 { return i.ColaMax["low"] })}}})
	guardar("comp_trabajos.svg", Grafica{Titulo: "Trabajos HLS completados por minuto vs. cargas por minuto", EjeX: "cargas por minuto (llegadas)", EjeY: "trabajos por minuto", Series: []Serie{
		{"completados/min", pts(xCargas, func(i Indicadores) float64 { return i.TrabajosPorMin })},
		{"llegadas/min", pts(xCargas, func(i Indicadores) float64 { return i.CargasPorMin })}}})
	guardar("comp_segmentos.svg", Grafica{Titulo: "Latencia de segmentos segun los espectadores", EjeX: "espectadores concurrentes", EjeY: "milisegundos", Series: []Serie{
		{"p50", pts(xEsp, func(i Indicadores) float64 { return i.Segmento.P50 })},
		{"p95", pts(xEsp, func(i Indicadores) float64 { return i.Segmento.P95 })},
		{"p99", pts(xEsp, func(i Indicadores) float64 { return i.Segmento.P99 })}}, Referencias: []Ref{{2000, "umbral p95 < 2000 ms"}}})
	guardar("comp_api.svg", Grafica{Titulo: "API: latencia p95 del plano de control segun la tasa de cargas", EjeX: "cargas por minuto", EjeY: "milisegundos", Series: []Serie{
		{"autorizar y firmar", pts(xCargas, func(i Indicadores) float64 { return i.Firma.P95 })},
		{"confirmar", pts(xCargas, func(i Indicadores) float64 { return i.Confirma.P95 })},
		{"consultar estado", pts(xCargas, func(i Indicadores) float64 { return i.Estado.P95 })}}})
	w("## Graficas")
	w("")
	for _, g := range graf {
		w("![%s](%s)", g[1], g[0])
		w("")
	}
	return os.WriteFile(filepath.Join(dir, "comparativa.md"), []byte(b.String()), 0o644)
}
