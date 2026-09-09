// Comando api: expone la interfaz HTTP de la plataforma.
//
// Este proceso NO ejecuta trabajo pesado. Su unica responsabilidad es
// validar peticiones, leer y escribir en Postgres, firmar URLs y PUBLICAR
// trabajos en la cola. Todo lo que tarde (transcodificar, escanear, enviar
// correo, emitir insignias) lo hace el proceso worker.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mooc-platform/internal/api"
	"mooc-platform/internal/auth"
	"mooc-platform/internal/config"
	"mooc-platform/internal/database"
	"mooc-platform/internal/queue"
	"mooc-platform/internal/storage"
	"mooc-platform/internal/telemetry"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[api] ")

	cfg := config.Load()
	log.Printf("iniciando: %s", cfg.Describe())

	// Trazas OpenTelemetry: se inicia ANTES que Postgres para que el pool ya
	// quede envuelto por el proveedor global. Si OTEL_EXPORTER_OTLP_ENDPOINT
	// esta vacio, no exporta nada y no falla.
	shutdownTracing, err := telemetry.Init(context.Background(), "mooc-api", cfg.OTelEndpoint, cfg.OTelSampleRatio)
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

	rdb, err := database.OpenRedis(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

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

	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelBoot()

	if err := store.EnsureBucket(bootCtx); err != nil {
		log.Fatalf("almacenamiento: %v", err)
	}
	if err := auth.EnsureAdmin(bootCtx, db, cfg); err != nil {
		log.Fatalf("bootstrap admin: %v", err)
	}

	q := queue.NewClient(cfg.RedisAddr, cfg.RedisPassword)
	defer q.Close()

	server := api.NewServer(cfg, db, rdb, store, q)

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: server.Router(),
		// Timeouts explicitos: sin ellos, una conexion lenta puede retener un
		// hilo indefinidamente. ReadTimeout es generoso porque el cuerpo de
		// algunas peticiones (autosave de contenido) puede ser grande, pero
		// los ARCHIVOS nunca pasan por aqui.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("escuchando en %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("servidor http: %v", err)
		}
	}()

	// Apagado ordenado: al recibir la senal, se dejan terminar las peticiones
	// en curso antes de cerrar. En un despliegue con varias instancias esto
	// evita respuestas cortadas durante un redespliegue.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("apagando...")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("apagado forzado: %v", err)
	}
	log.Println("adios")
}
