package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// idempotency implementa la cabecera Idempotency-Key que exige el enunciado.
//
// Problema que resuelve: el cliente envia el quiz, la red se cae antes de que
// llegue la respuesta y el cliente reintenta. Sin proteccion, el intento se
// calificaria dos veces. Con la cabecera, la segunda peticion recibe la MISMA
// respuesta de la primera, sin volver a ejecutar el efecto.
//
// Estados de la llave en Redis:
//
//	"processing"                -> hay una peticion en vuelo, se responde 409
//	{status, body} serializado  -> ya termino, se repite la respuesta guardada
//
// Se guarda tambien el hash del cuerpo: reutilizar la misma llave con un
// cuerpo distinto es un error del cliente y se rechaza con 422.
func (s *Server) idempotency(ttl time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		if key == "" {
			// La cabecera es opcional. Sin ella la peticion sigue su curso
			// normal; la responsabilidad de no duplicar queda del cliente.
			c.Next()
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			badRequest(c, "No se pudo leer el cuerpo de la peticion.", nil)
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(body)
		bodyHash := hex.EncodeToString(sum[:])

		user := "anon"
		if id := currentUser(c); id != nil {
			user = id.UserID
		}
		redisKey := "idem:" + user + ":" + c.FullPath() + ":" + key

		ctx := c.Request.Context()
		reserved, err := s.rdb.SetNX(ctx, redisKey, "processing:"+bodyHash, ttl).Result()
		if err != nil {
			internalError(c, err)
			return
		}

		if !reserved {
			stored, err := s.rdb.Get(ctx, redisKey).Result()
			if err != nil {
				internalError(c, err)
				return
			}
			if len(stored) > 11 && stored[:11] == "processing:" {
				if stored[11:] != bodyHash {
					unprocessable(c, "La Idempotency-Key ya se uso con un cuerpo diferente.", nil)
					return
				}
				conflict(c, "Ya hay una peticion en curso con esa Idempotency-Key.", nil)
				return
			}
			var saved storedResponse
			if uerr := json.Unmarshal([]byte(stored), &saved); uerr != nil {
				internalError(c, uerr)
				return
			}
			if saved.BodyHash != bodyHash {
				unprocessable(c, "La Idempotency-Key ya se uso con un cuerpo diferente.", nil)
				return
			}
			// Repeticion exacta de la respuesta original.
			c.Header("Idempotent-Replay", "true")
			c.Data(saved.Status, "application/json; charset=utf-8", saved.Body)
			c.Abort()
			return
		}

		// Interceptamos la escritura para poder guardar la respuesta.
		rec := &responseRecorder{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = rec
		c.Next()

		// Solo se memorizan respuestas definitivas. Un 5xx debe poder
		// reintentarse con la misma llave.
		if rec.Status() >= 200 && rec.Status() < 500 {
			payload, _ := json.Marshal(storedResponse{
				Status:   rec.Status(),
				Body:     rec.buf.Bytes(),
				BodyHash: bodyHash,
			})
			s.rdb.Set(ctx, redisKey, payload, ttl)
		} else {
			s.rdb.Del(ctx, redisKey)
		}
	}
}

type storedResponse struct {
	Status   int    `json:"status"`
	Body     []byte `json:"body"`
	BodyHash string `json:"body_hash"`
}

type responseRecorder struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseRecorder) WriteString(s string) (int, error) {
	r.buf.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}

var _ http.ResponseWriter = (*responseRecorder)(nil)
