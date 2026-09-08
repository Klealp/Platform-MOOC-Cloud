package api

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/metrics"
	"mooc-platform/internal/progress"
)

// =====================================================================
// Catalogo publico
// =====================================================================

// handleCatalogList lista cursos publicados con busqueda, filtros y paginacion
// por cursor.
//
// Por que cursor y no OFFSET: con OFFSET, si alguien publica un curso mientras
// el usuario pagina, los elementos se desplazan y se repiten o se pierden
// filas. El cursor apunta a "el ultimo que viste" y es estable.
func (s *Server) handleCatalogList(c *gin.Context) {
	limit := clampInt(c.Query("limit"), 20, 1, 100)
	search := strings.TrimSpace(c.Query("q"))
	category := c.Query("category")
	level := c.Query("level")
	language := c.Query("language")
	tag := c.Query("tag")

	cursorTime := time.Now().Add(24 * time.Hour)
	cursorID := "ffffffff-ffff-ffff-ffff-ffffffffffff"
	if raw := c.Query("cursor"); raw != "" {
		t, id, err := decodeCursor(raw)
		if err != nil {
			badRequest(c, "Cursor invalido.", nil)
			return
		}
		cursorTime, cursorID = t, id
	}

	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT c.id, c.slug, c.title, c.summary, c.category, c.level, c.language, c.tags,
		       u.full_name, v.published_at,
		       (SELECT count(*) FROM enrollments e WHERE e.course_id = c.id AND e.status = 'active')
		FROM courses c
		JOIN course_versions v ON v.id = c.current_version_id
		JOIN users u ON u.id = c.owner_id
		WHERE c.status = 'published'
		  AND ($1 = '' OR c.title ILIKE '%'||$1||'%' OR c.summary ILIKE '%'||$1||'%')
		  AND ($2 = '' OR c.category = $2)
		  AND ($3 = '' OR c.level = $3)
		  AND ($4 = '' OR c.language = $4)
		  AND ($5 = '' OR $5 = ANY(c.tags))
		  AND (v.published_at, c.id) < ($6::timestamptz, $7::uuid)
		ORDER BY v.published_at DESC, c.id DESC
		LIMIT $8`,
		search, category, level, language, tag, cursorTime, cursorID, limit+1)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	type item struct {
		ID          string    `json:"id"`
		Slug        string    `json:"slug"`
		Title       string    `json:"title"`
		Summary     string    `json:"summary"`
		Category    string    `json:"category"`
		Level       string    `json:"level"`
		Language    string    `json:"language"`
		Tags        []string  `json:"tags"`
		Teacher     string    `json:"teacher"`
		PublishedAt time.Time `json:"published_at"`
		Enrolled    int       `json:"enrolled"`
	}

	items := []item{}
	for rows.Next() {
		var it item
		var tags pq.StringArray
		var published sql.NullTime
		if err := rows.Scan(&it.ID, &it.Slug, &it.Title, &it.Summary, &it.Category,
			&it.Level, &it.Language, &tags, &it.Teacher, &published, &it.Enrolled); err != nil {
			internalError(c, err)
			return
		}
		it.Tags = []string(tags)
		if published.Valid {
			it.PublishedAt = published.Time
		}
		items = append(items, it)
	}

	nextCursor := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		nextCursor = encodeCursor(last.PublishedAt, last.ID)
	}

	c.JSON(http.StatusOK, gin.H{"data": items, "next_cursor": nextCursor})
}

func encodeCursor(t time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id))
}

func decodeCursor(raw string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, "", err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", errors.New("cursor malformado")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", err
	}
	return t, parts[1], nil
}

// handleCatalogDetail muestra la ficha publica del curso: metadatos y el
// esquema de contenidos VISIBLES de la version vigente. No entrega contenido
// ni URLs de archivos: para eso hay que inscribirse.
func (s *Server) handleCatalogDetail(c *gin.Context) {
	ctx := c.Request.Context()

	var (
		courseID, title, summary, category, level, language, teacher string
		tags                                                         pq.StringArray
		versionID                                                    string
		published                                                    sql.NullTime
		passing, requiredPct                                         float64
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, c.title, c.summary, c.category, c.level, c.language, c.tags,
		       u.full_name, v.id, v.published_at, v.passing_score, v.required_completion
		FROM courses c
		JOIN course_versions v ON v.id = c.current_version_id
		JOIN users u ON u.id = c.owner_id
		WHERE c.slug = $1 AND c.status = 'published'`, c.Param("slug")).
		Scan(&courseID, &title, &summary, &category, &level, &language, &tags,
			&teacher, &versionID, &published, &passing, &requiredPct)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Curso no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	modules, err := s.buildOutline(ctx, versionID, true)
	if err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id": courseID, "slug": c.Param("slug"), "title": title, "summary": summary,
		"category": category, "level": level, "language": language, "tags": []string(tags),
		"teacher": teacher, "version_id": versionID, "published_at": nullTime(published),
		"passing_score": passing, "required_completion": requiredPct,
		"outline": modules,
	})
}

