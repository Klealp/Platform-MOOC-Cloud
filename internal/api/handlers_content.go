package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// =====================================================================
// Modulos
// =====================================================================

type titleRequest struct {
	Title    string `json:"title" binding:"required"`
	Position *int   `json:"position"`
}

// updateTitleRequest no marca nada obligatorio: se usa para renombrar, para
// reordenar, o para ambas cosas a la vez.
type updateTitleRequest struct {
	Title    *string `json:"title"`
	Position *int    `json:"position"`
}

func (s *Server) handleCreateModule(c *gin.Context) {
	a, ok := s.guard(c, "version", c.Param("id"))
	if !ok {
		return
	}
	var req titleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requiere el campo title.", nil)
		return
	}

	ctx := c.Request.Context()
	var pos int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), 0) + 1 FROM modules WHERE version_id = $1`, a.VersionID).
		Scan(&pos); err != nil {
		internalError(c, err)
		return
	}

	var id string
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO modules (version_id, stable_id, title, position)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		a.VersionID, uuid.NewString(), strings.TrimSpace(req.Title), pos).Scan(&id); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "title": req.Title, "position": pos})
}

func (s *Server) handleUpdateModule(c *gin.Context) {
	a, ok := s.guard(c, "module", c.Param("id"))
	if !ok {
		return
	}
	var req updateTitleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Envia title y/o position.", nil)
		return
	}
	s.updateOrdered(c, "modules", "version_id", a.VersionID, c.Param("id"), req.Title, req.Position)
}

func (s *Server) handleDeleteModule(c *gin.Context) {
	if _, ok := s.guard(c, "module", c.Param("id")); !ok {
		return
	}
	s.deleteOrdered(c, "modules", c.Param("id"))
}

// =====================================================================
// Unidades
// =====================================================================

func (s *Server) handleCreateUnit(c *gin.Context) {
	if _, ok := s.guard(c, "module", c.Param("id")); !ok {
		return
	}
	var req titleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requiere el campo title.", nil)
		return
	}
	moduleID := c.Param("id")
	ctx := c.Request.Context()

	var pos int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), 0) + 1 FROM units WHERE module_id = $1`, moduleID).Scan(&pos); err != nil {
		internalError(c, err)
		return
	}
	var id string
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO units (module_id, stable_id, title, position)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		moduleID, uuid.NewString(), strings.TrimSpace(req.Title), pos).Scan(&id); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "title": req.Title, "position": pos})
}

