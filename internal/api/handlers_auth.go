package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/auth"
)

// =====================================================================
// Registro y verificacion de correo
// =====================================================================

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	FullName string `json:"full_name" binding:"required"`
}

// handleRegister crea SIEMPRE un estudiante.
//
// El enunciado es explicito: "los profesores solo se crean por administracion".
// Por eso el rol no se lee del cuerpo de la peticion; si se leyera, cualquiera
// podria registrarse como profesor enviando un campo extra.
//
// La respuesta es la misma exista o no el correo, para no permitir enumerar
// cuentas registradas. La diferencia queda solo en la bitacora de auditoria.
func (s *Server) handleRegister(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Cuerpo invalido. Se esperan email, password y full_name.", err.Error())
		return
	}
	if err := auth.ValidatePasswordPolicy(req.Password); err != nil {
		badRequest(c, err.Error(), nil)
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	ctx := c.Request.Context()

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		internalError(c, err)
		return
	}

	var userID string
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO users (email, password_hash, full_name, role, status)
		VALUES ($1, $2, $3, 'student', 'pending_verification')
		ON CONFLICT (email) DO NOTHING
		RETURNING id`, email, hash, strings.TrimSpace(req.FullName)).Scan(&userID)

	if errors.Is(err, sql.ErrNoRows) {
		// El correo ya existe. Respondemos igual que en el caso exitoso.
		s.audit.Log(ctx, "", audit.ActionUserRegistered, "user", "", c.ClientIP(),
			map[string]any{"email": email, "outcome": "duplicate"})
		c.JSON(http.StatusAccepted, gin.H{
			"message": "Si el correo no estaba registrado, recibiras un enlace de verificacion.",
		})
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	token, err := s.auth.IssueAuthToken(ctx, userID, "verify_email")
	if err != nil {
		internalError(c, err)
		return
	}
	s.sendVerificationEmail(email, token)
	s.audit.Log(ctx, userID, audit.ActionUserRegistered, "user", userID, c.ClientIP(),
		map[string]any{"email": email, "outcome": "created"})

	body := gin.H{
		"message": "Si el correo no estaba registrado, recibiras un enlace de verificacion.",
	}
	// AYUDA DE DESARROLLO: en dev devolvemos el token para poder probar con
	// Postman sin abrir Mailpit. En produccion (APP_ENV=prod) nunca aparece.
	if s.cfg.AppEnv == "dev" {
		body["dev_verification_token"] = token
		body["dev_user_id"] = userID
	}
	c.JSON(http.StatusAccepted, body)
}

func (s *Server) sendVerificationEmail(email, token string) {
	link := s.cfg.PublicBaseURL + "/api/v1/auth/verify-email?token=" + token
	_ = s.queue.EnqueueEmail(email, "Verifica tu cuenta",
		"Confirma tu correo enviando este token a POST /api/v1/auth/verify-email\n\n"+
			"token: "+token+"\n\nEnlace de referencia: "+link)
}

type tokenRequest struct {
	Token string `json:"token" binding:"required"`
}

func (s *Server) handleVerifyEmail(c *gin.Context) {
	var req tokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Tambien se acepta ?token=... para poder abrir el enlace del correo.
		req.Token = c.Query("token")
		if req.Token == "" {
			badRequest(c, "Falta el campo token.", nil)
			return
		}
	}

	ctx := c.Request.Context()
	userID, err := s.auth.ConsumeAuthToken(ctx, req.Token, "verify_email")
	if errors.Is(err, auth.ErrInvalidToken) {
		badRequest(c, "El token es invalido, ya se uso o expiro.", nil)
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	if _, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET status = 'active', email_verified_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'pending_verification'`, userID); err != nil {
		internalError(c, err)
		return
	}

	s.audit.Log(ctx, userID, audit.ActionUserVerified, "user", userID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, gin.H{"message": "Cuenta verificada. Ya puedes iniciar sesion."})
}

type emailRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (s *Server) handleResendVerification(c *gin.Context) {
	var req emailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Falta el campo email.", nil)
		return
	}
	ctx := c.Request.Context()
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var userID string
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM users WHERE email = $1 AND status = 'pending_verification'`, email).Scan(&userID)
	if err == nil {
		if token, err := s.auth.IssueAuthToken(ctx, userID, "verify_email"); err == nil {
			s.sendVerificationEmail(email, token)
		}
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "Si la cuenta existe y esta pendiente, se envio un nuevo enlace."})
}

// =====================================================================
// Inicio y cierre de sesion
// =====================================================================

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

func (s *Server) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren email y password.", nil)
		return
	}
	ctx := c.Request.Context()
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var (
		userID, hash, fullName, role, status string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT id, password_hash, full_name, role::text, status::text
		FROM users WHERE email = $1`, email).
		Scan(&userID, &hash, &fullName, &role, &status)

	// Mismo mensaje para "no existe" y "contrasena incorrecta": distinguirlos
	// permitiria averiguar que correos estan registrados.
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !auth.CheckPassword(hash, req.Password)) {
		s.audit.Log(ctx, "", audit.ActionLoginFailed, "user", "", c.ClientIP(),
			map[string]any{"email": email})
		unauthorized(c, "Credenciales invalidas.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	switch status {
	case "pending_verification":
		forbidden(c, "Debes verificar tu correo antes de iniciar sesion.")
		return
	case "suspended":
		forbidden(c, "La cuenta esta suspendida.")
		return
	}

	token, sessionID, err := s.auth.CreateSession(ctx, userID, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		internalError(c, err)
		return
	}
	s.audit.Log(ctx, userID, audit.ActionLogin, "session", sessionID, c.ClientIP(), nil)

	c.JSON(http.StatusOK, gin.H{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(s.cfg.SessionTTL.Seconds()),
		"session_id":   sessionID,
		"user": gin.H{
			"id": userID, "email": email, "full_name": fullName, "role": role,
		},
	})
}

