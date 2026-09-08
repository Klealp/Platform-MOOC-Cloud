package tasks

import (
	"context"
	"log"
	"time"

	"github.com/hibiken/asynq"
)

// HandleReapStale es el trabajo periodico de mantenimiento.
//
// Responde a una recomendacion explicita del enunciado: "mantener Postgres
// como fuente de verdad y REENCOLAR TRABAJOS SIN HEARTBEAT para mitigar
// perdidas en Redis".
//
// El escenario que resuelve: la API encola un escaneo, Redis se reinicia y
// pierde la cola. El asset queda para siempre en 'uploaded' y el profesor ve
// un archivo que nunca se procesa. Como Postgres SI conserva el estado real,
// aqui se detectan esos casos y se vuelven a encolar.
//
// Se ejecuta cada 5 minutos (ver el Scheduler en cmd/worker/main.go).
func (d *Deps) HandleReapStale(ctx context.Context, _ *asynq.Task) error {
	// 1. Cargas multipart vencidas: se abortan para liberar las partes que el
	//    almacenamiento guarda sin ensamblar (ocupan espacio y cuestan).
	rows, err := d.DB.QueryContext(ctx, `
		SELECT up.id, up.upload_id, a.object_key
		FROM uploads up JOIN assets a ON a.id = up.asset_id
		WHERE up.completed_at IS NULL AND up.aborted_at IS NULL AND up.expires_at < now()
		LIMIT 100`)
	if err != nil {
		return err
	}
	var expired []struct{ id, uploadID, key string }
	for rows.Next() {
		var r struct{ id, uploadID, key string }
		if err := rows.Scan(&r.id, &r.uploadID, &r.key); err != nil {
			rows.Close()
			return err
		}
		expired = append(expired, r)
	}
	rows.Close()

	for _, e := range expired {
		if err := d.Store.AbortMultipartUpload(ctx, e.key, e.uploadID); err != nil {
			log.Printf("reap: no se pudo abortar la carga %s: %v", e.id, err)
		}
		if _, err := d.DB.ExecContext(ctx,
			`UPDATE uploads SET aborted_at = now() WHERE id = $1`, e.id); err != nil {
			return err
		}
		log.Printf("reap: carga %s vencida y abortada", e.id)
	}

	// 2. Escaneos huerfanos: subidos hace rato y nunca procesados.
	if err := d.requeue(ctx,
		`SELECT id FROM assets WHERE status = 'uploaded' AND updated_at < now() - interval '10 minutes' LIMIT 50`,
		func(assetID string) error { return d.Queue.EnqueueScan(assetID, d.Cfg.MaxRetries) },
		"scan"); err != nil {
		return err
	}

	// 3. Escaneos que se quedaron a medias (el worker murio a mitad).
	if _, err := d.DB.ExecContext(ctx, `
		UPDATE assets SET status = 'uploaded', updated_at = now()
		WHERE status = 'scanning' AND updated_at < now() - interval '30 minutes'`); err != nil {
		return err
	}

	// 4. Transcodificaciones huerfanas.
	if err := d.requeue(ctx,
		`SELECT id FROM assets WHERE status = 'processing' AND updated_at < now() - interval '2 hours' LIMIT 20`,
		func(assetID string) error { return d.Queue.EnqueueTranscode(assetID, d.Cfg.MaxRetries) },
		"transcode"); err != nil {
		return err
	}

	// 5. Insignias que debieron emitirse y no estan.
	badges, err := d.DB.QueryContext(ctx, `
		SELECT e.id FROM enrollments e
		LEFT JOIN badges b ON b.enrollment_id = e.id AND b.revoked_at IS NULL
		WHERE e.state = 'approved' AND b.id IS NULL
		LIMIT 50`)
	if err != nil {
		return err
	}
	defer badges.Close()
	for badges.Next() {
		var enrollmentID string
		if err := badges.Scan(&enrollmentID); err != nil {
			return err
		}
		log.Printf("reap: reencolando insignia de la inscripcion %s", enrollmentID)
		if err := d.Queue.EnqueueBadge(enrollmentID, d.Cfg.MaxRetries); err != nil {
			log.Printf("reap: no se pudo reencolar la insignia: %v", err)
		}
	}

	// 6. Intentos de quiz vencidos que nadie envio.
	if _, err := d.DB.ExecContext(ctx, `
		UPDATE quiz_attempts SET status = 'expired', submitted_at = now(), score = 0, passed = FALSE
		WHERE status = 'in_progress' AND expires_at IS NOT NULL AND expires_at < now() - interval '5 minutes'`); err != nil {
		return err
	}

	// 7. Sesiones caducadas: se limpian para que la tabla no crezca sin fin.
	if _, err := d.DB.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < now() - interval '30 days'`); err != nil {
		return err
	}

	return nil
}

func (d *Deps) requeue(ctx context.Context, query string, enqueue func(string) error, label string) error {
	rows, err := d.DB.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		log.Printf("reap: reencolando %s del asset %s", label, id)
		if err := enqueue(id); err != nil {
			log.Printf("reap: no se pudo reencolar %s: %v", label, err)
		}
	}
	return nil
}

// ReapInterval es cada cuanto corre el mantenimiento.
const ReapInterval = 5 * time.Minute
