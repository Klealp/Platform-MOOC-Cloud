package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"mooc-platform/internal/config"
)

// El balanceador HTTPS externo de Google conecta desde sus rangos
// (130.211.0.0/22, 35.191.0.0/16) y agrega "<ip-cliente>,<ip-balanceador>"
// al final de X-Forwarded-For. Estas pruebas fijan que la API obtenga la IP
// real del cliente y que un cliente no pueda inventarse otra.
const ipBalanceador = "34.120.1.2"

func clientIPDetras(t *testing.T, proxies []string, remote, xff string) (ip, instancia string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	s := NewServer(config.Config{TrustedProxies: proxies, InstanceID: "mooc-api-x7k2"}, nil, nil, nil, nil)
	r := s.Router()
	r.GET("/_prueba_ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })

	req := httptest.NewRequest(http.MethodGet, "/_prueba_ip", nil)
	req.RemoteAddr = remote + ":51234"
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Body.String(), w.Header().Get("X-Instance-Id")
}

func TestClientIPDetrasDelBalanceador(t *testing.T) {
	proxies := []string{"130.211.0.0/22", "35.191.0.0/16", ipBalanceador + "/32"}
	ip, _ := clientIPDetras(t, proxies, "35.191.10.20", "203.0.113.7, "+ipBalanceador)
	if ip != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, se esperaba la IP del cliente 203.0.113.7", ip)
	}
}

func TestClientIPNoSeFalsificaConXForwardedFor(t *testing.T) {
	// El cliente manda su propio X-Forwarded-For; el balanceador lo conserva y
	// agrega la IP real a la derecha. Se lee de derecha a izquierda y se para
	// en el primer salto que no es de confianza: el valor inventado se ignora.
	proxies := []string{"130.211.0.0/22", "35.191.0.0/16", ipBalanceador + "/32"}
	ip, _ := clientIPDetras(t, proxies, "130.211.1.1", "1.1.1.1, 203.0.113.7, "+ipBalanceador)
	if ip != "203.0.113.7" {
		t.Fatalf("ClientIP = %q, se esperaba 203.0.113.7 (no la IP inventada 1.1.1.1)", ip)
	}
}

func TestClientIPIgnoraXForwardedForDeOrigenNoConfiable(t *testing.T) {
	// Alguien que llega directo (sin pasar por el balanceador) no puede
	// elegir su IP con la cabecera.
	proxies := []string{"130.211.0.0/22", "35.191.0.0/16", ipBalanceador + "/32"}
	ip, _ := clientIPDetras(t, proxies, "198.51.100.9", "1.1.1.1")
	if ip != "198.51.100.9" {
		t.Fatalf("ClientIP = %q, se esperaba la IP de la conexion 198.51.100.9", ip)
	}
}

func TestRespuestaIdentificaLaInstancia(t *testing.T) {
	_, inst := clientIPDetras(t, nil, "10.0.0.1", "")
	if inst != "mooc-api-x7k2" {
		t.Fatalf("X-Instance-Id = %q, se esperaba mooc-api-x7k2", inst)
	}
}
