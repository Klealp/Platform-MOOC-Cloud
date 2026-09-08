// Package tasks contiene los handlers que ejecutan los workers.
//
// Un worker es un proceso APARTE de la API. No atiende HTTP: se suscribe a la
// cola, toma trabajos y los ejecuta. Se puede escalar solo (docker compose up
// --scale worker=3) sin tocar la API, porque la cola reparte el trabajo entre
// todos los consumidores.
//
// IDEMPOTENCIA EN DOS CAPAS:
//
//	Capa 1 (Redis/asynq): la opcion TaskID rechaza encolar dos veces la misma
//	   llave mientras el trabajo existe.
//	Capa 2 (Postgres): la tabla processed_jobs registra el trabajo terminado.
//	   Esta capa es la que importa de verdad, porque Redis puede perder
//	   estado y Postgres no. Si la cola reentrega un trabajo ya hecho, el
//	   worker lo detecta aqui y no repite el efecto.
//
// Ademas, cada handler comprueba el ESTADO del dominio antes de actuar (por
// ejemplo, que el asset siga en 'uploaded'). Esa es la defensa mas robusta:
// no depende de ningun registro auxiliar.
package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/hibiken/asynq"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/config"
	"mooc-platform/internal/mailer"
	"mooc-platform/internal/metrics"
	"mooc-platform/internal/queue"
	"mooc-platform/internal/storage"
)

type Deps struct {
	Cfg    config.Config
	DB     *sql.DB
	Store  *storage.Storage
	Queue  *queue.Client
	Mailer *mailer.Mailer
	Audit  *audit.Logger
}

// Register conecta cada tipo de tarea con su handler.
func (d *Deps) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TaskSendEmail, d.instrument(queue.TaskSendEmail, d.HandleSendEmail))
	mux.HandleFunc(queue.TaskScanAsset, d.instrument(queue.TaskScanAsset, d.HandleScanAsset))
	mux.HandleFunc(queue.TaskTranscodeHLS, d.instrument(queue.TaskTranscodeHLS, d.HandleTranscode))
	mux.HandleFunc(queue.TaskIssueBadge, d.instrument(queue.TaskIssueBadge, d.HandleIssueBadge))
	mux.HandleFunc(queue.TaskReapStale, d.instrument(queue.TaskReapStale, d.HandleReapStale))
}

type handlerFunc func(context.Context, *asynq.Task) error

// instrument envuelve cada handler para medir duracion y resultado sin
// repetir el mismo codigo en los cinco handlers.
func (d *Deps) instrument(name string, fn handlerFunc) handlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		start := time.Now()
		err := fn(ctx, t)
		metrics.JobDuration.WithLabelValues(name).Observe(time.Since(start).Seconds())

		status := "success"
		if err != nil {
			status = "retry"
		}
		metrics.JobsProcessed.WithLabelValues(name, status).Inc()

		if err != nil {
			log.Printf("worker: %s fallo: %v", name, err)
		}
		return err
	}
}

// alreadyProcessed consulta la capa 2 de idempotencia.
func (d *Deps) alreadyProcessed(ctx context.Context, jobKey string) (bool, error) {
	var exists bool
	err := d.DB.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM processed_jobs WHERE job_key = $1)`, jobKey).Scan(&exists)
	return exists, err
}

func (d *Deps) markProcessed(ctx context.Context, jobKey, taskType string, result map[string]any) {
	raw, err := json.Marshal(result)
	if err != nil {
		raw = []byte("{}")
	}
	if _, err := d.DB.ExecContext(ctx, `
		INSERT INTO processed_jobs (job_key, task_type, result)
		VALUES ($1, $2, $3)
		ON CONFLICT (job_key) DO NOTHING`, jobKey, taskType, string(raw)); err != nil {
		log.Printf("worker: no se pudo registrar processed_jobs (%s): %v", jobKey, err)
	}
}

// skipDuplicate centraliza el mensaje y la metrica de una reentrega.
func (d *Deps) skipDuplicate(taskType, jobKey string) {
	metrics.JobsProcessed.WithLabelValues(taskType, "duplicate").Inc()
	log.Printf("worker: %s ya fue procesado (%s); se ignora la reentrega", taskType, jobKey)
}

// ErrorHandler se ejecuta cuando un trabajo falla.
//
// Cuando se agotan los reintentos, asynq mueve la tarea a la cola "archived":
// esa es la DEAD LETTER QUEUE. Ahi queda para inspeccion manual. Este handler
// deja ademas una linea de auditoria, que es la ALERTA que exige el enunciado.
func (d *Deps) ErrorHandler() asynq.ErrorHandlerFunc {
	return func(ctx context.Context, task *asynq.Task, err error) {
		retried, _ := asynq.GetRetryCount(ctx)
		maxRetry, _ := asynq.GetMaxRetry(ctx)

		if retried >= maxRetry {
			metrics.JobsProcessed.WithLabelValues(task.Type(), "dead_letter").Inc()
			log.Printf("ALERTA: %s agoto %d reintentos y paso a la DLQ: %v", task.Type(), maxRetry, err)

			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			d.Audit.Log(bgCtx, "", audit.ActionJobDeadLettered, "job", task.Type(), "",
				map[string]any{
					"payload": string(task.Payload()),
					"error":   err.Error(),
					"retries": retried,
				})
			return
		}
		log.Printf("worker: %s fallo (intento %d/%d): %v", task.Type(), retried+1, maxRetry, err)
	}
}
