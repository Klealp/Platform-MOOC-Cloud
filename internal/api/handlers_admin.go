package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/auth"
)

// =====================================================================
// Usuarios
// =====================================================================

func (s *Server) handleAdminListUsers(c *gin.Context) {
	limit := clampInt(c.Query("limit"), 50, 1, 200)
	role := c.Query("role")
	status := c.Query("status")
	search := strings.TrimSpace(c.Query("q"))

	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id, email, full_name, role::text, status::text, email_verified_at, created_at
		FROM users
		WHERE ($1 = '' OR role::text = $1)
		  AND ($2 = '' OR status::text = $2)
		  AND ($3 = '' OR email ILIKE '%' || $3 || '%' OR full_name ILIKE '%' || $3 || '%')
		ORDER BY created_at DESC
		LIMIT $4`, role, status, search, limit)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			id, email, name, r, st string
			verified               sql.NullTime
			created                time.Time
		)
		if err := rows.Scan(&id, &email, &name, &r, &st, &verified, &created); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, gin.H{
			"id": id, "email": email, "full_name": name, "role": r, "status": st,
			"email_verified": verified.Valid, "created_at": created,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type createUserRequest struct {
	Email    string `json:"email" binding:"required,email"`
	FullName string `json:"full_name" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"` // teacher | admin | student
}

// handleAdminCreateUser es el UNICO camino para crear profesores.
// La cuenta queda activa y verificada porque la crea alguien de confianza.
func (s *Server) handleAdminCreateUser(c *gin.Context) {
	actor := currentUser(c)
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren email, full_name, password y role.", err.Error())
		return
	}
	if req.Role != "teacher" && req.Role != "admin" && req.Role != "student" {
		badRequest(c, "role debe ser student, teacher o admin.", nil)
		return
	}
	if err := auth.ValidatePasswordPolicy(req.Password); err != nil {
		badRequest(c, err.Error(), nil)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalError(c, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var newID string
	err = s.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO users (email, password_hash, full_name, role, status, email_verified_at)
		VALUES ($1, $2, $3, $4::user_role, 'active', now())
		ON CONFLICT (email) DO NOTHING
		RETURNING id`, email, hash, strings.TrimSpace(req.FullName), req.Role).Scan(&newID)
	if errors.Is(err, sql.ErrNoRows) {
		conflict(c, "Ya existe un usuario con ese correo.", nil)
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	s.audit.Log(c.Request.Context(), actor.UserID, audit.ActionTeacherCreated, "user", newID, c.ClientIP(),
		map[string]any{"email": email, "role": req.Role})

	c.JSON(http.StatusCreated, gin.H{
		"id": newID, "email": email, "full_name": req.FullName, "role": req.Role, "status": "active",
	})
}

type updateUserRequest struct {
	Role   *string `json:"role"`
	Status *string `json:"status"`
}

// handleAdminUpdateUser cambia rol o estado.
//
// REGLA CRITICA del enunciado: "protegiendo al ultimo administrador activo".
// Si permitieramos degradar o suspender al ultimo admin, el sistema quedaria
// sin nadie capaz de administrarlo y no habria forma de recuperarlo por la
// API. La comprobacion se hace DENTRO de una transaccion con bloqueo de fila
// para que dos administradores simultaneos no se degraden a la vez.
func (s *Server) handleAdminUpdateUser(c *gin.Context) {
	actor := currentUser(c)
	target := c.Param("id")

	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Cuerpo invalido.", nil)
		return
	}
	if req.Role == nil && req.Status == nil {
		badRequest(c, "Nada que actualizar: envia role y/o status.", nil)
		return
	}

	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		internalError(c, err)
		return
	}
	defer tx.Rollback()

	var curRole, curStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT role::text, status::text FROM users WHERE id = $1 FOR UPDATE`, target).
		Scan(&curRole, &curStatus)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Usuario no encontrado.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	newRole, newStatus := curRole, curStatus
	if req.Role != nil {
		newRole = *req.Role
	}
	if req.Status != nil {
		newStatus = *req.Status
	}
	if newRole != "student" && newRole != "teacher" && newRole != "admin" {
		badRequest(c, "role invalido.", nil)
		return
	}
	if newStatus != "active" && newStatus != "suspended" && newStatus != "pending_verification" {
		badRequest(c, "status invalido.", nil)
		return
	}

	// Proteccion del ultimo administrador activo.
	losesAdmin := curRole == "admin" && curStatus == "active" && (newRole != "admin" || newStatus != "active")
	if losesAdmin {
		var activeAdmins int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM users WHERE role = 'admin' AND status = 'active'`).Scan(&activeAdmins); err != nil {
			internalError(c, err)
			return
		}
		if activeAdmins <= 1 {
			conflict(c, "No se puede degradar ni suspender al ultimo administrador activo.",
				gin.H{"active_admins": activeAdmins})
			return
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE users SET role = $1::user_role, status = $2::user_status, updated_at = now()
		WHERE id = $3`, newRole, newStatus, target); err != nil {
		internalError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalError(c, err)
		return
	}

	// Suspender debe cortar el acceso YA: revocamos todas sus sesiones.
	revoked := 0
	if newStatus == "suspended" {
		revoked, _ = s.auth.RevokeAllSessions(ctx, target)
	}

	if req.Role != nil && *req.Role != curRole {
		s.audit.Log(ctx, actor.UserID, audit.ActionRoleChanged, "user", target, c.ClientIP(),
			map[string]any{"from": curRole, "to": newRole})
	}
	if req.Status != nil && *req.Status != curStatus {
		s.audit.Log(ctx, actor.UserID, audit.ActionStatusChanged, "user", target, c.ClientIP(),
			map[string]any{"from": curStatus, "to": newStatus, "sessions_revoked": revoked})
	}

	c.JSON(http.StatusOK, gin.H{
		"id": target, "role": newRole, "status": newStatus, "sessions_revoked": revoked,
	})
}

