package tasks

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/badge"
	"mooc-platform/internal/metrics"
	"mooc-platform/internal/queue"
)

// HandleIssueBadge emite la insignia de una inscripcion aprobada.
//
// LA CONDICION MAS DELICADA DEL ENUNCIADO: "se crea una UNICA insignia".
// Aqui hay tres defensas superpuestas, y la tercera es la que de verdad
// garantiza el resultado:
//
//	1. asynq.TaskID("badge:<enrollment>") evita encolar dos veces.
//	2. processed_jobs registra que este trabajo ya se ejecuto.
//	3. El indice UNIQUE parcial uniq_badge_alive (course_id, user_id) WHERE
//	   revoked_at IS NULL hace que la base de datos RECHACE la segunda
//	   insercion, pase lo que pase con las otras dos capas.
//
// Confiar solo en 1 y 2 seria fragil: son estado en Redis y una tabla
// auxiliar. La restriccion de integridad es la unica que no se puede
// esquivar por una carrera entre dos workers.
func (d *Deps) HandleIssueBadge(ctx context.Context, t *asynq.Task) error {
	var p queue.BadgePayload
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("payload invalido: %w: %w", err, asynq.SkipRetry)
	}
	jobKey := "badge:" + p.EnrollmentID

	done, err := d.alreadyProcessed(ctx, jobKey)
	if err != nil {
		return err
	}
	if done {
		d.skipDuplicate(queue.TaskIssueBadge, jobKey)
		return nil
	}

	var (
		courseID, userID, state, courseTitle, fullName, email string
	)
	err = d.DB.QueryRowContext(ctx, `
		SELECT e.course_id, e.user_id, e.state::text, c.title, u.full_name, u.email
		FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		JOIN users u ON u.id = e.user_id
		WHERE e.id = $1`, p.EnrollmentID).
		Scan(&courseID, &userID, &state, &courseTitle, &fullName, &email)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("inscripcion %s no existe: %w", p.EnrollmentID, asynq.SkipRetry)
	}
	if err != nil {
		return err
	}

	// Comprobacion de dominio: no se emite si la inscripcion ya no esta
	// aprobada (por ejemplo, si se publico una version con mas requisitos).
	if state != "approved" {
		d.markProcessed(ctx, jobKey, queue.TaskIssueBadge,
			map[string]any{"result": "skipped", "state": state})
		return nil
	}

	publicCode, err := newPublicCode()
	if err != nil {
		return err
	}

	var badgeID string
	var issuedAt time.Time
	err = d.DB.QueryRowContext(ctx, `
		INSERT INTO badges (enrollment_id, course_id, user_id, public_code)
		VALUES ($1, $2, $3, $4)
		RETURNING id, issued_at`,
		p.EnrollmentID, courseID, userID, publicCode).Scan(&badgeID, &issuedAt)

	if err != nil {
		// Si la insercion choca con el indice unico, ya existe una insignia
		// viva: eso NO es un fallo, es exactamente lo que queriamos.
		if isUniqueViolation(err) {
			d.skipDuplicate(queue.TaskIssueBadge, jobKey)
			d.markProcessed(ctx, jobKey, queue.TaskIssueBadge, map[string]any{"result": "already_issued"})
			return nil
		}
		return err
	}

	// Imagen de la insignia, guardada en el almacenamiento de objetos.
	imageKey := fmt.Sprintf("badges/%s.svg", publicCode)
	svg := badge.RenderSVG(courseTitle, fullName, publicCode, issuedAt)
	if err := d.Store.PutBytes(ctx, imageKey, svg, "image/svg+xml"); err != nil {
		return err
	}
	if _, err := d.DB.ExecContext(ctx,
		`UPDATE badges SET image_key = $1 WHERE id = $2`, imageKey, badgeID); err != nil {
		return err
	}

	verifyURL := strings.TrimRight(d.Cfg.PublicBaseURL, "/") + "/api/v1/public/badges/" + publicCode
	_ = d.Queue.EnqueueEmail(email, "Aprobaste "+courseTitle,
		"Felicitaciones. Tu insignia esta disponible.\n\nVerificacion publica: "+verifyURL)

	metrics.BadgesIssued.Inc()
	d.Audit.Log(ctx, "", audit.ActionBadgeIssued, "badge", badgeID, "",
		map[string]any{"course_id": courseID, "enrollment_id": p.EnrollmentID})
	d.markProcessed(ctx, jobKey, queue.TaskIssueBadge,
		map[string]any{"result": "issued", "badge_id": badgeID})
	return nil
}

// newPublicCode genera el identificador que va en la URL publica.
// Es aleatorio a proposito: si fuera el id del usuario o un consecutivo,
// cualquiera podria enumerar las insignias de la plataforma.
func newPublicCode() (string, error) {
	b := make([]byte, 15)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b)), nil
}

// isUniqueViolation reconoce el codigo 23505 de PostgreSQL sin acoplarnos al
// tipo concreto del driver.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}