// =====================================================================
// Inscripcion
// =====================================================================

type enrollRequest struct {
	CourseID string `json:"course_id"`
	Slug     string `json:"slug"`
}

// handleEnroll inscribe al usuario.
//
// CONDICION VERIFICABLE: "reinscribirse conserva el progreso". Por eso hay un
// UNIQUE (course_id, user_id) y el retiro no borra la fila: la marca como
// 'withdrawn'. Reinscribirse reactiva ESA MISMA fila, con lo que
// resource_progress y quiz_attempts siguen apuntando a ella.
func (s *Server) handleEnroll(c *gin.Context) {
	id := currentUser(c)
	var req enrollRequest
	if err := c.ShouldBindJSON(&req); err != nil || (req.CourseID == "" && req.Slug == "") {
		badRequest(c, "Envia course_id o slug.", nil)
		return
	}
	ctx := c.Request.Context()

	var courseID string
	err := s.db.QueryRowContext(ctx, `
		SELECT id FROM courses
		WHERE status = 'published' AND current_version_id IS NOT NULL
		  AND (($1 <> '' AND id::text = $1) OR ($2 <> '' AND slug = $2))`,
		req.CourseID, req.Slug).Scan(&courseID)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Curso no encontrado o no publicado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	// Primero miramos si ya existia una inscripcion (aunque estuviera
	// retirada). Ese dato es el que distingue "inscripcion nueva" de
	// "reinscripcion", y es el que decide si se cuenta la metrica.
	var previousStatus string
	err = s.db.QueryRowContext(ctx,
		`SELECT status::text FROM enrollments WHERE course_id = $1 AND user_id = $2`,
		courseID, id.UserID).Scan(&previousStatus)
	reactivated := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		internalError(c, err)
		return
	}

	// El upsert reactiva la MISMA fila: por eso el progreso sobrevive.
	var enrollmentID string
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO enrollments (course_id, user_id, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (course_id, user_id) DO UPDATE
			SET status = 'active', withdrawn_at = NULL
		RETURNING id`, courseID, id.UserID).Scan(&enrollmentID); err != nil {
		internalError(c, err)
		return
	}

	// Recalculamos por si la version publicada cambio desde la ultima vez.
	result, err := progress.Recompute(ctx, s.db, enrollmentID)
	if err != nil {
		internalError(c, err)
		return
	}

	if !reactivated {
		metrics.Enrollments.Inc()
	}
	s.audit.Log(ctx, id.UserID, audit.ActionEnrolled, "enrollment", enrollmentID, c.ClientIP(),
		map[string]any{"course_id": courseID, "reactivated": reactivated})

	c.JSON(http.StatusCreated, gin.H{
		"id": enrollmentID, "course_id": courseID, "status": "active",
		"reactivated": reactivated, "progress": result,
	})
}

func (s *Server) handleListEnrollments(c *gin.Context) {
	id := currentUser(c)
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT e.id, e.course_id, c.slug, c.title, e.status::text, e.state::text,
		       e.progress_pct, e.enrolled_at, e.completed_at, e.approved_at
		FROM enrollments e JOIN courses c ON c.id = e.course_id
		WHERE e.user_id = $1
		ORDER BY e.enrolled_at DESC`, id.UserID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			eid, cid, slug, title, status, state string
			pct                                  float64
			enrolled                             time.Time
			completed, approved                  sql.NullTime
		)
		if err := rows.Scan(&eid, &cid, &slug, &title, &status, &state, &pct,
			&enrolled, &completed, &approved); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, gin.H{
			"id": eid, "course_id": cid, "slug": slug, "title": title,
			"status": status, "state": state, "progress_pct": pct,
			"enrolled_at": enrolled, "completed_at": nullTime(completed),
			"approved_at": nullTime(approved),
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// loadEnrollment aplica el aislamiento por usuario en el WHERE.
func (s *Server) loadEnrollment(c *gin.Context) (string, string, bool) {
	id := currentUser(c)
	var enrollmentID, courseID string
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT id, course_id FROM enrollments WHERE id = $1 AND user_id = $2`,
		c.Param("id"), id.UserID).Scan(&enrollmentID, &courseID)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Inscripcion no encontrada.")
		return "", "", false
	}
	if err != nil {
		internalError(c, err)
		return "", "", false
	}
	return enrollmentID, courseID, true
}

func (s *Server) handleGetEnrollment(c *gin.Context) {
	enrollmentID, _, ok := s.loadEnrollment(c)
	if !ok {
		return
	}
	result, err := progress.Recompute(c.Request.Context(), s.db, enrollmentID)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": enrollmentID, "progress": result})
}

// handleWithdraw retira al estudiante SIN borrar su avance.
func (s *Server) handleWithdraw(c *gin.Context) {
	enrollmentID, courseID, ok := s.loadEnrollment(c)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE enrollments SET status = 'withdrawn', withdrawn_at = now()
		WHERE id = $1`, enrollmentID); err != nil {
		internalError(c, err)
		return
	}
	s.audit.Log(c.Request.Context(), currentUser(c).UserID, audit.ActionWithdrawn,
		"enrollment", enrollmentID, c.ClientIP(), map[string]any{"course_id": courseID})
	c.JSON(http.StatusOK, gin.H{
		"message": "Te retiraste del curso. Tu progreso se conserva si vuelves a inscribirte.",
	})
}

