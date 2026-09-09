// Package api contiene la capa HTTP: rutas, middlewares y handlers.
//
// La API es un MONOLITO MODULAR: un solo binario, pero el codigo esta separado
// por dominio (auth, cursos, contenido, media, quizzes, aprendizaje, insignias).
// Cada archivo handlers_*.go es un modulo. La ventaja frente a microservicios
// en esta etapa es operativa: un despliegue, una transaccion de base de datos,
// y aun asi limites claros que permitirian extraer un modulo mas adelante.
//
// La API NO mantiene estado local: no guarda archivos ni sesiones en memoria
// ni en el disco del contenedor. Todo vive en Postgres, Redis o el
// almacenamiento de objetos. Por eso se puede escalar a N instancias detras de
// un balanceador sin coordinacion alguna.
package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"mooc-platform/internal/audit"
	"mooc-platform/internal/auth"
	"mooc-platform/internal/config"
	"mooc-platform/internal/queue"
	"mooc-platform/internal/storage"
)

type Server struct {
	cfg   config.Config
	db    *sql.DB
	rdb   *redis.Client
	store *storage.Storage
	queue *queue.Client
	auth  *auth.Service
	audit *audit.Logger
}

func NewServer(cfg config.Config, db *sql.DB, rdb *redis.Client, st *storage.Storage, q *queue.Client) *Server {
	return &Server{
		cfg:   cfg,
		db:    db,
		rdb:   rdb,
		store: st,
		queue: q,
		auth:  auth.NewService(db, rdb, cfg),
		audit: audit.New(db),
	}
}

