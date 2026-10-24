// Package queuemetrics mide la demanda de procesamiento de la cola asynq y
// la publica en Cloud Monitoring, donde la lee el autoescalador del grupo de
// workers (Entrega 3).
//
// Por que no la CPU: con WORKER_CONCURRENCY=2 y 2 vCPU, ffmpeg satura la
// maquina igual con 2 trabajos que con 200 en cola. Lo que representa la
// demanda es cuantos trabajos hay por atender.
//
// La senal es pendientes + activos. Los activos cuentan a proposito: una cola
// sin pendientes puede tener trabajos en ejecucion, y retirar workers por
// mirar solo los pendientes cortaria transcodificaciones a medias. Los
// reintentos programados no cuentan: todavia no son trabajo ejecutable.
package queuemetrics

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
)

// MetricPrefix es el espacio de las metricas propias en Cloud Monitoring.
// El autoescalador de mooc-worker lee MetricPrefix + "queue_backlog".
const MetricPrefix = "custom.googleapis.com/mooc/"

// Inspector es la parte de asynq.Inspector que se usa; permite probar sin
// Redis.
type Inspector interface {
	Queues() ([]string, error)
	GetQueueInfo(queue string) (*asynq.QueueInfo, error)
}

// Snapshot es el estado agregado de las colas en un instante.
type Snapshot struct {
	Pending int
	Active  int
	Retry   int
	// OldestPending es la antiguedad del trabajo pendiente mas viejo entre
	// todas las colas: cuanto lleva esperando lo que nadie ha tomado.
	OldestPending time.Duration
}

// Backlog es la senal del autoescalador.
func (s Snapshot) Backlog() int { return s.Pending + s.Active }

func (s Snapshot) String() string {
	return fmt.Sprintf("backlog=%d pendientes=%d activos=%d reintentos=%d antiguedad=%s",
		s.Backlog(), s.Pending, s.Active, s.Retry, s.OldestPending.Round(time.Second))
}

// Collect suma las colas indicadas. Una cola que nunca recibio trabajos no
// existe en Redis y cuenta como vacia: asynq responde con un error interno si
// se le pide su informacion, asi que primero se listan las existentes.
func Collect(insp Inspector, queues []string) (Snapshot, error) {
	existing, err := insp.Queues()
	if err != nil {
		return Snapshot{}, fmt.Errorf("listar colas: %w", err)
	}
	present := make(map[string]bool, len(existing))
	for _, q := range existing {
		present[q] = true
	}

	var s Snapshot
	for _, q := range queues {
		if !present[q] {
			continue
		}
		info, err := insp.GetQueueInfo(q)
		if err != nil {
			return Snapshot{}, fmt.Errorf("cola %s: %w", q, err)
		}
		s.Pending += info.Pending
		s.Active += info.Active
		s.Retry += info.Retry
		if info.Latency > s.OldestPending {
			s.OldestPending = info.Latency
		}
	}
	return s, nil
}
