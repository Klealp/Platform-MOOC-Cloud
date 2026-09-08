package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/metrics"
	"mooc-platform/internal/progress"
)

// =====================================================================
// Presentacion de quizzes
//
// CONDICION VERIFICABLE DEL ENUNCIADO: "La clave correcta nunca llega al
// cliente, el envio definitivo es idempotente y la calificacion se calcula en
// el servidor."
//
// Como se cumple:
//   - Al iniciar el intento se guarda un SNAPSHOT en la base. El snapshot
//     tiene dos partes: `questions` (lo que ve el estudiante, sin marcas) y
//     `key` (las respuestas correctas). La API solo serializa `questions`
//     hacia afuera; `key` no sale nunca de la base de datos.
//   - Guardar el snapshot ademas congela el examen: si el profesor edita el
//     quiz a mitad del intento, la calificacion usa lo que el estudiante vio.
//   - El envio pasa por el middleware de Idempotency-Key y, ademas, comprueba
//     el estado del intento: reenviar no recalifica.
// =====================================================================

type snapshotOption struct {
	StableID string `json:"stable_id"`
	Text     string `json:"text"`
	Feedback string `json:"feedback,omitempty"`
}

type snapshotQuestion struct {
	StableID string           `json:"stable_id"`
	Prompt   string           `json:"prompt"`
	Points   float64          `json:"points"`
	Multiple bool             `json:"multiple"`
	Options  []snapshotOption `json:"options"`
}

// attemptSnapshot es lo que se guarda en quiz_attempts.snapshot.
type attemptSnapshot struct {
	QuizID       string              `json:"quiz_id"`
	PassingScore float64             `json:"passing_score"`
	Feedback     string              `json:"feedback"`
	Questions    []snapshotQuestion  `json:"questions"`
	Key          map[string][]string `json:"key"` // NUNCA se envia al cliente
}

// publicQuestions produce la vista segura del snapshot.
func (a attemptSnapshot) publicQuestions(includeFeedback bool) []snapshotQuestion {
	out := make([]snapshotQuestion, 0, len(a.Questions))
	for _, q := range a.Questions {
		pq := snapshotQuestion{
			StableID: q.StableID, Prompt: q.Prompt, Points: q.Points, Multiple: q.Multiple,
			Options: make([]snapshotOption, 0, len(q.Options)),
		}
		for _, o := range q.Options {
			po := snapshotOption{StableID: o.StableID, Text: o.Text}
			if includeFeedback {
				po.Feedback = o.Feedback
			}
			pq.Options = append(pq.Options, po)
		}
		out = append(out, pq)
	}
	return out
}

// enrollmentForQuiz comprueba que el estudiante tenga derecho a presentar el
// quiz: curso publicado, inscripcion activa y recurso visible en la version
// vigente. Devuelve el id de la inscripcion.
func (s *Server) enrollmentForQuiz(ctx context.Context, quizID, userID string) (string, error) {
	var enrollmentID string
	err := s.db.QueryRowContext(ctx, `
		SELECT e.id
		FROM quizzes qz
		JOIN resources r ON r.id = qz.resource_id
		JOIN units u ON u.id = r.unit_id
		JOIN modules m ON m.id = u.module_id
		JOIN course_versions v ON v.id = m.version_id
		JOIN courses c ON c.id = v.course_id AND c.current_version_id = v.id
		JOIN enrollments e ON e.course_id = c.id AND e.user_id = $2
		WHERE qz.id = $1 AND r.visible AND c.status = 'published' AND e.status = 'active'`,
		quizID, userID).Scan(&enrollmentID)
	return enrollmentID, err
}

