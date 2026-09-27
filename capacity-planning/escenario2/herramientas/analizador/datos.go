package main

// Lectura de los archivos que deja una corrida (ver correr_nivel.sh):
//
//	metadata.json         condiciones de la corrida
//	consola_k6.log        una linea "E2_CARGA {...}" por cada carga de profesor
//	crudo_k6.json.gz      todos los puntos de metricas de k6 (salida --out json)
//	cola.csv              muestras de la cola (monitorcola)
//	tareas_eventos.csv    primera vez que se vio cada trabajo pendiente/activo
//	tareas_final.csv      estado final de cada trabajo (CompletedAt, reintentos)
//	recursos_*.csv        muestras de CPU/memoria/red/disco (monitor_recursos.sh)
//
// Cualquier archivo que falte se reporta como "sin datos" y el analisis sigue:
// en la nube, los CSV de la Worker Server se copian despues de la corrida.

import (
	"bufio"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Punto es una muestra de una metrica de k6.
type Punto struct {
	T    int64 // unix ms
	V    float64
	Tags map[string]string
}

// Carga es el registro que k6 imprime por cada carga de un profesor.
type Carga struct {
	Iter                int     `json:"iter"`
	Perfil              string  `json:"perfil"`
	AssetID             string  `json:"asset_id"`
	TamanoBytes         int64   `json:"tamano_bytes"`
	Partes              int     `json:"partes"`
	EstadoFinal         string  `json:"estado_final"`
	Motivo              string  `json:"motivo"`
	TInicio             int64   `json:"t_inicio"`
	TConfirmado         int64   `json:"t_confirmado"`
	TReady              int64   `json:"t_ready"`
	APIFirmaMs          float64 `json:"api_firma_ms"`
	TransferenciaMs     float64 `json:"transferencia_ms"`
	ConfirmacionMs      float64 `json:"confirmacion_ms"`
	CargaAReadyMs       float64 `json:"carga_a_ready_ms"`
	ExtremoAExtremoMs   float64 `json:"extremo_a_extremo_ms"`
	EsperaHastaProcesar float64 `json:"espera_hasta_procesar_ms"`

	// Calculados al unir con los trabajos de asynq (ms; -1 = sin dato).
	ColaEscaneoMs float64 // confirmado -> escaneo terminado (espera + escaneo)
	EsperaColaHLS float64 // escaneo terminado -> transcodificacion activa
	ProcesarHLSMs float64 // transcodificacion activa -> completada
	Reintentos    int
	HLSAproximado bool // no se alcanzo a ver el trabajo "activo" (duro menos que el muestreo)
	TieneTiemposQ bool
}

// MuestraCola es una fila de cola.csv.
type MuestraCola struct {
	T                                                    int64
	Cola                                                 string
	Pend, Act, Prog, Reint, Arch, Comp, ProcTot, FallTot int
	EdadMs                                               int64
}

// Tarea resume un trabajo asynq (eventos + estado final).
type Tarea struct {
	ID, Tipo, Cola, Estado string
	PendienteVisto         int64
	ActivoVisto            int64
	CompletadoMs           int64
	Reintentos             int
	UltimoError            string
}

// MuestraRecurso es una fila de recursos_*.csv.
type MuestraRecurso struct {
	T                                  int64
	Host, Contenedor                   string
	CPU, MemMB, MemLimMB, MemPct       float64
	RxMB, TxMB, DiscoLeeMB, DiscoEscMB float64
}

// Corrida agrupa todo lo leido de una carpeta de resultados.
type Corrida struct {
	Dir       string
	Meta      map[string]any
	Puntos    map[string][]Punto // por nombre de metrica
	Cargas    []Carga
	Cola      []MuestraCola
	Tareas    map[string]*Tarea
	Recursos  []MuestraRecurso
	Resumen   map[string]any // resumen_k6.json
	T0, TFin  int64          // primer y ultimo punto de k6
	Faltantes []string
}

// metricasCrudo: solo se guardan en memoria las metricas que se analizan.
func interesa(nombre string) bool {
	return nombre == "http_req_duration" || nombre == "http_req_failed" || strings.HasPrefix(nombre, "e2_")
}

func cargarCorrida(dir string) (*Corrida, error) {
	c := &Corrida{Dir: dir, Puntos: map[string][]Punto{}, Tareas: map[string]*Tarea{}}
	c.Meta = leerJSON(filepath.Join(dir, "metadata.json"), &c.Faltantes)
	c.Resumen = leerJSON(filepath.Join(dir, "resumen_k6.json"), &c.Faltantes)
	if err := c.leerCrudo(); err != nil {
		c.Faltantes = append(c.Faltantes, "crudo_k6.json(.gz): "+err.Error())
	}
	c.leerConsola()
	c.leerCola()
	c.leerTareas()
	c.leerRecursos()
	c.unirCargasConTareas()
	return c, nil
}

func leerJSON(path string, faltantes *[]string) map[string]any {
	b, err := os.ReadFile(path)
	if err != nil {
		*faltantes = append(*faltantes, filepath.Base(path))
		return map[string]any{}
	}
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

func abrirPosibleGzip(path string) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasSuffix(path, ".gz") {
		return f, nil
	}
	gz, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	return struct {
		io.Reader
		io.Closer
	}{gz, f}, nil
}

func (c *Corrida) leerCrudo() error {
	path := filepath.Join(c.Dir, "crudo_k6.json.gz")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join(c.Dir, "crudo_k6.json")
	}
	r, err := abrirPosibleGzip(path)
	if err != nil {
		return err
	}
	defer r.Close()
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	var linea struct {
		Type   string `json:"type"`
		Metric string `json:"metric"`
		Data   struct {
			Time  string            `json:"time"`
			Value float64           `json:"value"`
			Tags  map[string]string `json:"tags"`
		} `json:"data"`
	}
	for sc.Scan() {
		b := sc.Bytes()
		// Filtro barato antes de decodificar: la mayoria de lineas son metricas que no usamos.
		if !strings.Contains(string(b[:min(len(b), 80)]), `"Point"`) {
			continue
		}
		linea.Data.Tags = nil
		if err := json.Unmarshal(b, &linea); err != nil || linea.Type != "Point" || !interesa(linea.Metric) {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, linea.Data.Time)
		if err != nil {
			continue
		}
		ms := t.UnixMilli()
		c.Puntos[linea.Metric] = append(c.Puntos[linea.Metric], Punto{T: ms, V: linea.Data.Value, Tags: linea.Data.Tags})
		if c.T0 == 0 || ms < c.T0 {
			c.T0 = ms
		}
		if ms > c.TFin {
			c.TFin = ms
		}
	}
	return sc.Err()
}

