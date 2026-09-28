// Command monitorcola observa la cola asynq (Redis) durante una prueba de carga.
//
// Por que existe: el enunciado pide "profundidad, antiguedad y tasa de
// procesamiento de la cola" y, por cada carga, el "tiempo de espera en cola"
// y la "duracion de procesamiento". Nada de eso lo ve k6 (k6 solo ve la API),
// y asynq no guarda la hora en que un trabajo EMPIEZA a procesarse. Este
// programa consulta Redis cada -intervalo con el Inspector oficial de asynq y
// deja tres archivos CSV en -salida:
//
//	cola.csv            una fila por cola y por muestra: tamano, pendientes,
//	                    activos, reintentos, archivados (DLQ), completados,
//	                    procesados/fallidos acumulados y ANTIGUEDAD del
//	                    trabajo pendiente mas viejo (latency de asynq).
//	tareas_eventos.csv  la PRIMERA vez que se ve cada trabajo pendiente,
//	                    activo, en reintento o archivado. Como la API usa IDs
//	                    deterministas ("scan:<asset>", "hls:<asset>"), el
//	                    analizador une estos eventos con las cargas de k6.
//	tareas_final.csv    al terminar (Ctrl+C o -duracion): estado final de los
//	                    trabajos con su hora de finalizacion (CompletedAt,
//	                    que asynq conserva 24 h por Retention) y reintentos.
//
// La resolucion de "activo_visto" es el intervalo de muestreo (2 s por
// defecto); frente a transcodificaciones de decenas de segundos o minutos es
// suficiente, y se declara como limitacion en el informe.
//
// Uso (desde la raiz del repo):
//
//	go run ./capacity-planning/escenario2/herramientas/monitorcola \
//	    -redis 127.0.0.1:6379 -salida capacity-planning/escenario2/resultados/mi-corrida
//
// En la nube se compila un binario y se ejecuta en la Worker Server, que es
// la unica maquina que alcanza Redis (ver README.md).
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"syscall"
	"time"

	"github.com/hibiken/asynq"
)

// Colas que define internal/queue/queue.go. Se consultan siempre, aunque aun
// no existan en Redis, para que el CSV tenga las mismas columnas en toda corrida.
var colasBase = []string{"critical", "default", "low"}

func main() {
	redisAddr := flag.String("redis", envOr("REDIS_ADDR", "127.0.0.1:6379"), "direccion host:puerto de Redis")
	redisPass := flag.String("password", os.Getenv("REDIS_PASSWORD"), "contrasena de Redis (o variable REDIS_PASSWORD)")
	intervalo := flag.Duration("intervalo", 2*time.Second, "cada cuanto se toma una muestra")
	salida := flag.String("salida", ".", "carpeta donde se escriben los CSV")
	duracion := flag.Duration("duracion", 0, "tiempo maximo de monitoreo (0 = hasta Ctrl+C / SIGTERM)")
	flag.Parse()

	if err := os.MkdirAll(*salida, 0o755); err != nil {
		log.Fatalf("crear carpeta de salida: %v", err)
	}
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: *redisAddr, Password: *redisPass})
	defer insp.Close()

	// Verificacion temprana: si Redis no responde, mejor fallar ya que
	// descubrir al final de la corrida que el CSV esta vacio.
	if _, err := insp.Queues(); err != nil {
		log.Fatalf("no pude consultar Redis en %s: %v", *redisAddr, err)
	}

	colaW, colaF := nuevoCSV(filepath.Join(*salida, "cola.csv"),
		[]string{"ts_ms", "cola", "tamano", "pendientes", "activos", "programados", "reintentos",
			"archivados", "completados", "procesados_total", "fallidos_total", "antiguedad_ms"})
	defer colaF.Close()
	evW, evF := nuevoCSV(filepath.Join(*salida, "tareas_eventos.csv"),
		[]string{"ts_ms", "task_id", "tipo", "cola", "evento"})
	defer evF.Close()

	vistos := map[string]bool{} // clave: evento|id -> ya registrado
	registrar := func(ts int64, t *asynq.TaskInfo, evento string) {
		k := evento + "|" + t.ID
		if vistos[k] {
			return
		}
		vistos[k] = true
		_ = evW.Write([]string{i64(ts), t.ID, t.Type, t.Queue, evento})
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	var fin <-chan time.Time
	if *duracion > 0 {
		fin = time.After(*duracion)
	}
	tick := time.NewTicker(*intervalo)
	defer tick.Stop()

	log.Printf("monitorcola: Redis %s, muestra cada %s, salida %s (Ctrl+C para terminar)", *redisAddr, *intervalo, *salida)
	muestras := 0
	for {
		ts := time.Now().UnixMilli()
		resumen := ""
		for _, q := range colas(insp) {
			info, err := insp.GetQueueInfo(q)
			if err != nil {
				continue // la cola aun no existe en Redis
			}
			_ = colaW.Write([]string{i64(ts), q, itoa(info.Size), itoa(info.Pending), itoa(info.Active),
				itoa(info.Scheduled), itoa(info.Retry), itoa(info.Archived), itoa(info.Completed),
				itoa(info.ProcessedTotal), itoa(info.FailedTotal), i64(info.Latency.Milliseconds())})
			resumen += fmt.Sprintf(" %s[pend=%d act=%d retry=%d dlq=%d edad=%s]", q, info.Pending,
				info.Active, info.Retry, info.Archived, info.Latency.Round(time.Second))

			// Eventos por trabajo. PageSize alto: en esta prueba hay a lo sumo
			// cientos de trabajos vivos a la vez.
			if ts2, err := insp.ListPendingTasks(q, asynq.PageSize(5000)); err == nil {
				for _, t := range ts2 {
					registrar(ts, t, "pendiente_visto")
				}
			}
			if ts2, err := insp.ListActiveTasks(q, asynq.PageSize(5000)); err == nil {
				for _, t := range ts2 {
					registrar(ts, t, "activo_visto")
				}
			}
			if ts2, err := insp.ListRetryTasks(q, asynq.PageSize(5000)); err == nil {
				for _, t := range ts2 {
					registrar(ts, t, "reintento_visto")
				}
			}
			if ts2, err := insp.ListArchivedTasks(q, asynq.PageSize(5000)); err == nil {
				for _, t := range ts2 {
					registrar(ts, t, "archivado_visto")
				}
			}
		}
		colaW.Flush()
		evW.Flush()
		muestras++
		if muestras%5 == 1 {
			log.Printf("cola:%s", resumen)
		}

		select {
		case <-tick.C:
		case <-sig:
			escribirFinal(insp, *salida)
			return
		case <-fin:
			escribirFinal(insp, *salida)
			return
		}
	}
}