// handleStartAttempt crea el intento y devuelve el cuestionario sin claves.
func (s *Server) handleStartAttempt(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()
	quizID := c.Param("id")

	if _, err := s.enrollmentForQuiz(ctx, quizID, id.UserID); errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Quiz no encontrado.")
		return
	} else if err != nil {
		internalError(c, err)
		return
	}

	var (
		maxAttempts, timeLimit int
		passing                float64
		shuffle                bool
		feedback               string
	)
	if err := s.db.QueryRowContext(ctx, `
		SELECT max_attempts, time_limit_secs, passing_score, shuffle_questions, feedback::text
		FROM quizzes WHERE id = $1`, quizID).
		Scan(&maxAttempts, &timeLimit, &passing, &shuffle, &feedback); err != nil {
		internalError(c, err)
		return
	}

	// Si hay un intento abierto se devuelve ese mismo, en lugar de crear otro.
	// Recargar la pagina no debe consumir un intento.
	var openID string
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM quiz_attempts
		WHERE quiz_id = $1 AND user_id = $2 AND status = 'in_progress'
		ORDER BY attempt_number DESC LIMIT 1`, quizID, id.UserID).Scan(&openID)
	if err == nil {
		s.respondAttempt(c, openID, false)
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		internalError(c, err)
		return
	}

	var used int
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM quiz_attempts WHERE quiz_id = $1 AND user_id = $2`,
		quizID, id.UserID).Scan(&used); err != nil {
		internalError(c, err)
		return
	}
	if maxAttempts > 0 && used >= maxAttempts {
		conflict(c, "Agotaste los intentos disponibles para este quiz.",
			gin.H{"max_attempts": maxAttempts, "used": used})
		return
	}

	snap, err := s.buildSnapshot(ctx, quizID, passing, feedback, shuffle)
	if err != nil {
		internalError(c, err)
		return
	}
	if len(snap.Questions) == 0 {
		unprocessable(c, "El quiz no tiene preguntas.", nil)
		return
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		internalError(c, err)
		return
	}

	var expires any
	if timeLimit > 0 {
		expires = time.Now().Add(time.Duration(timeLimit) * time.Second)
	}

	var attemptID string
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO quiz_attempts (quiz_id, user_id, attempt_number, status, snapshot, expires_at)
		VALUES ($1, $2, $3, 'in_progress', $4, $5) RETURNING id`,
		quizID, id.UserID, used+1, string(raw), expires).Scan(&attemptID); err != nil {
		internalError(c, err)
		return
	}

	s.respondAttempt(c, attemptID, false)
}

func (s *Server) buildSnapshot(ctx context.Context, quizID string, passing float64, feedback string, shuffle bool) (attemptSnapshot, error) {
	snap := attemptSnapshot{
		QuizID: quizID, PassingScore: passing, Feedback: feedback,
		Key: map[string][]string{},
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT q.stable_id, q.prompt, q.points, q.multiple,
		       o.stable_id, o.text, o.is_correct, o.feedback
		FROM quiz_questions q
		JOIN quiz_options o ON o.question_id = q.id
		WHERE q.quiz_id = $1
		ORDER BY q.position, o.position`, quizID)
	if err != nil {
		return snap, err
	}
	defer rows.Close()

	index := map[string]int{}
	for rows.Next() {
		var (
			qsid, prompt, osid, otext, ofb string
			points                         float64
			multiple, correct              bool
		)
		if err := rows.Scan(&qsid, &prompt, &points, &multiple, &osid, &otext, &correct, &ofb); err != nil {
			return snap, err
		}
		i, ok := index[qsid]
		if !ok {
			i = len(snap.Questions)
			index[qsid] = i
			snap.Questions = append(snap.Questions, snapshotQuestion{
				StableID: qsid, Prompt: prompt, Points: points, Multiple: multiple,
			})
		}
		snap.Questions[i].Options = append(snap.Questions[i].Options,
			snapshotOption{StableID: osid, Text: otext, Feedback: ofb})
		if correct {
			snap.Key[qsid] = append(snap.Key[qsid], osid)
		}
	}
	if err := rows.Err(); err != nil {
		return snap, err
	}

	// El orden barajado se guarda EN el snapshot, no se recalcula en cada
	// lectura: el estudiante debe ver siempre el mismo examen.
	if shuffle {
		rand.Shuffle(len(snap.Questions), func(i, j int) {
			snap.Questions[i], snap.Questions[j] = snap.Questions[j], snap.Questions[i]
		})
	}
	return snap, nil
}

