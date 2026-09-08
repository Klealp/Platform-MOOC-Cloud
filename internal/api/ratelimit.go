package api

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/metrics"
)

// rateLimit implementa una ventana fija por minuto en Redis.
//
// Algoritmo: INCR sobre una llave que incluye el minuto actual; si el valor
// vuelve de 1 significa que la llave es nueva y se le pone caducidad. Es la
// forma mas simple y barata (dos comandos) y suficiente para el MVP. Un
// token bucket seria mas justo en los bordes de la ventana, pero no aporta
// nada al objetivo de la entrega.
//
// La llave es el usuario autenticado si lo hay, y la IP en caso contrario.
// Asi el login y el registro tambien quedan protegidos contra fuerza bruta.
func (s *Server) rateLimit(limitPerMin int, scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		subject := c.ClientIP()
		if id := currentUser(c); id != nil {
			subject = "u:" + id.UserID
		}
		window := time.Now().UTC().Format("200601021504") // hasta el minuto
		key := fmt.Sprintf("rl:%s:%s:%s", scope, subject, window)

		ctx := c.Request.Context()
		count, err := s.rdb.Incr(ctx, key).Result()
		if err != nil {
			// Si Redis no responde NO bloqueamos el trafico. El limite de tasa
			// es una proteccion, no una funcion critica: preferimos degradar
			// la proteccion antes que caer el servicio.
			c.Next()
			return
		}
		if count == 1 {
			s.rdb.Expire(ctx, key, 70*time.Second)
		}

		remaining := limitPerMin - int(count)
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(limitPerMin))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if int(count) > limitPerMin {
			route := c.FullPath()
			metrics.RateLimited.WithLabelValues(route).Inc()
			c.Header("Retry-After", "60")
			fail(c, http.StatusTooManyRequests, codeRateLimited,
				"Demasiadas peticiones. Intenta de nuevo en un minuto.", nil)
			return
		}
		c.Next()
	}
}
