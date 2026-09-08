package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// =====================================================================
// Autoria de quizzes
//
// ALCANCE DE ESTA ENTREGA: preguntas de seleccion multiple SOLO DE TEXTO.
// No hay imagenes, video ni archivos adjuntos en enunciados ni en opciones.
// El modelo de datos lo refleja: quiz_questions.prompt y quiz_options.text
// son columnas de texto y no existe referencia a assets.
// =====================================================================

type updateQuizRequest struct {
	MaxAttempts      *int     `json:"max_attempts"`
	TimeLimitSecs    *int     `json:"time_limit_secs"`
	PassingScore     *float64 `json:"passing_score"`
	ShuffleQuestions *bool    `json:"shuffle_questions"`
	Feedback         *string  `json:"feedback"` // none | on_submit | after_passing
}

func (s *Server) handleUpdateQuiz(c *gin.Context) {
	if _, ok := s.guard(c, "quiz", c.Param("id")); !ok {
		return
	}
	var req updateQuizRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Cuerpo invalido.", nil)
		return
	}
	if req.Feedback != nil {
		switch *req.Feedback {
		case "none", "on_submit", "after_passing":
		default:
			badRequest(c, "feedback debe ser none, on_submit o after_passing.", nil)
			return
		}
	}

	if _, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE quizzes SET
			max_attempts      = COALESCE($1, max_attempts),
			time_limit_secs   = COALESCE($2, time_limit_secs),
			passing_score     = COALESCE($3, passing_score),
			shuffle_questions = COALESCE($4, shuffle_questions),
			feedback          = COALESCE($5::feedback_policy, feedback),
			updated_at        = now()
		WHERE id = $6`,
		req.MaxAttempts, req.TimeLimitSecs, req.PassingScore,
		req.ShuffleQuestions, req.Feedback, c.Param("id")); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Quiz actualizado."})
}

// handleGetQuizAuthor devuelve el quiz COMPLETO, con la marca de respuesta
// correcta. Solo lo puede pedir el autor: es la unica vista de la aplicacion
// donde is_correct sale de la base de datos.
func (s *Server) handleGetQuizAuthor(c *gin.Context) {
	if _, ok := s.guard(c, "quiz", c.Param("id")); !ok {
		return
	}
	ctx := c.Request.Context()
	quizID := c.Param("id")

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

	rows, err := s.db.QueryContext(ctx, `
		SELECT q.id, q.stable_id, q.prompt, q.position, q.points, q.multiple,
		       o.id, o.stable_id, o.text, o.is_correct, o.feedback, o.position
		FROM quiz_questions q
		LEFT JOIN quiz_options o ON o.question_id = q.id
		WHERE q.quiz_id = $1
		ORDER BY q.position, o.position`, quizID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	type option struct {
		ID        string `json:"id"`
		StableID  string `json:"stable_id"`
		Text      string `json:"text"`
		IsCorrect bool   `json:"is_correct"`
		Feedback  string `json:"feedback"`
		Position  int    `json:"position"`
	}
	type question struct {
		ID       string   `json:"id"`
		StableID string   `json:"stable_id"`
		Prompt   string   `json:"prompt"`
		Position int      `json:"position"`
		Points   float64  `json:"points"`
		Multiple bool     `json:"multiple"`
		Options  []option `json:"options"`
	}

	var questions []question
	index := map[string]int{}
	for rows.Next() {
		var (
			qid, sid, prompt          string
			qpos                      int
			points                    float64
			multiple                  bool
			oid, osid, otext, ofb     *string
			ocorrect                  *bool
			opos                      *int
		)
		if err := rows.Scan(&qid, &sid, &prompt, &qpos, &points, &multiple,
			&oid, &osid, &otext, &ocorrect, &ofb, &opos); err != nil {
			internalError(c, err)
			return
		}
		if _, ok := index[qid]; !ok {
			index[qid] = len(questions)
			questions = append(questions, question{
				ID: qid, StableID: sid, Prompt: prompt, Position: qpos,
				Points: points, Multiple: multiple, Options: []option{},
			})
		}
		if oid != nil {
			i := index[qid]
			questions[i].Options = append(questions[i].Options, option{
				ID: *oid, StableID: *osid, Text: *otext,
				IsCorrect: *ocorrect, Feedback: *ofb, Position: *opos,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id": quizID, "max_attempts": maxAttempts, "time_limit_secs": timeLimit,
		"passing_score": passing, "shuffle_questions": shuffle, "feedback": feedback,
		"questions": questions,
	})
}

type optionRequest struct {
	Text      string `json:"text" binding:"required"`
	IsCorrect bool   `json:"is_correct"`
	Feedback  string `json:"feedback"`
}

type questionRequest struct {
	Prompt   string          `json:"prompt" binding:"required"`
	Points   *float64        `json:"points"`
	Multiple *bool           `json:"multiple"`
	Options  []optionRequest `json:"options" binding:"required"`
}

// validate aplica las reglas pedagogicas minimas. Rechazar aqui evita que un
// quiz imposible de aprobar llegue a un estudiante.
func (q questionRequest) validate() error {
	if len(q.Options) < 2 {
		return errTooFewOptions
	}
	correct := 0
	for _, o := range q.Options {
		if strings.TrimSpace(o.Text) == "" {
			return errEmptyOption
		}
		if o.IsCorrect {
			correct++
		}
	}
	if correct == 0 {
		return errNoCorrect
	}
	if correct > 1 && (q.Multiple == nil || !*q.Multiple) {
		return errTooManyCorrect
	}
	return nil
}

var (
	errTooFewOptions  = errString("la pregunta necesita al menos dos opciones")
	errEmptyOption    = errString("hay una opcion con texto vacio")
	errNoCorrect      = errString("la pregunta necesita al menos una opcion correcta")
	errTooManyCorrect = errString("hay varias opciones correctas: marca multiple = true")
)

type errString string

func (e errString) Error() string { return string(e) }

func (s *Server) handleCreateQuestion(c *gin.Context) {
	if _, ok := s.guard(c, "quiz", c.Param("id")); !ok {
		return
	}
	var req questionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren prompt y options.", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		badRequest(c, err.Error(), nil)
		return
	}

	ctx := c.Request.Context()
	quizID := c.Param("id")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var pos int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), 0) + 1 FROM quiz_questions WHERE quiz_id = $1`, quizID).
		Scan(&pos); err != nil {
		internalError(c, err)
		return
	}

	multiple := req.Multiple != nil && *req.Multiple
	points := 1.0
	if req.Points != nil {
		points = *req.Points
	}

	var questionID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO quiz_questions (quiz_id, stable_id, prompt, position, points, multiple)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		quizID, uuid.NewString(), strings.TrimSpace(req.Prompt), pos, points, multiple).
		Scan(&questionID); err != nil {
		internalError(c, err)
		return
	}

	for i, o := range req.Options {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO quiz_options (question_id, stable_id, text, is_correct, feedback, position)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			questionID, uuid.NewString(), strings.TrimSpace(o.Text), o.IsCorrect,
			o.Feedback, i+1); err != nil {
			internalError(c, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": questionID, "position": pos, "options": len(req.Options)})
}

// handleUpdateQuestion reemplaza la pregunta completa (enunciado y opciones).
// Reemplazar es mas simple y menos propenso a errores que un parche parcial
// sobre una lista ordenada de opciones.
func (s *Server) handleUpdateQuestion(c *gin.Context) {
	if _, ok := s.guard(c, "question", c.Param("id")); !ok {
		return
	}
	var req questionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren prompt y options.", err.Error())
		return
	}
	if err := req.validate(); err != nil {
		badRequest(c, err.Error(), nil)
		return
	}

	ctx := c.Request.Context()
	questionID := c.Param("id")

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	multiple := req.Multiple != nil && *req.Multiple
	points := 1.0
	if req.Points != nil {
		points = *req.Points
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE quiz_questions SET prompt = $1, points = $2, multiple = $3 WHERE id = $4`,
		strings.TrimSpace(req.Prompt), points, multiple, questionID); err != nil {
		internalError(c, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM quiz_options WHERE question_id = $1`, questionID); err != nil {
		internalError(c, err)
		return
	}
	for i, o := range req.Options {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO quiz_options (question_id, stable_id, text, is_correct, feedback, position)
			VALUES ($1, $2, $3, $4, $5, $6)`,
			questionID, uuid.NewString(), strings.TrimSpace(o.Text), o.IsCorrect, o.Feedback, i+1); err != nil {
			internalError(c, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Pregunta actualizada."})
}

func (s *Server) handleDeleteQuestion(c *gin.Context) {
	if _, ok := s.guard(c, "question", c.Param("id")); !ok {
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`DELETE FROM quiz_questions WHERE id = $1`, c.Param("id")); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Pregunta eliminada."})
}
