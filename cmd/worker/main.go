// Comando worker: consume la cola y ejecuta el trabajo pesado.
//
// Se despliega como un proceso separado de la API y se escala por su cuenta:
//
//	docker compose up --scale worker=3
//
// Cada instancia se suscribe a la misma cola de Redis; asynq reparte los
// trabajos entre todas. No hace falta coordinacion entre ellas porque ningun
// worker guarda estado propio: leen de Postgres y del almacenamiento.
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
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/config"
	"mooc-platform/internal/database"
	"mooc-platform/internal/mailer"
	"mooc-platform/internal/queue"
	"mooc-platform/internal/storage"
	"mooc-platform/internal/tasks"
	"mooc-platform/internal/telemetry"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[worker] ")

	cfg := config.Load()
	log.Printf("iniciando: %s", cfg.Describe())

	// Trazas OpenTelemetry para el worker: cada trabajo abre su propio span y
	// las consultas SQL cuelgan de el. Apagado si no hay endpoint configurado.
	shutdownTracing, err := telemetry.Init(context.Background(), "mooc-worker", cfg.OTelEndpoint, cfg.OTelSampleRatio)
	if err != nil {
		log.Printf("telemetria: %v (se continua sin trazas)", err)
		shutdownTracing = func(context.Context) error { return nil }
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(ctx)
	}()

	db, err := database.OpenPostgres(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer db.Close()

	store, err := storage.New(storage.Config{
		Endpoint:       cfg.S3Endpoint,
		PublicEndpoint: cfg.S3PublicEndpoint,
		AccessKey:      cfg.S3AccessKey,
		SecretKey:      cfg.S3SecretKey,
		Bucket:         cfg.S3Bucket,
		Region:         cfg.S3Region,
		UseSSL:         cfg.S3UseSSL,
	})
	if err != nil {
		log.Fatalf("almacenamiento: %v", err)
	}

	q := queue.NewClient(cfg.RedisAddr, cfg.RedisPassword)
	defer q.Close()

	deps := &tasks.Deps{
		Cfg:    cfg,
		DB:     db,
		Store:  store,
		Queue:  q,
		Mailer: mailer.New(cfg.SMTPAddr, cfg.SMTPFrom),
		Audit:  audit.New(db),
	}

	// Prometheus tambien raspa al worker: sin esto no habria forma de ver
	// cuantos trabajos se procesan ni cuantos caen en la DLQ.
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})
		log.Println("metricas del worker en :9100/metrics")
		if err := http.ListenAndServe(":9100", mux); err != nil {
			log.Printf("servidor de metricas: %v", err)
		}
	}()

	redisOpt := asynq.RedisClientOpt{Addr: cfg.RedisAddr, Password: cfg.RedisPassword}

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.WorkerConcurrency,
		// Pesos de las colas: por cada 6 trabajos criticos se atienden 3
		// normales y 1 de baja prioridad. Asi un correo de verificacion no
		// espera detras de una transcodificacion de dos horas.
		Queues: map[string]int{
			queue.QueueCritical: 6,
			queue.QueueDefault:  3,
			queue.QueueLow:      1,
		},
		ErrorHandler: deps.ErrorHandler(),
	})

	mux := asynq.NewServeMux()
	deps.Register(mux)

	// Scheduler: encola el mantenimiento periodico. Aunque haya varias
	// replicas del worker, asynq garantiza que la tarea programada se encole
	// una sola vez por periodo.
	scheduler := asynq.NewScheduler(redisOpt, nil)
	if _, err := scheduler.Register("@every 5m",
		asynq.NewTask(queue.TaskReapStale, nil), asynq.Queue(queue.QueueLow)); err != nil {
		log.Fatalf("scheduler: %v", err)
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			log.Printf("scheduler: %v", err)
		}
	}()

	go func() {
		log.Printf("consumiendo la cola (concurrencia=%d)", cfg.WorkerConcurrency)
		if err := srv.Run(mux); err != nil {
			log.Fatalf("asynq: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("apagando: se esperan los trabajos en curso...")
	scheduler.Shutdown()
	srv.Shutdown() // deja terminar lo que ya empezo
	log.Println("adios")
}