func (s *Server) handleUpdateUnit(c *gin.Context) {
	if _, ok := s.guard(c, "unit", c.Param("id")); !ok {
		return
	}
	var req updateTitleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Envia title y/o position.", nil)
		return
	}
	var moduleID string
	if err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT module_id FROM units WHERE id = $1`, c.Param("id")).Scan(&moduleID); err != nil {
		internalError(c, err)
		return
	}
	s.updateOrdered(c, "units", "module_id", moduleID, c.Param("id"), req.Title, req.Position)
}

func (s *Server) handleDeleteUnit(c *gin.Context) {
	if _, ok := s.guard(c, "unit", c.Param("id")); !ok {
		return
	}
	s.deleteOrdered(c, "units", c.Param("id"))
}

// =====================================================================
// Recursos
// =====================================================================

type resourceRequest struct {
	Type         string  `json:"type" binding:"required"`
	Title        string  `json:"title" binding:"required"`
	Visible      *bool   `json:"visible"`
	Downloadable *bool   `json:"downloadable"`
	Required     *bool   `json:"required"`
	ContentMD    string  `json:"content_md"`
	ExternalURL  string  `json:"external_url"`
	AssetID      *string `json:"asset_id"`
	Position     *int    `json:"position"`
}

var validResourceTypes = map[string]bool{
	"rich_text": true, "image": true, "video": true, "audio": true,
	"pdf": true, "download": true, "external_link": true, "quiz": true,
}

// tiposConAsset indica que tipos exigen un archivo en el almacenamiento.
var tiposConAsset = map[string]bool{
	"image": true, "video": true, "audio": true, "pdf": true, "download": true,
}

func (s *Server) handleCreateResource(c *gin.Context) {
	if _, ok := s.guard(c, "unit", c.Param("id")); !ok {
		return
	}
	var req resourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren type y title.", err.Error())
		return
	}
	if !validResourceTypes[req.Type] {
		badRequest(c, "type invalido.", gin.H{"allowed": keysOf(validResourceTypes)})
		return
	}

	unitID := c.Param("id")
	ctx := c.Request.Context()
	id := currentUser(c)

	// Si se adjunta un archivo, debe pertenecer a quien edita. Sin esta
	// comprobacion un profesor podria referenciar el archivo de otro.
	if req.AssetID != nil && *req.AssetID != "" {
		var owner string
		err := s.db.QueryRowContext(ctx, `SELECT owner_id FROM assets WHERE id = $1`, *req.AssetID).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			badRequest(c, "El asset_id no existe.", nil)
			return
		}
		if err != nil {
			internalError(c, err)
			return
		}
		if owner != id.UserID && id.Role != "admin" {
			notFound(c, "El asset_id no existe.")
			return
		}
	}
	if tiposConAsset[req.Type] && (req.AssetID == nil || *req.AssetID == "") {
		badRequest(c, fmt.Sprintf("Un recurso de tipo %s requiere asset_id.", req.Type), nil)
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var pos int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(position), 0) + 1 FROM resources WHERE unit_id = $1`, unitID).Scan(&pos); err != nil {
		internalError(c, err)
		return
	}

	processing := "not_applicable"
	if req.Type == "video" || req.Type == "audio" {
		processing = "pending"
	}

	var resourceID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO resources (unit_id, stable_id, type, title, position, visible, downloadable,
		                       required, content_md, external_url, asset_id, processing_status)
		VALUES ($1, $2, $3::resource_type, $4, $5,
		        COALESCE($6, TRUE), COALESCE($7, FALSE), COALESCE($8, TRUE),
		        $9, $10, $11, $12::resource_processing)
		RETURNING id`,
		unitID, uuid.NewString(), req.Type, strings.TrimSpace(req.Title), pos,
		req.Visible, req.Downloadable, req.Required,
		normalizeMarkdown(req.ContentMD), strings.TrimSpace(req.ExternalURL),
		nullableUUID(req.AssetID), processing).Scan(&resourceID)
	if err != nil {
		internalError(c, err)
		return
	}

	// Un recurso de tipo quiz crea su quiz vacio automaticamente: asi el autor
	// puede empezar a agregar preguntas sin un paso intermedio.
	var quizID string
	if req.Type == "quiz" {
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO quizzes (resource_id) VALUES ($1) RETURNING id`, resourceID).Scan(&quizID); err != nil {
			internalError(c, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	out := gin.H{"id": resourceID, "type": req.Type, "title": req.Title, "position": pos,
		"processing_status": processing}
	if quizID != "" {
		out["quiz_id"] = quizID
	}
	c.JSON(http.StatusCreated, out)
}

type updateResourceRequest struct {
	Title        *string `json:"title"`
	Visible      *bool   `json:"visible"`
	Downloadable *bool   `json:"downloadable"`
	Required     *bool   `json:"required"`
	ExternalURL  *string `json:"external_url"`
	Position     *int    `json:"position"`
}

func (s *Server) handleUpdateResource(c *gin.Context) {
	if _, ok := s.guard(c, "resource", c.Param("id")); !ok {
		return
	}
	var req updateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Cuerpo invalido.", nil)
		return
	}
	ctx := c.Request.Context()
	resourceID := c.Param("id")

	if _, err := s.db.ExecContext(ctx, `
		UPDATE resources SET
			title        = COALESCE($1, title),
			visible      = COALESCE($2, visible),
			downloadable = COALESCE($3, downloadable),
			required     = COALESCE($4, required),
			external_url = COALESCE($5, external_url),
			updated_at   = now()
		WHERE id = $6`,
		req.Title, req.Visible, req.Downloadable, req.Required, req.ExternalURL, resourceID); err != nil {
		internalError(c, err)
		return
	}

	if req.Position != nil {
		var unitID string
		if err := s.db.QueryRowContext(ctx,
			`SELECT unit_id FROM resources WHERE id = $1`, resourceID).Scan(&unitID); err != nil {
			internalError(c, err)
			return
		}
		if err := s.reorder(ctx, "resources", "unit_id", unitID, resourceID, *req.Position); err != nil {
			internalError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "Recurso actualizado."})
}

