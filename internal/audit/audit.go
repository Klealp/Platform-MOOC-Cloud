// Package audit escribe la bitacora inmutable del sistema.
//
// Regla: la auditoria NUNCA hace fallar la operacion principal. Si no se
// puede escribir la bitacora, se registra en el log y se sigue. Perder una
// linea de auditoria es malo; perder la operacion del usuario es peor.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
)

type Logger struct {
	db *sql.DB
}

func New(db *sql.DB) *Logger { return &Logger{db: db} }

// Acciones registradas. Tenerlas como constantes evita cadenas sueltas
// escritas de formas distintas en cada handler.
const (
	ActionUserRegistered   = "user.registered"
	ActionUserVerified     = "user.email_verified"
	ActionLogin            = "user.login"
	ActionLoginFailed      = "user.login_failed"
	ActionLogout           = "user.logout"
	ActionPasswordReset    = "user.password_reset"
	ActionRoleChanged      = "admin.role_changed"
	ActionStatusChanged    = "admin.status_changed"
	ActionTeacherCreated   = "admin.teacher_created"
	ActionSessionsRevoked  = "admin.sessions_revoked"
	ActionCoursePublished  = "course.published"
	ActionCourseUnpublish  = "course.unpublished"
	ActionVersionCreated   = "course.version_created"
	ActionUploadInit       = "asset.upload_initiated"
	ActionUploadCompleted  = "asset.upload_completed"
	ActionAssetInfected    = "asset.infected"
	ActionAssetReady       = "asset.ready"
	ActionEnrolled         = "enrollment.created"
	ActionWithdrawn        = "enrollment.withdrawn"
	ActionProgressRejected = "progress.rejected"
	ActionQuizSubmitted    = "quiz.submitted"
	ActionBadgeIssued      = "badge.issued"
	ActionBadgeRevoked     = "badge.revoked"
	ActionJobDeadLettered  = "job.dead_lettered" // alerta: trabajo agoto reintentos
)

// Log inserta una linea. actorID vacio significa "sistema" (por ejemplo, un
// worker) y se guarda como NULL.
func (l *Logger) Log(ctx context.Context, actorID, action, entityType, entityID, ip string, metadata map[string]any) {
	if metadata == nil {
		metadata = map[string]any{}
	}
	meta, err := json.Marshal(metadata)
	if err != nil {
		meta = []byte("{}")
	}

	var actor any
	if actorID != "" {
		actor = actorID
	}

	if _, err := l.db.ExecContext(ctx, `
		INSERT INTO audit_log (actor_id, action, entity_type, entity_id, metadata, ip)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		actor, action, entityType, entityID, string(meta), ip,
	); err != nil {
		log.Printf("audit: no se pudo registrar %s: %v", action, err)
	}
}
