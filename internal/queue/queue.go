// Package queue define el contrato entre la API (que publica trabajos) y los
// workers (que los consumen).
//
// Usamos asynq sobre Redis porque el enunciado lo exige. Conceptos clave:
//
//   - Cola: lista de trabajos pendientes. La API publica y retorna de
//     inmediato; nunca espera al worker.
//   - Idempotencia: cada trabajo lleva una llave estable (TaskID). Si se
//     encola dos veces la misma llave, asynq rechaza la segunda. Ademas el
//     worker vuelve a comprobarlo en Postgres (tabla processed_jobs), porque
//     Redis puede perder estado y Postgres no.
//   - Backoff: al fallar, asynq reintenta con espera creciente.
//   - DLQ (dead letter queue): cuando se agotan los reintentos, asynq mueve
//     el trabajo a la cola "archived". Ahi queda para inspeccion manual y se
//     emite una alerta en audit_log.
package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// Nombres de las tareas. Son el "contrato": si cambias uno, cambialo en la
// API y en el worker a la vez.
const (
	TaskSendEmail    = "email:send"
	TaskScanAsset    = "media:scan"      // MIME real + antimalware
	TaskTranscodeHLS = "media:transcode" // video/audio -> HLS
	TaskIssueBadge   = "badge:issue"
	TaskReapStale    = "maintenance:reap" // reencola trabajos sin heartbeat
)

// Prioridades. asynq atiende "critical" mas seguido que "default" y esta mas
// que "low", segun los pesos configurados en el worker.
const (
	QueueCritical = "critical"
	QueueDefault  = "default"
	QueueLow      = "low"
)

// ------------------------- Cargas utiles -------------------------

type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type AssetPayload struct {
	AssetID string `json:"asset_id"`
}

type BadgePayload struct {
	EnrollmentID string `json:"enrollment_id"`
}

// ------------------------- Cliente -------------------------

type Client struct {
	c *asynq.Client
}

func NewClient(redisAddr, redisPassword string) *Client {
	return &Client{c: asynq.NewClient(asynq.RedisClientOpt{
		Addr:     redisAddr,
		Password: redisPassword,
	})}
}

func (q *Client) Close() error { return q.c.Close() }

// Enqueue publica un trabajo.
//
// jobKey es la llave de idempotencia. Debe derivarse del EFECTO deseado
// ("emitir la insignia de la inscripcion X"), no del momento de la llamada,
// para que dos publicaciones del mismo efecto colisionen a proposito.
func (q *Client) Enqueue(taskType, jobKey string, payload any, queueName string, maxRetries int, timeout time.Duration) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("serializar payload: %w", err)
	}

	opts := []asynq.Option{
		asynq.Queue(queueName),
		asynq.MaxRetry(maxRetries),
		asynq.Timeout(timeout),
		// Retention conserva el registro del trabajo terminado para poder
		// inspeccionarlo en la demostracion.
		asynq.Retention(24 * time.Hour),
	}
	if jobKey != "" {
		opts = append(opts, asynq.TaskID(jobKey))
	}

	_, err = q.c.Enqueue(asynq.NewTask(taskType, body), opts...)
	if err != nil {
		// Que la llave ya exista NO es un error: significa que el trabajo ya
		// estaba pedido. Es exactamente el comportamiento que buscamos.
		if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
			return nil
		}
		return fmt.Errorf("encolar %s: %w", taskType, err)
	}
	return nil
}

// Atajos para no repetir parametros en los handlers.

func (q *Client) EnqueueEmail(to, subject, body string) error {
	return q.Enqueue(TaskSendEmail, "", EmailPayload{To: to, Subject: subject, Body: body},
		QueueCritical, 5, 30*time.Second)
}

func (q *Client) EnqueueScan(assetID string, maxRetries int) error {
	return q.Enqueue(TaskScanAsset, "scan:"+assetID, AssetPayload{AssetID: assetID},
		QueueDefault, maxRetries, 5*time.Minute)
}

func (q *Client) EnqueueTranscode(assetID string, maxRetries int) error {
	return q.Enqueue(TaskTranscodeHLS, "hls:"+assetID, AssetPayload{AssetID: assetID},
		QueueLow, maxRetries, 2*time.Hour)
}

func (q *Client) EnqueueBadge(enrollmentID string, maxRetries int) error {
	return q.Enqueue(TaskIssueBadge, "badge:"+enrollmentID, BadgePayload{EnrollmentID: enrollmentID},
		QueueCritical, maxRetries, time.Minute)
}
