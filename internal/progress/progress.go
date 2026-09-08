// Package progress concentra las reglas de avance, finalizacion y aprobacion.
//
// Esta en un paquete propio (y no dentro de los handlers) porque lo necesitan
// dos procesos distintos: la API, cuando llega un evento o se califica un
// quiz, y el worker, cuando reevalua una inscripcion. Tener una sola
// implementacion evita que ambos calculen porcentajes distintos.
//
// PRINCIPIO: el porcentaje SIEMPRE lo calcula el servidor a partir de la
// evidencia almacenada. El cliente jamas envia "voy en 80%": envia senales
// (abri esto, sigo aqui, termine esto) y el servidor decide que significan.
package progress

import (
	"context"
	"database/sql"
	"fmt"
)

// Result es el estado de una inscripcion despues de recalcular.
type Result struct {
	ProgressPct     float64 `json:"progress_pct"`
	State           string  `json:"state"` // in_progress | completed | approved
	RequiredTotal   int     `json:"required_total"`
	RequiredDone    int     `json:"required_done"`
	QuizzesRequired int     `json:"quizzes_required"`
	QuizzesPassed   int     `json:"quizzes_passed"`
	// NewlyApproved indica que esta llamada fue la que provoco la aprobacion.
	// Solo entonces se encola la emision de la insignia.
	NewlyApproved bool `json:"newly_approved"`
}

// MinDwellSeconds calcula cuanto tiempo debe permanecer un estudiante en un
// recurso para que se pueda dar por consumido.
//
// Para video y audio se exige el 80% de la duracion real del archivo (dato
// que produce el worker al transcodificar, no el cliente). Para material de
// lectura se usa un minimo fijo. El objetivo no es vigilar al estudiante sino
// que "completado" signifique algo: sin umbral, bastaria abrir y cerrar.
func MinDwellSeconds(resourceType string, durationSecs int) int {
	switch resourceType {
	case "video", "audio":
		if durationSecs > 0 {
			return int(float64(durationSecs) * 0.8)
		}
		return 60
	case "pdf", "rich_text":
		return 15
	default:
		return 5
	}
}

// Recompute recalcula el estado completo de una inscripcion y lo persiste.
func Recompute(ctx context.Context, db *sql.DB, enrollmentID string) (Result, error) {
	var res Result

	var (
		courseID       string
		userID         string
		currentVersion sql.NullString
		currentState   string
		requiredPct    float64
		passingScore   float64
	)
	err := db.QueryRowContext(ctx, `
		SELECT e.course_id, e.user_id, c.current_version_id, e.state::text,
		       COALESCE(v.required_completion, 100.00), COALESCE(v.passing_score, 70.00)
		FROM enrollments e
		JOIN courses c ON c.id = e.course_id
		LEFT JOIN course_versions v ON v.id = c.current_version_id
		WHERE e.id = $1`, enrollmentID).
		Scan(&courseID, &userID, &currentVersion, &currentState, &requiredPct, &passingScore)
	if err != nil {
		return res, fmt.Errorf("cargar inscripcion: %w", err)
	}
	if !currentVersion.Valid {
		// Curso sin version publicada: no hay nada que medir.
		return res, nil
	}

	// Recursos obligatorios y visibles de la version vigente, y cuantos de
	// ellos tiene completados el estudiante. La union es por stable_id, que
	// es lo que hace que el progreso sobreviva a una version nueva.
	err = db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*)
			   FROM resources r
			   JOIN units u ON u.id = r.unit_id
			   JOIN modules m ON m.id = u.module_id
			  WHERE m.version_id = $1 AND r.visible AND r.required),
			(SELECT count(*)
			   FROM resources r
			   JOIN units u ON u.id = r.unit_id
			   JOIN modules m ON m.id = u.module_id
			   JOIN resource_progress rp
			        ON rp.resource_stable_id = r.stable_id AND rp.enrollment_id = $2
			  WHERE m.version_id = $1 AND r.visible AND r.required AND rp.completed)
		`, currentVersion.String, enrollmentID).Scan(&res.RequiredTotal, &res.RequiredDone)
	if err != nil {
		return res, fmt.Errorf("contar recursos obligatorios: %w", err)
	}

	// Quizzes obligatorios y cuantos estan aprobados.
	err = db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*)
			   FROM resources r
			   JOIN units u ON u.id = r.unit_id
			   JOIN modules m ON m.id = u.module_id
			   JOIN quizzes qz ON qz.resource_id = r.id
			  WHERE m.version_id = $1 AND r.visible AND r.required),
			(SELECT count(DISTINCT qz.id)
			   FROM resources r
			   JOIN units u ON u.id = r.unit_id
			   JOIN modules m ON m.id = u.module_id
			   JOIN quizzes qz ON qz.resource_id = r.id
			   JOIN quiz_attempts qa ON qa.quiz_id = qz.id AND qa.user_id = $2
			  WHERE m.version_id = $1 AND r.visible AND r.required
			    AND qa.status = 'submitted' AND qa.passed = TRUE)
		`, currentVersion.String, userID).Scan(&res.QuizzesRequired, &res.QuizzesPassed)
	if err != nil {
		return res, fmt.Errorf("contar quizzes: %w", err)
	}

	if res.RequiredTotal > 0 {
		res.ProgressPct = float64(res.RequiredDone) * 100.0 / float64(res.RequiredTotal)
	}

	// completed: consumio el material exigido.
	// approved: ademas aprobo todos los quizzes obligatorios.
	res.State = "in_progress"
	if res.RequiredTotal > 0 && res.ProgressPct >= requiredPct {
		res.State = "completed"
		if res.QuizzesPassed >= res.QuizzesRequired {
			res.State = "approved"
		}
	}

	res.NewlyApproved = res.State == "approved" && currentState != "approved"

	_, err = db.ExecContext(ctx, `
		UPDATE enrollments SET
			progress_pct = $1,
			state = $2::learning_state,
			completed_at = CASE WHEN $2::text IN ('completed','approved') AND completed_at IS NULL
			                    THEN now() ELSE completed_at END,
			approved_at  = CASE WHEN $2::text = 'approved' AND approved_at IS NULL
			                    THEN now() ELSE approved_at END
		WHERE id = $3`, res.ProgressPct, res.State, enrollmentID)
	if err != nil {
		return res, fmt.Errorf("guardar progreso: %w", err)
	}

	return res, nil
}
