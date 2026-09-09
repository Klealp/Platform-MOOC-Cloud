package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/openapi"
)

// handleOpenAPISpec entrega el contrato crudo. Es lo que consume Swagger UI y
// tambien sirve para generar clientes o importarlo en Postman/Insomnia.
func (s *Server) handleOpenAPISpec(c *gin.Context) {
	c.Data(http.StatusOK, "application/yaml; charset=utf-8", openapi.Spec)
}

// handleDocs pinta Swagger UI. La pagina es minima: carga los assets de Swagger
// UI desde un CDN y le apunta al spec que sirve la propia API. Como el spec
// declara servers = http://localhost:8080/api/v1, el boton "Try it out" dispara
// las peticiones contra este mismo backend, sin configuracion extra.
//
// Los assets se cargan por CDN a proposito: no queremos versionar megabytes de
// JS/CSS de Swagger dentro del repositorio. Si hiciera falta funcionar sin
// internet, se cambiaria por swagger-ui-dist servido tambien con go:embed.
func (s *Server) handleDocs(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerUIHTML))
}

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="es">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Plataforma MOOC · API</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui.css">
  <style>body { margin: 0; } .topbar { display: none; }</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14/swagger-ui-bundle.js" crossorigin></script>
  <script>
    window.onload = function () {
      window.ui = SwaggerUIBundle({
        url: '/openapi.yaml',
        dom_id: '#swagger-ui',
        deepLinking: true,
        tryItOutEnabled: true,
        persistAuthorization: true,
      });
    };
  </script>
</body>
</html>`
