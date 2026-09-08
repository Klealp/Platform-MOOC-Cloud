package api

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/audit"
)

// =====================================================================
// Insignias
//
// CONDICION VERIFICABLE: "Al alcanzar el estado approved, se crea una unica
// insignia verificable sin exponer el correo del estudiante."
//
// Dos piezas garantizan eso:
//   - La emision la hace el worker (tasks/badge.go) y esta protegida por un
//     indice UNIQUE parcial: una sola insignia viva por (curso, usuario).
//   - La URL publica usa public_code, un valor aleatorio que no revela ni el
//     correo ni el id del usuario, y la respuesta publica solo lleva el
//     nombre del estudiante y el curso.
// =====================================================================

func (s *Server) handleListMyBadges(c *gin.Context) {
	id := currentUser(c)
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT b.id, b.public_code, b.image_key, b.issued_at, b.revoked_at,
		       c.title, c.slug
		FROM badges b JOIN courses c ON c.id = b.course_id
		WHERE b.user_id = $1
		ORDER BY b.issued_at DESC`, id.UserID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			bid, code, imageKey, title, slug string
			issued                           time.Time
			revoked                          sql.NullTime
		)
		if err := rows.Scan(&bid, &code, &imageKey, &issued, &revoked, &title, &slug); err != nil {
			internalError(c, err)
			return
		}
		item := gin.H{
			"id": bid, "public_code": code, "course_title": title, "course_slug": slug,
			"issued_at": issued, "revoked_at": nullTime(revoked),
			"verify_url": s.cfg.PublicBaseURL + "/api/v1/public/badges/" + code,
		}
		if imageKey != "" {
			if url, err := s.signDelivery(c.Request.Context(), imageKey, ""); err == nil {
				item["image_url"] = url
			}
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// handleVerifyBadge es PUBLICO: cualquiera con el codigo puede comprobar la
// insignia. Fijate en lo que NO devuelve: correo, id de usuario, id de
// inscripcion o nota obtenida.
func (s *Server) handleVerifyBadge(c *gin.Context) {
	var (
		fullName, courseTitle, courseSlug, teacher string
		issued                                     time.Time
		revoked                                    sql.NullTime
		revokeReason                               string
	)
	err := s.db.QueryRowContext(c.Request.Context(), `
		SELECT u.full_name, c.title, c.slug, t.full_name, b.issued_at, b.revoked_at, b.revoke_reason
		FROM badges b
		JOIN users u ON u.id = b.user_id
		JOIN courses c ON c.id = b.course_id
		JOIN users t ON t.id = c.owner_id
		WHERE b.public_code = $1`, c.Param("code")).
		Scan(&fullName, &courseTitle, &courseSlug, &teacher, &issued, &revoked, &revokeReason)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "No existe una insignia con ese codigo.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	body := gin.H{
		"valid":        !revoked.Valid,
		"recipient":    fullName,
		"course":       courseTitle,
		"course_slug":  courseSlug,
		"issued_by":    teacher,
		"issued_at":    issued,
		"public_code":  c.Param("code"),
	}
	if revoked.Valid {
		body["revoked_at"] = revoked.Time
		body["revoke_reason"] = revokeReason
	}
	c.JSON(http.StatusOK, body)
}

type revokeBadgeRequest struct {
	Reason string `json:"reason"`
}

// handleRevokeBadge (administrador). No borra la fila: la marca. Una insignia
// revocada debe seguir siendo consultable, porque alguien puede tener el
// enlace y necesita saber que ya no es valida.
func (s *Server) handleRevokeBadge(c *gin.Context) {
	actor := currentUser(c)
	var req revokeBadgeRequest
	_ = c.ShouldBindJSON(&req)

	res, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE badges SET revoked_at = now(), revoke_reason = $1
		WHERE id = $2 AND revoked_at IS NULL`, req.Reason, c.Param("id"))
	if err != nil {
		internalError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		notFound(c, "Insignia no encontrada o ya revocada.")
		return
	}

	s.audit.Log(c.Request.Context(), actor.UserID, audit.ActionBadgeRevoked, "badge", c.Param("id"),
		c.ClientIP(), map[string]any{"reason": req.Reason})
	c.JSON(http.StatusOK, gin.H{"message": "Insignia revocada."})
}
