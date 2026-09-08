package api

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// El enunciado exige "errores uniformes". Toda respuesta de error del sistema
// tiene exactamente esta forma:
//
//	{"error": {"code": "...", "message": "...", "details": {...},
//	           "request_id": "..."}}
//
// request_id permite correlacionar el error que vio el usuario con la linea
// del log del servidor.
type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Details   any    `json:"details,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// Codigos estables. El cliente puede programar contra ellos; los mensajes son
// para humanos y pueden cambiar.
const (
	codeValidation   = "validation_error"
	codeUnauthorized = "unauthorized"
	codeForbidden    = "forbidden"
	codeNotFound     = "not_found"
	codeConflict     = "conflict"
	codeRateLimited  = "rate_limited"
	codeUnprocess    = "unprocessable"
	codeInternal     = "internal_error"
	codeTooLarge     = "payload_too_large"
)

func fail(c *gin.Context, status int, code, message string, details any) {
	c.AbortWithStatusJSON(status, errorEnvelope{Error: errorBody{
		Code:      code,
		Message:   message,
		Details:   details,
		RequestID: requestID(c),
	}})
}

func badRequest(c *gin.Context, message string, details any) {
	fail(c, http.StatusBadRequest, codeValidation, message, details)
}

func unauthorized(c *gin.Context, message string) {
	fail(c, http.StatusUnauthorized, codeUnauthorized, message, nil)
}

func forbidden(c *gin.Context, message string) {
	fail(c, http.StatusForbidden, codeForbidden, message, nil)
}

// notFound se usa TAMBIEN cuando el recurso existe pero pertenece a otro
// usuario. Responder 403 confirmaria que el identificador es real y filtraria
// informacion; el enunciado pide denegar "sin revelar informacion".
func notFound(c *gin.Context, message string) {
	fail(c, http.StatusNotFound, codeNotFound, message, nil)
}

func conflict(c *gin.Context, message string, details any) {
	fail(c, http.StatusConflict, codeConflict, message, details)
}

func unprocessable(c *gin.Context, message string, details any) {
	fail(c, http.StatusUnprocessableEntity, codeUnprocess, message, details)
}

// internalError registra el error real en el servidor y devuelve al cliente
// un mensaje generico: los detalles internos no salen nunca.
func internalError(c *gin.Context, err error) {
	log.Printf("[%s] error interno: %v", requestID(c), err)
	fail(c, http.StatusInternalServerError, codeInternal, "Ocurrio un error interno.", nil)
}