func (c *Corrida) leerConsola() {
	f, err := os.Open(filepath.Join(c.Dir, "consola_k6.log"))
	if err != nil {
		c.Faltantes = append(c.Faltantes, "consola_k6.log")
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 4*1024*1024)
	for sc.Scan() {
		var l struct {
			Msg string `json:"msg"`
		}
		if json.Unmarshal(sc.Bytes(), &l) != nil || !strings.HasPrefix(l.Msg, "E2_CARGA ") {
			continue
		}
		var cg Carga
		if json.Unmarshal([]byte(strings.TrimPrefix(l.Msg, "E2_CARGA ")), &cg) == nil {
			c.Cargas = append(c.Cargas, cg)
		}
	}
	sort.Slice(c.Cargas, func(i, j int) bool { return c.Cargas[i].TInicio < c.Cargas[j].TInicio })
}

func leerCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	filas, err := r.ReadAll()
	if err != nil || len(filas) == 0 {
		return nil, err
	}
	out := make([]map[string]string, 0, len(filas)-1)
	for _, fila := range filas[1:] {
		m := map[string]string{}
		for i, h := range filas[0] {
			if i < len(fila) {
				m[h] = fila[i]
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func atoi(s string) int     { n, _ := strconv.Atoi(strings.TrimSpace(s)); return n }
func atoi64(s string) int64 { n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64); return n }
func atof(s string) float64 { n, _ := strconv.ParseFloat(strings.TrimSpace(s), 64); return n }

func (c *Corrida) leerCola() {
	filas, err := leerCSV(filepath.Join(c.Dir, "cola.csv"))
	if err != nil || filas == nil {
		c.Faltantes = append(c.Faltantes, "cola.csv")
		return
	}
	for _, f := range filas {
		c.Cola = append(c.Cola, MuestraCola{T: atoi64(f["ts_ms"]), Cola: f["cola"], Pend: atoi(f["pendientes"]),
			Act: atoi(f["activos"]), Prog: atoi(f["programados"]), Reint: atoi(f["reintentos"]),
			Arch: atoi(f["archivados"]), Comp: atoi(f["completados"]), ProcTot: atoi(f["procesados_total"]),
			FallTot: atoi(f["fallidos_total"]), EdadMs: atoi64(f["antiguedad_ms"])})
	}
}

func (c *Corrida) tarea(id string) *Tarea {
	t, ok := c.Tareas[id]
	if !ok {
		t = &Tarea{ID: id}
		c.Tareas[id] = t
	}
	return t
}

func (c *Corrida) leerTareas() {
	if filas, err := leerCSV(filepath.Join(c.Dir, "tareas_eventos.csv")); err == nil && filas != nil {
		for _, f := range filas {
			t := c.tarea(f["task_id"])
			t.Tipo, t.Cola = f["tipo"], f["cola"]
			ts := atoi64(f["ts_ms"])
			switch f["evento"] {
			case "pendiente_visto":
				t.PendienteVisto = ts
			case "activo_visto":
				t.ActivoVisto = ts
			}
		}
	} else {
		c.Faltantes = append(c.Faltantes, "tareas_eventos.csv")
	}
	if filas, err := leerCSV(filepath.Join(c.Dir, "tareas_final.csv")); err == nil && filas != nil {
		for _, f := range filas {
			t := c.tarea(f["task_id"])
			t.Tipo, t.Cola, t.Estado = f["tipo"], f["cola"], f["estado"]
			t.Reintentos = atoi(f["reintentos"])
			t.CompletadoMs = atoi64(f["completado_ts_ms"])
			t.UltimoError = f["ultimo_error"]
		}
	} else {
		c.Faltantes = append(c.Faltantes, "tareas_final.csv")
	}
}

func (c *Corrida) leerRecursos() {
	archivos, _ := filepath.Glob(filepath.Join(c.Dir, "recursos_*.csv"))
	if len(archivos) == 0 {
		c.Faltantes = append(c.Faltantes, "recursos_*.csv")
	}
	for _, a := range archivos {
		filas, err := leerCSV(a)
		if err != nil {
			continue
		}
		for _, f := range filas {
			c.Recursos = append(c.Recursos, MuestraRecurso{T: atoi64(f["ts_ms"]), Host: f["host"],
				Contenedor: f["contenedor"], CPU: atof(f["cpu_pct"]), MemMB: atof(f["mem_mb"]),
				MemLimMB: atof(f["mem_limite_mb"]), MemPct: atof(f["mem_pct"]), RxMB: atof(f["red_rx_mb"]),
				TxMB: atof(f["red_tx_mb"]), DiscoLeeMB: atof(f["disco_lectura_mb"]), DiscoEscMB: atof(f["disco_escritura_mb"])})
		}
	}
}

// unirCargasConTareas calcula, por carga, los tiempos de cola y de
// procesamiento usando los IDs deterministas de la API: "scan:<asset>" y
// "hls:<asset>" (internal/queue/queue.go).
//
//	cola+escaneo  = scan completado  - confirmado (k6)
//	espera HLS    = hls visto activo - scan completado
//	procesar HLS  = hls completado   - hls visto activo
//
// CompletedAt de asynq tiene resolucion de 1 s y "activo" la del muestreo
// (2 s); por eso los valores negativos pequenos se llevan a 0. Si un trabajo
// duro menos que el muestreo y nunca se vio activo, se toma todo el tramo
// scan-completado -> hls-completado como procesamiento (marcado aproximado).
func (c *Corrida) unirCargasConTareas() {
	for i := range c.Cargas {
		cg := &c.Cargas[i]
		cg.ColaEscaneoMs, cg.EsperaColaHLS, cg.ProcesarHLSMs = -1, -1, -1
		if cg.AssetID == "" {
			continue
		}
		scan, okS := c.Tareas["scan:"+cg.AssetID]
		hls, okH := c.Tareas["hls:"+cg.AssetID]
		if okS {
			cg.Reintentos += scan.Reintentos
		}
		if okH {
			cg.Reintentos += hls.Reintentos
		}
		if !okS || scan.CompletadoMs == 0 || cg.TConfirmado == 0 {
			continue
		}
		cg.TieneTiemposQ = true
		cg.ColaEscaneoMs = max(0, float64(scan.CompletadoMs-cg.TConfirmado))
		if !okH || hls.CompletadoMs == 0 {
			continue
		}
		if hls.ActivoVisto > 0 {
			cg.EsperaColaHLS = max(0, float64(hls.ActivoVisto-scan.CompletadoMs))
			cg.ProcesarHLSMs = max(0, float64(hls.CompletadoMs-hls.ActivoVisto))
		} else {
			cg.EsperaColaHLS = 0
			cg.ProcesarHLSMs = max(0, float64(hls.CompletadoMs-scan.CompletadoMs))
			cg.HLSAproximado = true
		}
	}
}

// metaStr devuelve un valor de metadata.json como texto (o "?").
func (c *Corrida) metaStr(claves ...string) string {
	var v any = c.Meta
	for _, k := range claves {
		m, ok := v.(map[string]any)
		if !ok {
			return "?"
		}
		v = m[k]
	}
	switch x := v.(type) {
	case nil:
		return "?"
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}