func (s *Server) handleDeleteResource(c *gin.Context) {
	if _, ok := s.guard(c, "resource", c.Param("id")); !ok {
		return
	}
	s.deleteOrdered(c, "resources", c.Param("id"))
}

// =====================================================================
// Contenido y autosave
// =====================================================================

type contentRequest struct {
	ContentMD string `json:"content_md"`
	// ExpectedRevision permite deteccion de conflictos: si otra pestana
	// guardo mientras tanto, el numero ya no coincide y se responde 409 en
	// lugar de pisar el trabajo del otro editor.
	ExpectedRevision *int `json:"expected_revision"`
}

// handleAutosaveContent guarda el borrador del editor.
//
// Sobre "Markdown extendido canonico": el contenido se normaliza antes de
// guardar (finales de linea LF, sin espacios al final, un salto final). Que
// la representacion almacenada sea siempre la misma es lo que permite que la
// conversion editor -> Markdown -> editor sea reversible. La biyeccion
// completa contra el AST del editor se implementara junto con el frontend;
// aqui queda fijado el formato de almacenamiento.
func (s *Server) handleAutosaveContent(c *gin.Context) {
	if _, ok := s.guard(c, "resource", c.Param("id")); !ok {
		return
	}
	var req contentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requiere content_md.", nil)
		return
	}

	ctx := c.Request.Context()
	resourceID := c.Param("id")

	if req.ExpectedRevision != nil {
		var current int
		if err := s.db.QueryRowContext(ctx,
			`SELECT content_revision FROM resources WHERE id = $1`, resourceID).Scan(&current); err != nil {
			internalError(c, err)
			return
		}
		if current != *req.ExpectedRevision {
			conflict(c, "El contenido cambio desde tu ultima lectura.",
				gin.H{"current_revision": current, "your_revision": *req.ExpectedRevision})
			return
		}
	}

	var revision int
	var updated time.Time
	if err := s.db.QueryRowContext(ctx, `
		UPDATE resources
		SET content_md = $1, content_revision = content_revision + 1, updated_at = now()
		WHERE id = $2
		RETURNING content_revision, updated_at`,
		normalizeMarkdown(req.ContentMD), resourceID).Scan(&revision, &updated); err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"revision": revision, "saved_at": updated})
}

func (s *Server) handleGetContent(c *gin.Context) {
	a, err := s.resolveAuthoring(c.Request.Context(), "resource", c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Recurso no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	id := currentUser(c)
	if id.Role != "admin" && a.OwnerID != id.UserID {
		notFound(c, "Recurso no encontrado.")
		return
	}

	var content, title, rtype string
	var revision int
	var updated time.Time
	if err := s.db.QueryRowContext(c.Request.Context(), `
		SELECT content_md, title, type::text, content_revision, updated_at
		FROM resources WHERE id = $1`, c.Param("id")).
		Scan(&content, &title, &rtype, &revision, &updated); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": c.Param("id"), "title": title, "type": rtype,
		"content_md": content, "revision": revision, "updated_at": updated,
	})
}

// normalizeMarkdown fija la forma canonica del contenido almacenado.
func normalizeMarkdown(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = strings.ReplaceAll(md, "\r", "\n")
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	out := strings.Join(lines, "\n")
	out = strings.TrimRight(out, "\n")
	if out != "" {
		out += "\n"
	}
	return out
}

