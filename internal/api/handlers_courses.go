package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/auth"
)

// =====================================================================
// Contexto de autoria y control de acceso por propiedad
// =====================================================================

// authoringCtx reune los datos que hacen falta para decidir si alguien puede
// tocar un elemento del arbol de contenido, sin importar a que profundidad
// este (modulo, unidad, recurso, pregunta...).
type authoringCtx struct {
	CourseID      string
	VersionID     string
	OwnerID       string
	VersionStatus string
}

// canEdit resume las dos reglas que gobiernan toda la autoria:
//
//  1. Propiedad: solo el autor del curso (o un administrador) puede editarlo.
//  2. Inmutabilidad: una version publicada NO se modifica jamas. Para cambiar
//     algo se crea una version nueva en borrador.
func (a authoringCtx) canEdit(id *auth.Identity) error {
	if id.Role != "admin" && a.OwnerID != id.UserID {
		return errNotOwner
	}
	if a.VersionStatus != "draft" {
		return errImmutable
	}
	return nil
}

var (
	errNotOwner  = errors.New("no es el propietario")
	errImmutable = errors.New("version publicada inmutable")
)

// resolveAuthoring sube por el arbol desde cualquier nodo hasta el curso.
// Un solo lugar hace estas uniones; los handlers no repiten SQL de permisos.
func (s *Server) resolveAuthoring(ctx context.Context, level, id string) (authoringCtx, error) {
	var q string
	switch level {
	case "course":
		q = `SELECT c.id, '', c.owner_id, 'draft' FROM courses c WHERE c.id = $1`
	case "version":
		q = `SELECT c.id, v.id, c.owner_id, v.status::text
		     FROM course_versions v JOIN courses c ON c.id = v.course_id WHERE v.id = $1`
	case "module":
		q = `SELECT c.id, v.id, c.owner_id, v.status::text
		     FROM modules m
		     JOIN course_versions v ON v.id = m.version_id
		     JOIN courses c ON c.id = v.course_id WHERE m.id = $1`
	case "unit":
		q = `SELECT c.id, v.id, c.owner_id, v.status::text
		     FROM units u
		     JOIN modules m ON m.id = u.module_id
		     JOIN course_versions v ON v.id = m.version_id
		     JOIN courses c ON c.id = v.course_id WHERE u.id = $1`
	case "resource":
		q = `SELECT c.id, v.id, c.owner_id, v.status::text
		     FROM resources r
		     JOIN units u ON u.id = r.unit_id
		     JOIN modules m ON m.id = u.module_id
		     JOIN course_versions v ON v.id = m.version_id
		     JOIN courses c ON c.id = v.course_id WHERE r.id = $1`
	case "quiz":
		q = `SELECT c.id, v.id, c.owner_id, v.status::text
		     FROM quizzes qz
		     JOIN resources r ON r.id = qz.resource_id
		     JOIN units u ON u.id = r.unit_id
		     JOIN modules m ON m.id = u.module_id
		     JOIN course_versions v ON v.id = m.version_id
		     JOIN courses c ON c.id = v.course_id WHERE qz.id = $1`
	case "question":
		q = `SELECT c.id, v.id, c.owner_id, v.status::text
		     FROM quiz_questions qq
		     JOIN quizzes qz ON qz.id = qq.quiz_id
		     JOIN resources r ON r.id = qz.resource_id
		     JOIN units u ON u.id = r.unit_id
		     JOIN modules m ON m.id = u.module_id
		     JOIN course_versions v ON v.id = m.version_id
		     JOIN courses c ON c.id = v.course_id WHERE qq.id = $1`
	default:
		return authoringCtx{}, fmt.Errorf("nivel desconocido: %s", level)
	}

	var a authoringCtx
	err := s.db.QueryRowContext(ctx, q, id).Scan(&a.CourseID, &a.VersionID, &a.OwnerID, &a.VersionStatus)
	return a, err
}