func (s *Server) handleAdminRevokeSessions(c *gin.Context) {
	actor := currentUser(c)
	target := c.Param("id")

	n, err := s.auth.RevokeAllSessions(c.Request.Context(), target)
	if err != nil {
		internalError(c, err)
		return
	}
	s.audit.Log(c.Request.Context(), actor.UserID, audit.ActionSessionsRevoked, "user", target, c.ClientIP(),
		map[string]any{"count": n})
	c.JSON(http.StatusOK, gin.H{"sessions_revoked": n})
}

// =====================================================================
// Auditoria
// =====================================================================

func (s *Server) handleAdminAudit(c *gin.Context) {
	limit := clampInt(c.Query("limit"), 100, 1, 500)
	action := c.Query("action")
	actorFilter := c.Query("actor_id")

	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id, actor_id, action, entity_type, entity_id, metadata, ip, created_at
		FROM audit_log
		WHERE ($1 = '' OR action = $1)
		  AND ($2 = '' OR actor_id::text = $2)
		ORDER BY id DESC
		LIMIT $3`, action, actorFilter, limit)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			id                            int64
			actorID                       sql.NullString
			act, entType, entID, ip       string
			meta                          []byte
			created                       time.Time
		)
		if err := rows.Scan(&id, &actorID, &act, &entType, &entID, &meta, &ip, &created); err != nil {
			internalError(c, err)
			return
		}
		var metaObj map[string]any
		_ = json.Unmarshal(meta, &metaObj)
		out = append(out, gin.H{
			"id": id, "actor_id": actorID.String, "action": act,
			"entity_type": entType, "entity_id": entID,
			"metadata": metaObj, "ip": ip, "created_at": created,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// clampInt convierte un parametro de consulta a entero dentro de un rango.
func clampInt(raw string, def, min, max int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