// =====================================================================
// Esquema de la version (previsualizacion del autor)
// =====================================================================

type outlineResource struct {
	ID               string `json:"id"`
	StableID         string `json:"stable_id"`
	Type             string `json:"type"`
	Title            string `json:"title"`
	Position         int    `json:"position"`
	Visible          bool   `json:"visible"`
	Required         bool   `json:"required"`
	Downloadable     bool   `json:"downloadable"`
	ProcessingStatus string `json:"processing_status"`
	AssetID          string `json:"asset_id,omitempty"`
	AssetStatus      string `json:"asset_status,omitempty"`
	QuizID           string `json:"quiz_id,omitempty"`
	ExternalURL      string `json:"external_url,omitempty"`
	HasContent       bool   `json:"has_content"`
}

type outlineUnit struct {
	ID        string            `json:"id"`
	StableID  string            `json:"stable_id"`
	Title     string            `json:"title"`
	Position  int               `json:"position"`
	Resources []outlineResource `json:"resources"`
}

type outlineModule struct {
	ID       string        `json:"id"`
	StableID string        `json:"stable_id"`
	Title    string        `json:"title"`
	Position int           `json:"position"`
	Units    []outlineUnit `json:"units"`
}

// buildOutline arma el arbol completo de una version con TRES consultas
// (modulos, unidades, recursos) en vez de una por nodo. Evitar el problema
// N+1 importa porque este endpoint lo llama cada estudiante al abrir el curso.
func (s *Server) buildOutline(ctx context.Context, versionID string, onlyVisible bool) ([]outlineModule, error) {
	modRows, err := s.db.QueryContext(ctx,
		`SELECT id, stable_id, title, position FROM modules WHERE version_id = $1 ORDER BY position`, versionID)
	if err != nil {
		return nil, err
	}
	defer modRows.Close()

	var modules []outlineModule
	byModuleID := map[string]int{}
	for modRows.Next() {
		var m outlineModule
		if err := modRows.Scan(&m.ID, &m.StableID, &m.Title, &m.Position); err != nil {
			return nil, err
		}
		m.Units = []outlineUnit{}
		byModuleID[m.ID] = len(modules)
		modules = append(modules, m)
	}
	if len(modules) == 0 {
		return []outlineModule{}, nil
	}

	unitRows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.stable_id, u.title, u.position, u.module_id
		FROM units u JOIN modules m ON m.id = u.module_id
		WHERE m.version_id = $1 ORDER BY m.position, u.position`, versionID)
	if err != nil {
		return nil, err
	}
	defer unitRows.Close()

	type unitPos struct{ mod, idx int }
	byUnitID := map[string]unitPos{}
	for unitRows.Next() {
		var u outlineUnit
		var moduleID string
		if err := unitRows.Scan(&u.ID, &u.StableID, &u.Title, &u.Position, &moduleID); err != nil {
			return nil, err
		}
		u.Resources = []outlineResource{}
		mi, ok := byModuleID[moduleID]
		if !ok {
			continue
		}
		byUnitID[u.ID] = unitPos{mod: mi, idx: len(modules[mi].Units)}
		modules[mi].Units = append(modules[mi].Units, u)
	}

	resRows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.stable_id, r.type::text, r.title, r.position, r.visible, r.required,
		       r.downloadable, r.processing_status::text, r.unit_id,
		       COALESCE(r.asset_id::text, ''), COALESCE(a.status::text, ''),
		       COALESCE(qz.id::text, ''), r.external_url, (length(r.content_md) > 0)
		FROM resources r
		JOIN units u ON u.id = r.unit_id
		JOIN modules m ON m.id = u.module_id
		LEFT JOIN assets a ON a.id = r.asset_id
		LEFT JOIN quizzes qz ON qz.resource_id = r.id
		WHERE m.version_id = $1 AND ($2 = FALSE OR r.visible = TRUE)
		ORDER BY m.position, u.position, r.position`, versionID, onlyVisible)
	if err != nil {
		return nil, err
	}
	defer resRows.Close()

	for resRows.Next() {
		var r outlineResource
		var unitID string
		if err := resRows.Scan(&r.ID, &r.StableID, &r.Type, &r.Title, &r.Position, &r.Visible,
			&r.Required, &r.Downloadable, &r.ProcessingStatus, &unitID,
			&r.AssetID, &r.AssetStatus, &r.QuizID, &r.ExternalURL, &r.HasContent); err != nil {
			return nil, err
		}
		p, ok := byUnitID[unitID]
		if !ok {
			continue
		}
		modules[p.mod].Units[p.idx].Resources = append(modules[p.mod].Units[p.idx].Resources, r)
	}

	return modules, nil
}