// guard es el atajo que usan casi todos los handlers de autoria: resuelve el
// contexto, aplica las reglas y escribe la respuesta de error si procede.
// Devuelve false cuando el handler debe detenerse.
func (s *Server) guard(c *gin.Context, level, id string) (authoringCtx, bool) {
	a, err := s.resolveAuthoring(c.Request.Context(), level, id)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Recurso no encontrado.")
		return a, false
	}
	if err != nil {
		internalError(c, err)
		return a, false
	}
	switch err := a.canEdit(currentUser(c)); {
	case errors.Is(err, errNotOwner):
		// 404 y no 403: no confirmamos la existencia de contenido ajeno.
		notFound(c, "Recurso no encontrado.")
		return a, false
	case errors.Is(err, errImmutable):
		conflict(c, "La version esta publicada y es inmutable. Crea una version nueva para editar.", nil)
		return a, false
	}
	return a, true
}

// =====================================================================
// Cursos
// =====================================================================

type courseRequest struct {
	Title    string   `json:"title" binding:"required"`
	Summary  string   `json:"summary"`
	Language string   `json:"language"`
	Level    string   `json:"level"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}

// handleCreateCourse crea el curso y, con el, su primera version en borrador.
// Curso y version se crean juntos porque un curso sin version no es editable
// ni publicable: seria un estado intermedio inutil.
func (s *Server) handleCreateCourse(c *gin.Context) {
	id := currentUser(c)
	var req courseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requiere al menos el campo title.", err.Error())
		return
	}

	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	slug, err := uniqueSlug(ctx, tx, req.Title)
	if err != nil {
		internalError(c, err)
		return
	}

	if req.Tags == nil {
    req.Tags = []string{}
}

	var courseID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO courses (slug, title, summary, owner_id, language, level, category, tags)
		VALUES ($1, $2, $3, $4, COALESCE(NULLIF($5,''),'es'), COALESCE(NULLIF($6,''),'beginner'), $7, $8)
		RETURNING id`,
		slug, strings.TrimSpace(req.Title), req.Summary, id.UserID,
		req.Language, req.Level, req.Category, pq.Array(req.Tags)).Scan(&courseID)
	if err != nil {
		internalError(c, err)
		return
	}

	var versionID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO course_versions (course_id, version_number, status)
		VALUES ($1, 1, 'draft') RETURNING id`, courseID).Scan(&versionID); err != nil {
		internalError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id": courseID, "slug": slug, "title": req.Title, "status": "draft",
		"draft_version": gin.H{"id": versionID, "version_number": 1},
	})
}

func (s *Server) handleListMyCourses(c *gin.Context) {
	id := currentUser(c)
	// Un administrador ve todo; un profesor solo lo suyo. La condicion va en
	// el WHERE para que no exista forma de omitirla.
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT c.id, c.slug, c.title, c.status::text, c.category, c.updated_at,
		       COALESCE((SELECT count(*) FROM enrollments e
		                 WHERE e.course_id = c.id AND e.status = 'active'), 0)
		FROM courses c
		WHERE ($1 = 'admin' OR c.owner_id = $2)
		ORDER BY c.updated_at DESC`, id.Role, id.UserID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var cid, slug, title, status, category string
		var updated time.Time
		var enrolled int
		if err := rows.Scan(&cid, &slug, &title, &status, &category, &updated, &enrolled); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, gin.H{
			"id": cid, "slug": slug, "title": title, "status": status,
			"category": category, "updated_at": updated, "active_enrollments": enrolled,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (s *Server) handleGetCourse(c *gin.Context) {
	a, ok := s.guardCourseRead(c)
	if !ok {
		return
	}

	var (
		slug, title, summary, status, lang, level, category string
		tags                                                pq.StringArray
		currentVersion                                      sql.NullString
		created, updated                                    time.Time
	)
	err := s.db.QueryRowContext(c.Request.Context(), `
		SELECT slug, title, summary, status::text, language, level, category, tags,
		       current_version_id, created_at, updated_at
		FROM courses WHERE id = $1`, a.CourseID).
		Scan(&slug, &title, &summary, &status, &lang, &level, &category, &tags,
			&currentVersion, &created, &updated)
	if err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id": a.CourseID, "slug": slug, "title": title, "summary": summary,
		"status": status, "language": lang, "level": level, "category": category,
		"tags": []string(tags), "current_version_id": currentVersion.String,
		"created_at": created, "updated_at": updated,
	})
}