// handleEnrollmentOutline devuelve el contenido visible del curso con el
// estado de avance de cada recurso.
func (s *Server) handleEnrollmentOutline(c *gin.Context) {
	enrollmentID, courseID, ok := s.loadEnrollment(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	var versionID string
	if err := s.db.QueryRowContext(ctx,
		`SELECT current_version_id FROM courses WHERE id = $1`, courseID).Scan(&versionID); err != nil {
		internalError(c, err)
		return
	}

	modules, err := s.buildOutline(ctx, versionID, true)
	if err != nil {
		internalError(c, err)
		return
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT resource_stable_id, completed, dwell_secs, last_position_secs
		FROM resource_progress WHERE enrollment_id = $1`, enrollmentID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	type rp struct {
		Completed bool `json:"completed"`
		DwellSecs int  `json:"dwell_secs"`
		Position  int  `json:"last_position_secs"`
	}
	byStable := map[string]rp{}
	for rows.Next() {
		var sid string
		var r rp
		if err := rows.Scan(&sid, &r.Completed, &r.DwellSecs, &r.Position); err != nil {
			internalError(c, err)
			return
		}
		byStable[sid] = r
	}

	result, err := progress.Recompute(ctx, s.db, enrollmentID)
	if err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"enrollment_id": enrollmentID, "course_id": courseID, "version_id": versionID,
		"modules": modules, "resource_progress": byStable, "progress": result,
	})
}

// =====================================================================
// Eventos de progreso
// =====================================================================

type progressEventRequest struct {
	EnrollmentID     string `json:"enrollment_id" binding:"required"`
	ResourceStableID string `json:"resource_stable_id" binding:"required"`
	EventType        string `json:"event_type" binding:"required"` // open | heartbeat | complete
	PositionSecs     int    `json:"position_secs"`
	DeltaSecs        int    `json:"delta_secs"`

	// Trampa deliberada: si el cliente intenta dictar el porcentaje, la
	// peticion se rechaza y queda registrada. El porcentaje es competencia
	// exclusiva del servidor.
	ProgressPct *float64 `json:"progress_pct"`
	Completed   *bool    `json:"completed"`
}

const maxHeartbeatDelta = 60 // segundos de credito por latido

func (s *Server) handleProgressEvent(c *gin.Context) {
	id := currentUser(c)
	ctx := c.Request.Context()

	var req progressEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren enrollment_id, resource_stable_id y event_type.", err.Error())
		return
	}

	if req.ProgressPct != nil || req.Completed != nil {
		s.audit.Log(ctx, id.UserID, audit.ActionProgressRejected, "enrollment", req.EnrollmentID,
			c.ClientIP(), map[string]any{"reason": "el cliente intento fijar el progreso"})
		unprocessable(c,
			"El progreso lo calcula el servidor. Envia solo senales (open, heartbeat, complete).",
			gin.H{"rejected_fields": []string{"progress_pct", "completed"}})
		return
	}
	switch req.EventType {
	case "open", "heartbeat", "complete":
	default:
		badRequest(c, "event_type debe ser open, heartbeat o complete.", nil)
		return
	}

	// La inscripcion debe ser del usuario y estar activa.
	var courseID string
	err := s.db.QueryRowContext(ctx, `
		SELECT course_id FROM enrollments
		WHERE id = $1 AND user_id = $2 AND status = 'active'`,
		req.EnrollmentID, id.UserID).Scan(&courseID)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Inscripcion no encontrada.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	// El recurso debe existir, ser visible y pertenecer a la version vigente.
	var resourceType string
	var durationSecs int
	err = s.db.QueryRowContext(ctx, `
		SELECT r.type::text, COALESCE(a.duration_secs, 0)
		FROM resources r
		JOIN units u ON u.id = r.unit_id
		JOIN modules m ON m.id = u.module_id
		JOIN courses c ON c.current_version_id = m.version_id
		LEFT JOIN assets a ON a.id = r.asset_id
		WHERE c.id = $1 AND r.stable_id = $2 AND r.visible`,
		courseID, req.ResourceStableID).Scan(&resourceType, &durationSecs)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Recurso no encontrado en la version vigente del curso.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if resourceType == "quiz" {
		unprocessable(c, "El avance en un quiz se registra al enviarlo, no con eventos.", nil)
		return
	}

	// Se acota el credito de tiempo por evento: sin este limite, un cliente
	// podria enviar delta_secs = 999999 y "completar" el video al instante.
	delta := req.DeltaSecs
	if delta < 0 {
		delta = 0
	}
	if delta > maxHeartbeatDelta {
		delta = maxHeartbeatDelta
	}
	position := req.PositionSecs
	if position < 0 {
		position = 0
	}

	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO progress_events (enrollment_id, resource_stable_id, event_type, position_secs, delta_secs)
		VALUES ($1, $2, $3, $4, $5)`,
		req.EnrollmentID, req.ResourceStableID, req.EventType, position, delta); err != nil {
		internalError(c, err)
		return
	}

	var dwell int
	var completed bool
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO resource_progress (enrollment_id, resource_stable_id, dwell_secs, last_position_secs)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (enrollment_id, resource_stable_id) DO UPDATE
			SET dwell_secs = resource_progress.dwell_secs + $3,
			    last_position_secs = GREATEST(resource_progress.last_position_secs, $4),
			    updated_at = now()
		RETURNING dwell_secs, completed`,
		req.EnrollmentID, req.ResourceStableID, delta, position).Scan(&dwell, &completed); err != nil {
		internalError(c, err)
		return
	}

	minDwell := progress.MinDwellSeconds(resourceType, durationSecs)

	if req.EventType == "complete" && !completed {
		if dwell < minDwell {
			// El cliente dice "termine" pero la evidencia no lo respalda.
			s.audit.Log(ctx, id.UserID, audit.ActionProgressRejected, "enrollment", req.EnrollmentID,
				c.ClientIP(), map[string]any{
					"resource": req.ResourceStableID, "dwell": dwell, "required": minDwell})
			unprocessable(c, "Aun no se cumple el tiempo minimo de permanencia para dar por visto el recurso.",
				gin.H{"dwell_secs": dwell, "required_secs": minDwell})
			return
		}
		if _, err := s.db.ExecContext(ctx, `
			UPDATE resource_progress SET completed = TRUE, completed_at = now(), updated_at = now()
			WHERE enrollment_id = $1 AND resource_stable_id = $2`,
			req.EnrollmentID, req.ResourceStableID); err != nil {
			internalError(c, err)
			return
		}
		completed = true
	}

	result, err := progress.Recompute(ctx, s.db, req.EnrollmentID)
	if err != nil {
		internalError(c, err)
		return
	}
	if result.NewlyApproved {
		if err := s.queue.EnqueueBadge(req.EnrollmentID, s.cfg.MaxRetries); err != nil {
			internalError(c, err)
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"resource": gin.H{
			"stable_id": req.ResourceStableID, "dwell_secs": dwell,
			"required_secs": minDwell, "completed": completed,
		},
		"progress": result,
	})
}