func (s *Server) handleVersionOutline(c *gin.Context) {
	a, err := s.resolveAuthoring(c.Request.Context(), "version", c.Param("id"))
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Version no encontrada.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	id := currentUser(c)
	if id.Role != "admin" && a.OwnerID != id.UserID {
		notFound(c, "Version no encontrada.")
		return
	}

	modules, err := s.buildOutline(c.Request.Context(), a.VersionID, false)
	if err != nil {
		internalError(c, err)
		return
	}

	// Previsualizacion: se anticipa si la version podria publicarse.
	problems, err := s.validateForPublication(c.Request.Context(), a.CourseID, a.VersionID)
	if err != nil {
		internalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"version_id":  a.VersionID,
		"course_id":   a.CourseID,
		"status":      a.VersionStatus,
		"modules":     modules,
		"publishable": len(problems) == 0,
		"problems":    problems,
	})
}

// =====================================================================
// Ordenamiento
// =====================================================================

// reorder recoloca un elemento dentro de sus hermanos y renumera todo el
// bloque de 1..N. Se hace en una transaccion; las restricciones UNIQUE de
// posicion son DEFERRABLE precisamente para que los valores intermedios
// puedan repetirse mientras dura la renumeracion.
func (s *Server) reorder(ctx context.Context, table, parentCol, parentID, itemID string, newPos int) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SET CONSTRAINTS ALL DEFERRED`); err != nil {
		return err
	}

	rows, err := tx.QueryContext(ctx,
		fmt.Sprintf(`SELECT id FROM %s WHERE %s = $1 ORDER BY position`, table, parentCol), parentID)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()

	// Quitar el elemento de su lugar actual.
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != itemID {
			filtered = append(filtered, id)
		}
	}
	if newPos < 1 {
		newPos = 1
	}
	if newPos > len(filtered)+1 {
		newPos = len(filtered) + 1
	}
	// Insertarlo en la posicion pedida.
	final := append([]string{}, filtered[:newPos-1]...)
	final = append(final, itemID)
	final = append(final, filtered[newPos-1:]...)

	for i, id := range final {
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET position = $1 WHERE id = $2`, table), i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Server) updateOrdered(c *gin.Context, table, parentCol, parentID, itemID string, title *string, position *int) {
	ctx := c.Request.Context()
	if title != nil && strings.TrimSpace(*title) != "" {
		if _, err := s.db.ExecContext(ctx,
			fmt.Sprintf(`UPDATE %s SET title = $1 WHERE id = $2`, table), strings.TrimSpace(*title), itemID); err != nil {
			internalError(c, err)
			return
		}
	}
	if position != nil {
		if err := s.reorder(ctx, table, parentCol, parentID, itemID, *position); err != nil {
			internalError(c, err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "Actualizado."})
}

// deleteOrdered borra el elemento. No renumera a los hermanos: los huecos en
// la secuencia de posiciones son inofensivos porque el orden se calcula con
// ORDER BY position, no con el valor exacto.
func (s *Server) deleteOrdered(c *gin.Context, table, itemID string) {
	ctx := c.Request.Context()
	if _, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, table), itemID); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Eliminado."})
}

func keysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func nullableUUID(v *string) any {
	if v == nil || *v == "" {
		return nil
	}
	return *v
}