// attemptRow es el intento tal como esta en la base.
type attemptRow struct {
	ID       string
	QuizID   string
	UserID   string
	Number   int
	Status   string
	Snapshot attemptSnapshot
	Answers  map[string][]string
	Score    sql.NullFloat64
	Passed   sql.NullBool
	Expires  sql.NullTime
}

func (s *Server) loadAttempt(ctx context.Context, attemptID string) (attemptRow, error) {
	var a attemptRow
	var snapRaw, ansRaw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, quiz_id, user_id, attempt_number, status::text, snapshot, answers,
		       score, passed, expires_at
		FROM quiz_attempts WHERE id = $1`, attemptID).
		Scan(&a.ID, &a.QuizID, &a.UserID, &a.Number, &a.Status, &snapRaw, &ansRaw,
			&a.Score, &a.Passed, &a.Expires)
	if err != nil {
		return a, err
	}
	if err := json.Unmarshal(snapRaw, &a.Snapshot); err != nil {
		return a, err
	}
	a.Answers = map[string][]string{}
	_ = json.Unmarshal(ansRaw, &a.Answers)
	return a, nil
}

// respondAttempt escribe la vista del intento SIN claves correctas.
func (s *Server) respondAttempt(c *gin.Context, attemptID string, includeResult bool) {
	a, err := s.loadAttempt(c.Request.Context(), attemptID)
	if err != nil {
		internalError(c, err)
		return
	}

	// La retroalimentacion solo se adjunta cuando la politica lo permite y el
	// intento ya termino.
	showFeedback := includeResult &&
		(a.Snapshot.Feedback == "on_submit" ||
			(a.Snapshot.Feedback == "after_passing" && a.Passed.Valid && a.Passed.Bool))

	body := gin.H{
		"id": a.ID, "quiz_id": a.QuizID, "attempt_number": a.Number,
		"status": a.Status, "questions": a.Snapshot.publicQuestions(showFeedback),
		"answers": a.Answers, "passing_score": a.Snapshot.PassingScore,
	}
	if a.Expires.Valid {
		body["expires_at"] = a.Expires.Time
		body["seconds_left"] = int(time.Until(a.Expires.Time).Seconds())
	}
	if a.Score.Valid {
		body["score"] = a.Score.Float64
		body["passed"] = a.Passed.Bool
	}

	c.JSON(http.StatusOK, body)
}

func (s *Server) handleGetAttempt(c *gin.Context) {
	id := currentUser(c)
	a, err := s.loadAttempt(c.Request.Context(), c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Intento no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if a.UserID != id.UserID {
		// Aislamiento: el intento de otro estudiante no existe para ti.
		notFound(c, "Intento no encontrado.")
		return
	}
	s.respondAttempt(c, a.ID, a.Status != "in_progress")
}

type saveAttemptRequest struct {
	// answers: { "<question_stable_id>": ["<option_stable_id>", ...] }
	Answers map[string][]string `json:"answers" binding:"required"`
}

// handleSaveAttempt guarda respuestas parciales.
// Se valida que las llaves existan en el snapshot: no se acepta responder a
// una pregunta que no forma parte de este examen.
func (s *Server) handleSaveAttempt(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	a, err := s.loadAttempt(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Intento no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if a.UserID != id.UserID {
		notFound(c, "Intento no encontrado.")
		return
	}
	if a.Status != "in_progress" {
		conflict(c, "El intento ya fue enviado o expiro.", gin.H{"status": a.Status})
		return
	}
	if a.Expires.Valid && time.Now().After(a.Expires.Time) {
		conflict(c, "El tiempo del intento expiro.", nil)
		return
	}

	var req saveAttemptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requiere el objeto answers.", nil)
		return
	}

	valid, problems := validateAnswers(a.Snapshot, req.Answers)
	if len(problems) > 0 {
		unprocessable(c, "Hay respuestas que no corresponden a este examen.", gin.H{"problems": problems})
		return
	}

	// Fusion: las preguntas no incluidas conservan su respuesta anterior.
	for k, v := range valid {
		a.Answers[k] = v
	}
	raw, _ := json.Marshal(a.Answers)

	if _, err := s.db.ExecContext(ctx,
		`UPDATE quiz_attempts SET answers = $1 WHERE id = $2 AND status = 'in_progress'`,
		string(raw), a.ID); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"saved": true, "answered": len(a.Answers), "total": len(a.Snapshot.Questions)})
}

func validateAnswers(snap attemptSnapshot, answers map[string][]string) (map[string][]string, []string) {
	valid := map[string][]string{}
	var problems []string

	options := map[string]map[string]bool{}
	multiple := map[string]bool{}
	for _, q := range snap.Questions {
		set := map[string]bool{}
		for _, o := range q.Options {
			set[o.StableID] = true
		}
		options[q.StableID] = set
		multiple[q.StableID] = q.Multiple
	}

	for qid, selected := range answers {
		set, ok := options[qid]
		if !ok {
			problems = append(problems, "pregunta desconocida: "+qid)
			continue
		}
		if !multiple[qid] && len(selected) > 1 {
			problems = append(problems, "la pregunta "+qid+" admite una sola respuesta")
			continue
		}
		clean := make([]string, 0, len(selected))
		seen := map[string]bool{}
		bad := false
		for _, oid := range selected {
			if !set[oid] {
				problems = append(problems, "opcion desconocida en la pregunta "+qid)
				bad = true
				break
			}
			if !seen[oid] {
				seen[oid] = true
				clean = append(clean, oid)
			}
		}
		if !bad {
			valid[qid] = clean
		}
	}
	return valid, problems
}

// handleSubmitAttempt califica en el servidor y cierra el intento.
func (s *Server) handleSubmitAttempt(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	a, err := s.loadAttempt(ctx, c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Intento no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if a.UserID != id.UserID {
		notFound(c, "Intento no encontrado.")
		return
	}

	// IDEMPOTENCIA a nivel de dominio: aunque el middleware no tuviera la
	// llave, reenviar no recalifica. El estado del intento es la garantia.
	if a.Status != "in_progress" {
		c.JSON(http.StatusOK, gin.H{
			"id": a.ID, "status": a.Status, "score": a.Score.Float64,
			"passed": a.Passed.Bool, "already_submitted": true,
		})
		return
	}

	// Permite enviar respuestas finales en el mismo cuerpo del submit.
	var req saveAttemptRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.Answers != nil {
		valid, problems := validateAnswers(a.Snapshot, req.Answers)
		if len(problems) > 0 {
			unprocessable(c, "Hay respuestas que no corresponden a este examen.", gin.H{"problems": problems})
			return
		}
		for k, v := range valid {
			a.Answers[k] = v
		}
	}

	expired := a.Expires.Valid && time.Now().After(a.Expires.Time)
	score, correctCount := gradeAttempt(a.Snapshot, a.Answers)
	passed := score >= a.Snapshot.PassingScore

	finalStatus := "submitted"
	if expired {
		// Un intento vencido se califica con lo que alcanzo a guardar y se
		// marca como expirado: no se descarta el trabajo del estudiante.
		finalStatus = "expired"
	}

	answersRaw, _ := json.Marshal(a.Answers)
	res, err := s.db.ExecContext(ctx, `
		UPDATE quiz_attempts
		SET status = $1::attempt_status, answers = $2, score = $3, passed = $4, submitted_at = now()
		WHERE id = $5 AND status = 'in_progress'`,
		finalStatus, string(answersRaw), score, passed, a.ID)
	if err != nil {
		internalError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Otra peticion gano la carrera: devolvemos su resultado.
		s.respondAttempt(c, a.ID, true)
		return
	}

	result := "failed"
	if passed {
		result = "passed"
	}
	metrics.QuizSubmissions.WithLabelValues(result).Inc()

	// El quiz aprobado marca su recurso como completado y dispara el
	// recalculo del progreso del curso.
	var progressResult progress.Result
	enrollmentID, err := s.enrollmentForQuiz(ctx, a.QuizID, id.UserID)
	if err == nil {
		if passed && finalStatus == "submitted" {
			if err := s.markQuizResourceComplete(ctx, enrollmentID, a.QuizID); err != nil {
				internalError(c, err)
				return
			}
		}
		progressResult, err = progress.Recompute(ctx, s.db, enrollmentID)
		if err != nil {
			internalError(c, err)
			return
		}
		if progressResult.NewlyApproved {
			// La emision de la insignia es asincrona e idempotente.
			if err := s.queue.EnqueueBadge(enrollmentID, s.cfg.MaxRetries); err != nil {
				internalError(c, err)
				return
			}
		}
	}

	s.audit.Log(ctx, id.UserID, audit.ActionQuizSubmitted, "quiz_attempt", a.ID, c.ClientIP(),
		map[string]any{"quiz_id": a.QuizID, "score": score, "passed": passed, "status": finalStatus})

	body := gin.H{
		"id": a.ID, "status": finalStatus, "score": score, "passed": passed,
		"correct_questions": correctCount, "total_questions": len(a.Snapshot.Questions),
		"progress": progressResult,
	}
	if a.Snapshot.Feedback == "on_submit" || (a.Snapshot.Feedback == "after_passing" && passed) {
		body["questions"] = a.Snapshot.publicQuestions(true)
	}
	c.JSON(http.StatusOK, body)
}

// gradeAttempt es la calificacion. Vive en el servidor y usa la clave del
// snapshot, que el cliente nunca vio.
//
// Regla: una pregunta se acredita solo si el conjunto marcado coincide
// EXACTAMENTE con el conjunto correcto. No hay puntaje parcial.
func gradeAttempt(snap attemptSnapshot, answers map[string][]string) (float64, int) {
	var total, earned float64
	correctCount := 0

	for _, q := range snap.Questions {
		total += q.Points

		want := map[string]bool{}
		for _, oid := range snap.Key[q.StableID] {
			want[oid] = true
		}
		got := map[string]bool{}
		for _, oid := range answers[q.StableID] {
			got[oid] = true
		}
		if len(want) != len(got) || len(want) == 0 {
			continue
		}
		ok := true
		for oid := range want {
			if !got[oid] {
				ok = false
				break
			}
		}
		if ok {
			earned += q.Points
			correctCount++
		}
	}

	if total == 0 {
		return 0, 0
	}
	return earned * 100.0 / total, correctCount
}

// markQuizResourceComplete registra el consumo del recurso quiz.
func (s *Server) markQuizResourceComplete(ctx context.Context, enrollmentID, quizID string) error {
	var stableID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT r.stable_id FROM quizzes qz JOIN resources r ON r.id = qz.resource_id
		WHERE qz.id = $1`, quizID).Scan(&stableID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO resource_progress (enrollment_id, resource_stable_id, completed, completed_at, updated_at)
		VALUES ($1, $2, TRUE, now(), now())
		ON CONFLICT (enrollment_id, resource_stable_id)
		DO UPDATE SET completed = TRUE,
		              completed_at = COALESCE(resource_progress.completed_at, now()),
		              updated_at = now()`, enrollmentID, stableID)
	return err
}