func (s *Server) handleLogout(c *gin.Context) {
	id := currentUser(c)
	if err := s.auth.RevokeSession(c.Request.Context(), id.SessionID); err != nil {
		internalError(c, err)
		return
	}
	s.audit.Log(c.Request.Context(), id.UserID, audit.ActionLogout, "session", id.SessionID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, gin.H{"message": "Sesion cerrada."})
}

func (s *Server) handleMe(c *gin.Context) {
	id := currentUser(c)
	c.JSON(http.StatusOK, gin.H{
		"id": id.UserID, "email": id.Email, "full_name": id.FullName,
		"role": id.Role, "status": id.Status, "session_id": id.SessionID,
	})
}

func (s *Server) handleListSessions(c *gin.Context) {
	id := currentUser(c)
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id, user_agent, ip, created_at, last_seen_at, expires_at
		FROM sessions
		WHERE user_id = $1 AND revoked_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC`, id.UserID)
	if err != nil {
		internalError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var (
			sid, ua, ip                     string
			created, lastSeen, expires      time.Time
		)
		if err := rows.Scan(&sid, &ua, &ip, &created, &lastSeen, &expires); err != nil {
			internalError(c, err)
			return
		}
		out = append(out, gin.H{
			"id": sid, "user_agent": ua, "ip": ip,
			"created_at": created, "last_seen_at": lastSeen, "expires_at": expires,
			"current": sid == id.SessionID,
		})
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// handleRevokeSession solo permite revocar sesiones PROPIAS. Se comprueba la
// pertenencia en el WHERE, no en el codigo: asi no hay forma de saltarse la
// verificacion por un camino olvidado.
func (s *Server) handleRevokeSession(c *gin.Context) {
	id := currentUser(c)
	target := c.Param("id")

	var owner string
	err := s.db.QueryRowContext(c.Request.Context(),
		`SELECT user_id FROM sessions WHERE id = $1 AND user_id = $2`, target, id.UserID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		notFound(c, "Sesion no encontrada.")
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}
	if err := s.auth.RevokeSession(c.Request.Context(), target); err != nil {
		internalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Sesion revocada."})
}

// =====================================================================
// Recuperacion de contrasena
// =====================================================================

func (s *Server) handleForgotPassword(c *gin.Context) {
	var req emailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Falta el campo email.", nil)
		return
	}
	ctx := c.Request.Context()
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var userID string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&userID)

	body := gin.H{"message": "Si la cuenta existe, recibiras instrucciones por correo."}

	if err == nil {
		token, terr := s.auth.IssueAuthToken(ctx, userID, "reset_password")
		if terr == nil {
			_ = s.queue.EnqueueEmail(email, "Restablece tu contrasena",
				"Usa este token en POST /api/v1/auth/password/reset\n\ntoken: "+token)
			if s.cfg.AppEnv == "dev" {
				body["dev_reset_token"] = token
			}
		}
	}
	c.JSON(http.StatusAccepted, body)
}

type resetRequest struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

// handleResetPassword cambia la contrasena y REVOCA todas las sesiones
// abiertas. Si alguien habia entrado con la contrasena robada, pierde el
// acceso en ese instante.
func (s *Server) handleResetPassword(c *gin.Context) {
	var req resetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Se requieren token y new_password.", nil)
		return
	}
	if err := auth.ValidatePasswordPolicy(req.NewPassword); err != nil {
		badRequest(c, err.Error(), nil)
		return
	}
	ctx := c.Request.Context()

	userID, err := s.auth.ConsumeAuthToken(ctx, req.Token, "reset_password")
	if errors.Is(err, auth.ErrInvalidToken) {
		badRequest(c, "El token es invalido, ya se uso o expiro.", nil)
		return
	}
	if err != nil {
		internalError(c, err)
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		internalError(c, err)
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`, hash, userID); err != nil {
		internalError(c, err)
		return
	}
	revoked, err := s.auth.RevokeAllSessions(ctx, userID)
	if err != nil {
		internalError(c, err)
		return
	}
	s.audit.Log(ctx, userID, audit.ActionPasswordReset, "user", userID, c.ClientIP(),
		map[string]any{"sessions_revoked": revoked})

	c.JSON(http.StatusOK, gin.H{
		"message":          "Contrasena actualizada.",
		"sessions_revoked": revoked,
	})
}
