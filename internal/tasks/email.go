package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"

	"mooc-platform/internal/queue"
)

// HandleSendEmail entrega el correo por SMTP.
//
// El correo se procesa en la cola y no dentro de la peticion HTTP por dos
// razones: el servidor SMTP puede tardar o estar caido, y el usuario no tiene
// por que esperar a que su correo salga para recibir la respuesta de registro.
func (d *Deps) HandleSendEmail(ctx context.Context, t *asynq.Task) error {
	var p queue.EmailPayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		// Un payload corrupto no mejora reintentando: se descarta.
		return fmt.Errorf("payload invalido: %w: %w", err, asynq.SkipRetry)
	}
	if err := d.Mailer.Send(p.To, p.Subject, p.Body); err != nil {
		return err // si falla, asynq reintenta con backoff
	}
	return nil
}