func (s *Server) Router() *gin.Engine {
	if s.cfg.AppEnv == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(recovery(), requestIDMiddleware(), observability(), securityHeaders())

	// --- Operacion ---
	r.GET("/healthz", s.handleHealth) // el proceso vive
	r.GET("/readyz", s.handleReady)   // ademas sus dependencias responden
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// --- Documentacion viva del contrato ---
	// El spec OpenAPI se sirve crudo y se pinta con Swagger UI. Ambos van fuera
	// de /api/v1 y sin sesion: son documentacion, no parte de la API versionada.
	r.GET("/openapi.yaml", s.handleOpenAPISpec)
	r.GET("/docs", s.handleDocs)

	v1 := r.Group("/api/v1")
	// El trazado OTel se aplica SOLO a la API versionada: asi el span del
	// servidor envuelve el handler y sus consultas SQL, pero se evitan las
	// trazas de ruido de /metrics (que Prometheus raspa cada 15s), /healthz y
	// /docs. Usa el TracerProvider global; si las trazas estan apagadas, no hace
	// nada.
	v1.Use(otelgin.Middleware("mooc-api"))

	// =================================================================
	// PUBLICO (sin sesion)
	// =================================================================
	pub := v1.Group("")
	pub.Use(s.rateLimit(30, "public"))
	{
		pub.POST("/auth/register", s.handleRegister)
		pub.POST("/auth/verify-email", s.handleVerifyEmail)
		pub.POST("/auth/resend-verification", s.handleResendVerification)
		pub.POST("/auth/login", s.handleLogin)
		pub.POST("/auth/password/forgot", s.handleForgotPassword)
		pub.POST("/auth/password/reset", s.handleResetPassword)

		// Verificacion publica de una insignia. No expone el correo.
		pub.GET("/public/badges/:code", s.handleVerifyBadge)
	}

	// Catalogo: visible sin sesion (solo cursos publicados).
	cat := v1.Group("")
	cat.Use(s.rateLimit(s.cfg.RateLimitPerMin, "catalog"))
	{
		cat.GET("/catalog", s.handleCatalogList)
		cat.GET("/catalog/:slug", s.handleCatalogDetail)
	}

	// =================================================================
	// AUTENTICADO
	// =================================================================
	priv := v1.Group("")
	priv.Use(s.requireAuth(), s.rateLimit(s.cfg.RateLimitPerMin, "private"))
	{
		// --- Sesion propia ---
		priv.GET("/auth/me", s.handleMe)
		priv.POST("/auth/logout", s.handleLogout)
		priv.GET("/auth/sessions", s.handleListSessions)
		priv.DELETE("/auth/sessions/:id", s.handleRevokeSession)

		// --- Administracion ---
		adm := priv.Group("/admin", requireRole("admin"))
		{
			adm.GET("/users", s.handleAdminListUsers)
			adm.POST("/users", s.handleAdminCreateUser) // unico modo de crear profesores
			adm.PATCH("/users/:id", s.handleAdminUpdateUser)
			adm.POST("/users/:id/revoke-sessions", s.handleAdminRevokeSessions)
			adm.GET("/audit", s.handleAdminAudit)
			adm.POST("/badges/:id/revoke", s.handleRevokeBadge)
		}

		// --- Autoria (profesor o administrador) ---
		aut := priv.Group("", requireRole("teacher", "admin"))
		{
			aut.POST("/courses", s.handleCreateCourse)
			aut.GET("/courses", s.handleListMyCourses)
			aut.GET("/courses/:id", s.handleGetCourse)
			aut.PATCH("/courses/:id", s.handleUpdateCourse)
			aut.GET("/courses/:id/versions", s.handleListVersions)
			aut.POST("/courses/:id/versions", s.handleCreateDraftVersion)
			aut.POST("/courses/:id/unpublish", s.handleUnpublishCourse)

			aut.GET("/versions/:id", s.handleVersionOutline) // previsualizacion
			aut.POST("/versions/:id/publish", s.handlePublishVersion)
			aut.POST("/versions/:id/modules", s.handleCreateModule)

			aut.PATCH("/modules/:id", s.handleUpdateModule)
			aut.DELETE("/modules/:id", s.handleDeleteModule)
			aut.POST("/modules/:id/units", s.handleCreateUnit)

			aut.PATCH("/units/:id", s.handleUpdateUnit)
			aut.DELETE("/units/:id", s.handleDeleteUnit)
			aut.POST("/units/:id/resources", s.handleCreateResource)

			aut.PATCH("/resources/:id", s.handleUpdateResource)
			aut.DELETE("/resources/:id", s.handleDeleteResource)
			aut.GET("/resources/:id/content", s.handleGetContent)
			aut.PUT("/resources/:id/content", s.handleAutosaveContent)

			// Autoria de quizzes (solo texto en esta entrega)
			aut.GET("/quizzes/:id", s.handleGetQuizAuthor)
			aut.PATCH("/quizzes/:id", s.handleUpdateQuiz)
			aut.POST("/quizzes/:id/questions", s.handleCreateQuestion)
			aut.PATCH("/questions/:id", s.handleUpdateQuestion)
			aut.DELETE("/questions/:id", s.handleDeleteQuestion)
		}

		// --- Carga de archivos (cualquier autor autenticado) ---
		med := priv.Group("", requireRole("teacher", "admin"))
		{
			med.POST("/uploads", s.handleInitUpload)
			med.GET("/uploads/:id", s.handleUploadStatus) // partes faltantes -> reanudar
			med.POST("/uploads/:id/complete", s.handleCompleteUpload)
			med.DELETE("/uploads/:id", s.handleAbortUpload)
		}
		// La consulta de un asset la hace tambien el estudiante inscrito.
		priv.GET("/assets/:id", s.handleGetAsset)
		priv.GET("/assets/:id/url", s.handleAssetDeliveryURL)
		priv.GET("/assets/:id/playlist", s.handleHLSPlaylist)

		// --- Aprendizaje ---
		priv.POST("/enrollments", s.handleEnroll)
		priv.GET("/enrollments", s.handleListEnrollments)
		priv.GET("/enrollments/:id", s.handleGetEnrollment)
		priv.DELETE("/enrollments/:id", s.handleWithdraw)
		priv.GET("/enrollments/:id/outline", s.handleEnrollmentOutline)
		priv.POST("/progress/events", s.handleProgressEvent)

		// --- Presentacion de quizzes ---
		priv.POST("/quizzes/:id/attempts", s.handleStartAttempt)
		priv.GET("/attempts/:id", s.handleGetAttempt)
		priv.PATCH("/attempts/:id", s.handleSaveAttempt) // guardado parcial
		// El envio definitivo es la operacion mas sensible a duplicados:
		// aqui si exigimos la cabecera Idempotency-Key.
		priv.POST("/attempts/:id/submit", s.idempotency(24*time.Hour), s.handleSubmitAttempt)

		// --- Insignias propias ---
		priv.GET("/badges", s.handleListMyBadges)
	}

	r.NoRoute(func(c *gin.Context) {
		notFound(c, "Ruta no encontrada.")
	})

	return r
}

// ------------------------- Salud -------------------------

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleReady es la sonda que deberia mirar un balanceador: comprueba que las
// dependencias externas responden antes de declarar la instancia lista.
func (s *Server) handleReady(c *gin.Context) {
	ctx := c.Request.Context()
	checks := gin.H{"postgres": "ok", "redis": "ok", "storage": "ok"}
	ready := true

	if err := s.db.PingContext(ctx); err != nil {
		checks["postgres"] = err.Error()
		ready = false
	}
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		checks["redis"] = err.Error()
		ready = false
	}
	if _, err := s.store.StatSize(ctx, "___probe___"); err != nil {
		// Que el objeto no exista es lo normal; lo que importa es que el
		// servicio conteste. Solo un fallo de conexion marca no-listo.
		if isConnectionError(err) {
			checks["storage"] = err.Error()
			ready = false
		}
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"ready": ready, "checks": checks})
}

func isConnectionError(err error) bool {
	msg := err.Error()
	for _, needle := range []string{"connection refused", "no such host", "timeout", "EOF"} {
		if len(msg) >= len(needle) && contains(msg, needle) {
			return true
		}
	}
	return false
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
