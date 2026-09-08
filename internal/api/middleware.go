package api

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"mooc-platform/internal/auth"
	"mooc-platform/internal/metrics"
)

const (
	ctxIdentity  = "identity"
	ctxRequestID = "request_id"
)

// ------------------------- Contexto -------------------------

func requestID(c *gin.Context) string {
	if v, ok := c.Get(ctxRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// currentUser devuelve la identidad autenticada. Solo se usa dentro de rutas
// protegidas por requireAuth, donde siempre existe.
func currentUser(c *gin.Context) *auth.Identity {
	v, ok := c.Get(ctxIdentity)
	if !ok {
		return nil
	}
	id, _ := v.(*auth.Identity)
	return id
}

// ------------------------- Middlewares generales -------------------------

// requestIDMiddleware asigna un identificador a cada peticion y lo devuelve en
// la cabecera. Es la pieza minima de trazabilidad: con ese valor se encuentra
// la peticion en los logs de la API y en los del worker.
func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Set(ctxRequestID, rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

// observability registra la peticion y alimenta Prometheus.
// Usa c.FullPath() (el patron de ruta, "/api/v1/courses/:id") y no la URL
// concreta: si usaramos la URL, cada identificador crearia una serie de
// tiempo nueva y Prometheus explotaria en cardinalidad.
func observability() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		elapsed := time.Since(start)
		status := strconv.Itoa(c.Writer.Status())

		metrics.HTTPRequests.WithLabelValues(c.Request.Method, route, status).Inc()
		metrics.HTTPDuration.WithLabelValues(c.Request.Method, route).Observe(elapsed.Seconds())

		log.Printf("[%s] %s %s -> %s (%s)",
			requestID(c), c.Request.Method, c.Request.URL.Path, status, elapsed.Round(time.Millisecond))
	}
}

// recovery evita que un panic tumbe el proceso completo.
func recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[%s] PANIC: %v", requestID(c), r)
				if !c.Writer.Written() {
					fail(c, http.StatusInternalServerError, codeInternal, "Ocurrio un error interno.", nil)
				}
			}
		}()
		c.Next()
	}
}

// securityHeaders aplica cabeceras defensivas basicas.
// La proteccion contra XSS empieza por no confiar en el contenido almacenado:
// el Markdown se guarda crudo y se sanea al renderizar en el frontend.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Next()
	}
}

// ------------------------- Autenticacion -------------------------

// requireAuth exige un token de sesion valido.
//
// Nota sobre CSRF: la API acepta la credencial en la cabecera Authorization,
// no en una cookie. Un formulario de otro sitio no puede anadir cabeceras
// personalizadas, de modo que este diseno no es vulnerable a CSRF por
// construccion. Si mas adelante se usan cookies, habra que anadir SameSite y
// un token anti-CSRF.
func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
			unauthorized(c, "Falta la cabecera Authorization: Bearer <token>.")
			return
		}
		token := strings.TrimSpace(header[len("bearer "):])
		if token == "" {
			unauthorized(c, "Token vacio.")
			return
		}

		id, err := s.auth.ValidateSession(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidSession) {
				unauthorized(c, "Sesion invalida, revocada o expirada.")
				return
			}
			internalError(c, err)
			return
		}

		c.Set(ctxIdentity, id)
		c.Next()
	}
}

// requireRole restringe una ruta a ciertos roles globales.
func requireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}
	return func(c *gin.Context) {
		id := currentUser(c)
		if id == nil {
			unauthorized(c, "Se requiere autenticacion.")
			return
		}
		if !allowed[id.Role] {
			forbidden(c, "Tu rol no permite esta operacion.")
			return
		}
		c.Next()
	}
}