// guardCourseRead permite LEER un curso propio aunque este publicado (a
// diferencia de guard(), que ademas exige que sea editable).
func (s *Server) guardCourseRead(c *gin.Context) (authoringCtx, bool) {
	a, err := s.resolveAuthoring(c.Request.Context(), "course", c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Curso no encontrado.")
		return a, false
	}
	if err != nil {
		internalError(c, err)
		return a, false
	}
	id := currentUser(c)
	if id.Role != "admin" && a.OwnerID != id.UserID {
		notFound(c, "Curso no encontrado.")
		return a, false
	}
	return a, true
}

// handleUpdateCourse edita los metadatos del curso (no su contenido).
func (s *Server) handleUpdateCourse(c *gin.Context) {
	a, ok := s.guardCourseRead(c)
	if !ok {
		return
	}
	var req courseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Cuerpo invalido.", err.Error())
		return
	}
	_, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE courses SET
			title = COALESCE(NULLIF($1,''), title),
			summary = COALESCE(NULLIF($2,''), summary),
			language = COALESCE(NULLIF($3,''), language),
			level = COALESCE(NULLIF($4,''), level),
			category = COALESCE(NULLIF($5,''), category),
			tags = CASE WHEN $6::text[] IS NULL THEN tags ELSE $6::text[] END,
			updated_at = now()
		WHERE id = $7`,
		req.Title, req.Summary, req.Language, req.Level, req.Category,
		pq.Array(req.Tags), a.CourseID)
	if err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Curso actualizado."})
}

func (s *Server) handleListVersions(c *gin.Context) {
	a, ok := s.guardCourseRead(c)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id, version_number, status::text, passing_score, required_completion,
		       published_at, created_at
		FROM course_versions WHERE course_id = $1 ORDER BY version_number DESC`, a.CourseID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			vid, status              string
			num                      int
			pass, req                float64
			published                sql.NullTime
			created                  time.Time
		)
		if err := rows.Scan(&vid, &num, &status, &pass, &req, &published, &created); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, gin.H{
			"id": vid, "version_number": num, "status": status,
			"passing_score": pass, "required_completion": req,
			"published_at": nullTime(published), "created_at": created,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// handleCreateDraftVersion crea una version nueva CLONANDO la vigente.
//
// Este es el mecanismo que concilia dos exigencias opuestas del enunciado:
// "una version publicada es inmutable" y "el progreso del estudiante debe
// conservarse al publicar una actualizacion". La clonacion copia los
// stable_id tal cual; como el progreso apunta a stable_id y no al id fisico,
// el avance del estudiante sigue siendo valido en la version nueva.
func (s *Server) handleCreateDraftVersion(c *gin.Context) {
	a, ok := s.guardCourseRead(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var existingDraft string
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM course_versions WHERE course_id = $1 AND status = 'draft'`, a.CourseID).
		Scan(&existingDraft)
	if err == nil {
		conflict(c, "Ya existe una version en borrador para este curso.",
			gin.H{"draft_version_id": existingDraft})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		internalError(c, err)
		return
	}

	var sourceID sql.NullString
	var nextNum int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_number), 0) + 1 FROM course_versions WHERE course_id = $1`,
		a.CourseID).Scan(&nextNum); err != nil {
		internalError(c, err)
		return
	}
	_ = tx.QueryRowContext(ctx,
		`SELECT current_version_id FROM courses WHERE id = $1`, a.CourseID).Scan(&sourceID)

	var newVersionID string
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO course_versions (course_id, version_number, status, passing_score, required_completion)
		SELECT $1::uuid, $2::int, 'draft'::version_status,
		       COALESCE((SELECT passing_score FROM course_versions WHERE id = $3::uuid), 70.00),
		       COALESCE((SELECT required_completion FROM course_versions WHERE id = $3::uuid), 100.00)
		RETURNING id`, a.CourseID, nextNum, sourceID).Scan(&newVersionID); err != nil {
		internalError(c, err)
		return
	}

	if sourceID.Valid {
		if err := cloneVersionTree(ctx, tx, sourceID.String, newVersionID); err != nil {
			internalError(c, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	s.audit.Log(ctx, currentUser(c).UserID, audit.ActionVersionCreated, "course_version", newVersionID,
		c.ClientIP(), map[string]any{"course_id": a.CourseID, "version_number": nextNum})

	c.JSON(http.StatusCreated, gin.H{
		"id": newVersionID, "version_number": nextNum, "status": "draft",
		"cloned_from": sourceID.String,
	})
}

// cloneVersionTree copia modulos, unidades, recursos y quizzes preservando
// stable_id. Se hace con INSERT ... SELECT para que todo ocurra dentro del
// motor de base de datos, sin traer el arbol a memoria de la API.
func cloneVersionTree(ctx context.Context, tx *sql.Tx, srcVersion, dstVersion string) error {
	// Modulos
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO modules (version_id, stable_id, title, position)
		SELECT $1, stable_id, title, position FROM modules WHERE version_id = $2`,
		dstVersion, srcVersion); err != nil {
		return fmt.Errorf("clonar modulos: %w", err)
	}
	// Unidades: se emparejan por stable_id del modulo.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO units (module_id, stable_id, title, position)
		SELECT dm.id, su.stable_id, su.title, su.position
		FROM units su
		JOIN modules sm ON sm.id = su.module_id AND sm.version_id = $2
		JOIN modules dm ON dm.version_id = $1 AND dm.stable_id = sm.stable_id`,
		dstVersion, srcVersion); err != nil {
		return fmt.Errorf("clonar unidades: %w", err)
	}
	// Recursos
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO resources (unit_id, stable_id, type, title, position, visible,
		                       downloadable, required, content_md, external_url,
		                       asset_id, processing_status)
		SELECT du.id, sr.stable_id, sr.type, sr.title, sr.position, sr.visible,
		       sr.downloadable, sr.required, sr.content_md, sr.external_url,
		       sr.asset_id, sr.processing_status
		FROM resources sr
		JOIN units su ON su.id = sr.unit_id
		JOIN modules sm ON sm.id = su.module_id AND sm.version_id = $2
		JOIN modules dm ON dm.version_id = $1 AND dm.stable_id = sm.stable_id
		JOIN units du ON du.module_id = dm.id AND du.stable_id = su.stable_id`,
		dstVersion, srcVersion); err != nil {
		return fmt.Errorf("clonar recursos: %w", err)
	}
	// Quizzes
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO quizzes (resource_id, max_attempts, time_limit_secs, passing_score,
		                     shuffle_questions, feedback)
		SELECT dr.id, sq.max_attempts, sq.time_limit_secs, sq.passing_score,
		       sq.shuffle_questions, sq.feedback
		FROM quizzes sq
		JOIN resources sr ON sr.id = sq.resource_id
		JOIN units su ON su.id = sr.unit_id
		JOIN modules sm ON sm.id = su.module_id AND sm.version_id = $2
		JOIN modules dm ON dm.version_id = $1 AND dm.stable_id = sm.stable_id
		JOIN units du ON du.module_id = dm.id AND du.stable_id = su.stable_id
		JOIN resources dr ON dr.unit_id = du.id AND dr.stable_id = sr.stable_id`,
		dstVersion, srcVersion); err != nil {
		return fmt.Errorf("clonar quizzes: %w", err)
	}
	// Preguntas
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO quiz_questions (quiz_id, stable_id, prompt, position, points, multiple)
		SELECT dq.id, sqq.stable_id, sqq.prompt, sqq.position, sqq.points, sqq.multiple
		FROM quiz_questions sqq
		JOIN quizzes sq ON sq.id = sqq.quiz_id
		JOIN resources sr ON sr.id = sq.resource_id
		JOIN units su ON su.id = sr.unit_id
		JOIN modules sm ON sm.id = su.module_id AND sm.version_id = $2
		JOIN modules dm ON dm.version_id = $1 AND dm.stable_id = sm.stable_id
		JOIN units du ON du.module_id = dm.id AND du.stable_id = su.stable_id
		JOIN resources dr ON dr.unit_id = du.id AND dr.stable_id = sr.stable_id
		JOIN quizzes dq ON dq.resource_id = dr.id`,
		dstVersion, srcVersion); err != nil {
		return fmt.Errorf("clonar preguntas: %w", err)
	}
	// Opciones
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO quiz_options (question_id, stable_id, text, is_correct, feedback, position)
		SELECT dqq.id, so.stable_id, so.text, so.is_correct, so.feedback, so.position
		FROM quiz_options so
		JOIN quiz_questions sqq ON sqq.id = so.question_id
		JOIN quizzes sq ON sq.id = sqq.quiz_id
		JOIN resources sr ON sr.id = sq.resource_id
		JOIN units su ON su.id = sr.unit_id
		JOIN modules sm ON sm.id = su.module_id AND sm.version_id = $2
		JOIN modules dm ON dm.version_id = $1 AND dm.stable_id = sm.stable_id
		JOIN units du ON du.module_id = dm.id AND du.stable_id = su.stable_id
		JOIN resources dr ON dr.unit_id = du.id AND dr.stable_id = sr.stable_id
		JOIN quizzes dq ON dq.resource_id = dr.id
		JOIN quiz_questions dqq ON dqq.quiz_id = dq.id AND dqq.stable_id = sqq.stable_id`,
		dstVersion, srcVersion); err != nil {
		return fmt.Errorf("clonar opciones: %w", err)
	}
	return nil
}

// =====================================================================
// Publicacion
// =====================================================================

type publishRequest struct {
	PassingScore       *float64 `json:"passing_score"`
	RequiredCompletion *float64 `json:"required_completion"`
}

// handlePublishVersion valida y publica.
//
// La validacion devuelve la LISTA COMPLETA de problemas, no el primero que
// encuentra. El enunciado lo pide de forma explicita ("lista exhaustiva de
// errores") y ademas es lo util: el profesor corrige todo de una vez en lugar
// de descubrir un error nuevo en cada intento.
func (s *Server) handlePublishVersion(c *gin.Context) {
	a, ok := s.guard(c, "version", c.Param("id"))
	if !ok {
		return
	}
	ctx := c.Request.Context()

	var req publishRequest
	_ = c.ShouldBindJSON(&req)
	if req.PassingScore != nil || req.RequiredCompletion != nil {
		if _, err := s.db.ExecContext(ctx, `
			UPDATE course_versions SET
				passing_score = COALESCE($1, passing_score),
				required_completion = COALESCE($2, required_completion),
				updated_at = now()
			WHERE id = $3`, req.PassingScore, req.RequiredCompletion, a.VersionID); err != nil {
			internalError(c, err)
			return
		}
	}

	problems, err := s.validateForPublication(ctx, a.CourseID, a.VersionID)
	if err != nil {
		internalError(c, err)
		return
	}
	if len(problems) > 0 {
		unprocessable(c, "La version no cumple los requisitos de publicacion.",
			gin.H{"problems": problems})
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	// La version anterior pasa a 'superseded' pero sigue existiendo intacta:
	// eso es lo que significa que sea inmutable.
	if _, err := tx.ExecContext(ctx, `
		UPDATE course_versions SET status = 'superseded'
		WHERE course_id = $1 AND status = 'published'`, a.CourseID); err != nil {
		internalError(c, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE course_versions SET status = 'published', published_at = now(), updated_at = now()
		WHERE id = $1`, a.VersionID); err != nil {
		internalError(c, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE courses SET status = 'published', current_version_id = $1, updated_at = now()
		WHERE id = $2`, a.VersionID, a.CourseID); err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	s.audit.Log(ctx, currentUser(c).UserID, audit.ActionCoursePublished, "course_version", a.VersionID,
		c.ClientIP(), map[string]any{"course_id": a.CourseID})

	c.JSON(http.StatusOK, gin.H{"message": "Version publicada.", "version_id": a.VersionID})
}

type publishProblem struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// validateForPublication implementa la condicion verificable "publicacion
// valida" del enunciado: metadatos completos, estructura minima, criterios de
// aprobacion y todos los recursos visibles disponibles.
func (s *Server) validateForPublication(ctx context.Context, courseID, versionID string) ([]publishProblem, error) {
	var problems []publishProblem

	// 1. Metadatos del curso.
	var title, summary, category string
	if err := s.db.QueryRowContext(ctx,
		`SELECT title, summary, category FROM courses WHERE id = $1`, courseID).
		Scan(&title, &summary, &category); err != nil {
		return nil, err
	}
	if strings.TrimSpace(title) == "" {
		problems = append(problems, publishProblem{"missing_title", "course.title", "El curso necesita titulo."})
	}
	if strings.TrimSpace(summary) == "" {
		problems = append(problems, publishProblem{"missing_summary", "course.summary", "El curso necesita una descripcion."})
	}
	if strings.TrimSpace(category) == "" {
		problems = append(problems, publishProblem{"missing_category", "course.category", "El curso necesita una categoria."})
	}

	// 2. Criterios de aprobacion.
	var passing, requiredPct float64
	if err := s.db.QueryRowContext(ctx,
		`SELECT passing_score, required_completion FROM course_versions WHERE id = $1`, versionID).
		Scan(&passing, &requiredPct); err != nil {
		return nil, err
	}
	if passing <= 0 || passing > 100 {
		problems = append(problems, publishProblem{"invalid_passing_score", "version.passing_score",
			"La nota minima debe estar entre 1 y 100."})
	}
	if requiredPct <= 0 || requiredPct > 100 {
		problems = append(problems, publishProblem{"invalid_required_completion", "version.required_completion",
			"El porcentaje exigido debe estar entre 1 y 100."})
	}

	// 3. Estructura minima: Curso -> Modulo -> Unidad -> Recurso visible.
	var modules, units, visibleResources, requiredResources int
	if err := s.db.QueryRowContext(ctx, `
		SELECT
			(SELECT count(*) FROM modules WHERE version_id = $1),
			(SELECT count(*) FROM units u JOIN modules m ON m.id = u.module_id WHERE m.version_id = $1),
			(SELECT count(*) FROM resources r
			   JOIN units u ON u.id = r.unit_id
			   JOIN modules m ON m.id = u.module_id
			  WHERE m.version_id = $1 AND r.visible),
			(SELECT count(*) FROM resources r
			   JOIN units u ON u.id = r.unit_id
			   JOIN modules m ON m.id = u.module_id
			  WHERE m.version_id = $1 AND r.visible AND r.required)
		`, versionID).Scan(&modules, &units, &visibleResources, &requiredResources); err != nil {
		return nil, err
	}
	if modules == 0 {
		problems = append(problems, publishProblem{"no_modules", "version.modules", "La version no tiene modulos."})
	}
	if units == 0 {
		problems = append(problems, publishProblem{"no_units", "version.units", "La version no tiene unidades."})
	}
	if visibleResources == 0 {
		problems = append(problems, publishProblem{"no_resources", "version.resources", "No hay recursos visibles."})
	}
	if requiredResources == 0 {
		problems = append(problems, publishProblem{"no_required_resources", "version.resources",
			"Debe existir al menos un recurso obligatorio para poder medir el avance."})
	}

	// Modulos vacios y unidades vacias.
	emptyRows, err := s.db.QueryContext(ctx, `
		SELECT 'module', m.id, m.title FROM modules m
		WHERE m.version_id = $1 AND NOT EXISTS (SELECT 1 FROM units u WHERE u.module_id = m.id)
		UNION ALL
		SELECT 'unit', u.id, u.title FROM units u
		JOIN modules m ON m.id = u.module_id
		WHERE m.version_id = $1 AND NOT EXISTS (SELECT 1 FROM resources r WHERE r.unit_id = u.id AND r.visible)`,
		versionID)
	if err != nil {
		return nil, err
	}
	defer emptyRows.Close()
	for emptyRows.Next() {
		var kind, id, title string
		if err := emptyRows.Scan(&kind, &id, &title); err != nil {
			return nil, err
		}
		problems = append(problems, publishProblem{
			Code: "empty_" + kind, Path: kind + ":" + id,
			Message: fmt.Sprintf("El %s \"%s\" esta vacio.", kind, title)})
	}

	// 4. Disponibilidad de cada recurso visible.
	resRows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.title, r.type::text, r.content_md, r.external_url,
		       COALESCE(a.status::text, ''), r.processing_status::text,
		       COALESCE(qz.id::text, ''),
		       COALESCE((SELECT count(*) FROM quiz_questions q WHERE q.quiz_id = qz.id), 0)
		FROM resources r
		JOIN units u ON u.id = r.unit_id
		JOIN modules m ON m.id = u.module_id
		LEFT JOIN assets a ON a.id = r.asset_id
		LEFT JOIN quizzes qz ON qz.resource_id = r.id
		WHERE m.version_id = $1 AND r.visible`, versionID)
	if err != nil {
		return nil, err
	}
	defer resRows.Close()

	for resRows.Next() {
		var (
			rid, title, rtype, content, extURL, assetStatus, procStatus, quizID string
			questionCount                                                       int
		)
		if err := resRows.Scan(&rid, &title, &rtype, &content, &extURL,
			&assetStatus, &procStatus, &quizID, &questionCount); err != nil {
			return nil, err
		}

		switch rtype {
		case "rich_text":
			if strings.TrimSpace(content) == "" {
				problems = append(problems, publishProblem{"empty_content", "resource:" + rid,
					fmt.Sprintf("El recurso \"%s\" no tiene contenido.", title)})
			}
		case "external_link":
			if strings.TrimSpace(extURL) == "" {
				problems = append(problems, publishProblem{"missing_url", "resource:" + rid,
					fmt.Sprintf("El enlace \"%s\" no tiene URL.", title)})
			}
		case "quiz":
			if quizID == "" || questionCount == 0 {
				problems = append(problems, publishProblem{"empty_quiz", "resource:" + rid,
					fmt.Sprintf("El quiz \"%s\" no tiene preguntas.", title)})
			}
		default: // video, audio, pdf, image, download
			if assetStatus == "" {
				problems = append(problems, publishProblem{"missing_asset", "resource:" + rid,
					fmt.Sprintf("El recurso \"%s\" no tiene archivo asociado.", title)})
			} else if assetStatus != "ready" {
				problems = append(problems, publishProblem{"asset_not_ready", "resource:" + rid,
					fmt.Sprintf("El archivo de \"%s\" aun no esta disponible (estado: %s).", title, assetStatus)})
			}
		}
	}

	// 5. Preguntas sin respuesta correcta: haria imposible aprobar.
	badQ, err := s.db.QueryContext(ctx, `
		SELECT qq.id, qq.prompt,
		       (SELECT count(*) FROM quiz_options o WHERE o.question_id = qq.id),
		       (SELECT count(*) FROM quiz_options o WHERE o.question_id = qq.id AND o.is_correct)
		FROM quiz_questions qq
		JOIN quizzes qz ON qz.id = qq.quiz_id
		JOIN resources r ON r.id = qz.resource_id
		JOIN units u ON u.id = r.unit_id
		JOIN modules m ON m.id = u.module_id
		WHERE m.version_id = $1`, versionID)
	if err != nil {
		return nil, err
	}
	defer badQ.Close()
	for badQ.Next() {
		var qid, prompt string
		var total, correct int
		if err := badQ.Scan(&qid, &prompt, &total, &correct); err != nil {
			return nil, err
		}
		if total < 2 {
			problems = append(problems, publishProblem{"too_few_options", "question:" + qid,
				fmt.Sprintf("La pregunta \"%s\" necesita al menos dos opciones.", trimTo(prompt, 40))})
		}
		if correct == 0 {
			problems = append(problems, publishProblem{"no_correct_option", "question:" + qid,
				fmt.Sprintf("La pregunta \"%s\" no tiene respuesta correcta.", trimTo(prompt, 40))})
		}
	}

	return problems, nil
}

// handleUnpublishCourse retira el curso del catalogo sin borrar nada. Las
// inscripciones y el progreso sobreviven.
func (s *Server) handleUnpublishCourse(c *gin.Context) {
	a, ok := s.guardCourseRead(c)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(),
		`UPDATE courses SET status = 'unpublished', updated_at = now() WHERE id = $1`, a.CourseID); err != nil {
		internalError(c, err)
		return
	}
	s.audit.Log(c.Request.Context(), currentUser(c).UserID, audit.ActionCourseUnpublish,
		"course", a.CourseID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, gin.H{"message": "Curso retirado del catalogo."})
}

// =====================================================================
// Utilidades
// =====================================================================

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	replacer := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ñ", "n", "ü", "u")
	s = replacer.Replace(s)
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "curso"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func uniqueSlug(ctx context.Context, tx *sql.Tx, title string) (string, error) {
	base := slugify(title)
	candidate := base
	for i := 2; i < 100; i++ {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM courses WHERE slug = $1)`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s-%d", base, i)
	}
	return "", errors.New("no se pudo generar un slug unico")
}

func trimTo(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func nullTime(t sql.NullTime) any {
	if !t.Valid {
		return nil
	}
	return t.Time
}
