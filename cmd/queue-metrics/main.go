// Comando queue-metrics: publica la demanda de la cola asynq en Cloud
// Monitoring para el autoescalador del grupo de workers (Entrega 3).
//
// Corre UNA sola instancia, en infra-server junto a Redis. No puede vivir
// en el grupo de workers: el autoescalador retiraria justo a quien le
// informa, y con varias replicas habria series duplicadas.
//
// Cada QUEUE_METRICS_INTERVAL_SECONDS publica pendientes + activos
// (custom.googleapis.com/mooc/queue_backlog) y deja una linea en el log,
// que sirve de linea de tiempo para el escenario 3B del informe.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hibiken/asynq"

	"mooc-platform/internal/config"
	"mooc-platform/internal/queue"
	"mooc-platform/internal/queuemetrics"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[queue-metrics] ")

	cfg := config.Load()
	insp := asynq.NewInspector(asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword})
	defer insp.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpc := &http.Client{Timeout: 10 * time.Second}
	project := cfg.QueueMetricsProject
	if project == "" {
		p, err := queuemetrics.MetadataProject(ctx, httpc)
		if err != nil {
			log.Printf("sin proyecto de GCP (%v): solo se registra en el log", err)
		}
		project = p
	}
	pub := &queuemetrics.Publisher{Project: project, HTTP: httpc}
	queues := []string{queue.QueueCritical, queue.QueueDefault, queue.QueueLow}
	log.Printf("redis=%s proyecto=%q intervalo=%s colas=%v", cfg.RedisAddr, project, cfg.QueueMetricsInterval, queues)

	tick := time.NewTicker(cfg.QueueMetricsInterval)
	defer tick.Stop()
	for {
		medir(ctx, insp, pub, queues)
		select {
		case <-ctx.Done():
			log.Println("adios")
			return
		case <-tick.C:
		}
	}
}

// medir no detiene el proceso ante un error: si Redis o Cloud Monitoring
// fallan un ciclo, el siguiente lo reintenta. Mientras no haya datos el
// autoescalador mantiene el tamano del grupo, que es lo seguro.
func medir(ctx context.Context, insp queuemetrics.Inspector, pub *queuemetrics.Publisher, queues []string) {
	s, err := queuemetrics.Collect(insp, queues)
	if err != nil {
		log.Printf("medicion fallida: %v", err)
		return
	}
	log.Print(s)
	if pub.Project == "" {
		return
	}
	pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pub.Publish(pctx, s, time.Now()); err != nil {
		log.Printf("publicacion fallida: %v", err)
	}
}