// escribirFinal vuelca el estado final de los trabajos. CompletedAt solo
// existe para los completados (asynq lo guarda con resolucion de segundos).
func escribirFinal(insp *asynq.Inspector, dir string) {
	w, f := nuevoCSV(filepath.Join(dir, "tareas_final.csv"),
		[]string{"task_id", "tipo", "cola", "estado", "reintentos", "completado_ts_ms", "ultimo_error"})
	defer f.Close()
	n := 0
	for _, q := range colas(insp) {
		listas := []func(string, ...asynq.ListOption) ([]*asynq.TaskInfo, error){
			insp.ListCompletedTasks, insp.ListArchivedTasks, insp.ListRetryTasks,
			insp.ListActiveTasks, insp.ListPendingTasks, insp.ListScheduledTasks,
		}
		for _, listar := range listas {
			for pagina := 1; ; pagina++ {
				ts, err := listar(q, asynq.PageSize(1000), asynq.Page(pagina))
				if err != nil || len(ts) == 0 {
					break
				}
				for _, t := range ts {
					comp := ""
					if !t.CompletedAt.IsZero() {
						comp = i64(t.CompletedAt.UnixMilli())
					}
					_ = w.Write([]string{t.ID, t.Type, t.Queue, t.State.String(), itoa(t.Retried), comp, t.LastErr})
					n++
				}
				if len(ts) < 1000 {
					break
				}
			}
		}
	}
	w.Flush()
	log.Printf("monitorcola: estado final de %d trabajos en %s", n, filepath.Join(dir, "tareas_final.csv"))
}

func colas(insp *asynq.Inspector) []string {
	set := map[string]bool{}
	for _, q := range colasBase {
		set[q] = true
	}
	if qs, err := insp.Queues(); err == nil {
		for _, q := range qs {
			set[q] = true
		}
	}
	out := make([]string, 0, len(set))
	for q := range set {
		out = append(out, q)
	}
	sort.Strings(out)
	return out
}

func nuevoCSV(path string, header []string) (*csv.Writer, *os.File) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("crear %s: %v", path, err)
	}
	w := csv.NewWriter(f)
	_ = w.Write(header)
	w.Flush()
	return w, f
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func itoa(n int) string  { return strconv.Itoa(n) }
func i64(n int64) string { return strconv.FormatInt(n, 10) }
