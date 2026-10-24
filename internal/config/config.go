// Package config concentra TODA la configuracion del sistema en un solo
// lugar. Ninguna otra parte del codigo lee variables de entorno: si algo
// necesita un valor configurable, se agrega aqui.
//
// Esto cumple la regla "separacion entre configuracion, codigo y estado":
// la misma imagen de Docker funciona en local y en la nube cambiando solo
// el entorno, sin recompilar.
package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// --- HTTP ---
	AppEnv        string // dev | prod
	HTTPAddr      string // ":8080"
	PublicBaseURL string // se usa para construir enlaces de correo e insignias

	// --- Postgres ---
	DatabaseURL string
	// Tope de conexiones POR PROCESO. Con replicas, el total es la suma de
	// todas las instancias de API y workers y debe caber en max_connections
	// de Cloud SQL (ver deploy/gcp/mig/*/*.env.example).
	DBMaxOpenConns int
	DBMaxIdleConns int

	// --- Redis (sesiones en cache, rate limit y cola asynq) ---
	RedisAddr     string
	RedisPassword string

	// --- Almacenamiento de objetos (MinIO en local, GCS/S3 en la nube) ---
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3Region    string
	S3UseSSL    bool
	// URL publica del bucket vista desde el navegador. En Docker el endpoint
	// interno es "minio:9000" pero el navegador del anfitrion solo alcanza
	// "localhost:9000": las URLs prefirmadas se reescriben con este valor.
	S3PublicEndpoint string

	// CDN: si esta vacio, los binarios se entregan con URL prefirmada.
	// En la nube (Entrega 2) apuntara a Cloud CDN.
	CDNBaseURL string

	// --- Correo (Mailpit en local) ---
	SMTPAddr string
	SMTPFrom string

	// --- Politicas ---
	SessionTTL      time.Duration // vigencia de una sesion
	AuthTokenTTL    time.Duration // vigencia de los enlaces de correo
	UploadTTL       time.Duration // ventana para reanudar una carga (24 h)
	SignedURLTTL    time.Duration // vigencia de una URL prefirmada de descarga
	RateLimitPerMin int           // peticiones por minuto y por identidad
	MaxUploadBytes  int64         // tamano maximo declarado por archivo

	// --- Workers ---
	WorkerConcurrency int
	MaxRetries        int // reintentos antes de la DLQ
	// Espera por los trabajos activos al apagar. Lo que no termine en ese
	// tiempo asynq lo devuelve a la cola y otro worker lo reintenta.
	WorkerShutdownTimeout time.Duration

	// --- Replicas detras del balanceador (Entrega 3) ---
	// TrustedProxies: rangos de los proxies cuyo X-Forwarded-For se cree para
	// obtener la IP del cliente (limite de tasa, auditoria). Vacio = el
	// comportamiento por defecto de gin, que confia en cualquiera.
	TrustedProxies []string
	// InstanceID identifica la replica en la cabecera X-Instance-Id y en los
	// logs: es la evidencia de a que instancia llego cada peticion.
	InstanceID string

	// --- Metrica de la cola (cmd/queue-metrics, autoescalado de workers) ---
	// QueueMetricsProject vacio = se toma del servidor de metadatos de GCE;
	// fuera de GCE el proceso solo registra la medicion en el log.
	QueueMetricsInterval time.Duration
	QueueMetricsProject  string

	// --- Antimalware ---
	// stub  = detecta la firma de prueba EICAR (suficiente para demostrar)
	// clamav= delega en un demonio ClamAV (no se despliega en esta entrega)
	AntimalwareMode string
	ClamAVAddr      string

	// --- Bootstrap del administrador ---
	AdminEmail    string
	AdminPassword string

	// --- Observabilidad (OpenTelemetry) ---
	// OTelEndpoint: host:puerto del colector OTLP/HTTP (Jaeger en local).
	// Vacio = trazas apagadas. OTelSampleRatio: fraccion muestreada (0..1).
	OTelEndpoint    string
	OTelSampleRatio float64
}

func Load() Config {
	cfg := Config{
		AppEnv:        env("APP_ENV", "dev"),
		HTTPAddr:      env("HTTP_ADDR", ":8080"),
		PublicBaseURL: env("PUBLIC_BASE_URL", "http://localhost:8080"),

		DatabaseURL:    env("DATABASE_URL", "postgres://mooc:mooc@postgres:5432/mooc?sslmode=disable"),
		DBMaxOpenConns: envInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: envInt("DB_MAX_IDLE_CONNS", 10),

		RedisAddr:     env("REDIS_ADDR", "redis:6379"),
		RedisPassword: env("REDIS_PASSWORD", ""),

		S3Endpoint:       env("S3_ENDPOINT", "minio:9000"),
		S3AccessKey:      env("S3_ACCESS_KEY", "minioadmin"),
		S3SecretKey:      env("S3_SECRET_KEY", "minioadmin"),
		S3Bucket:         env("S3_BUCKET", "mooc"),
		S3Region:         env("S3_REGION", "us-east-1"),
		S3UseSSL:         envBool("S3_USE_SSL", false),
		S3PublicEndpoint: env("S3_PUBLIC_ENDPOINT", "localhost:9000"),

		CDNBaseURL: env("CDN_BASE_URL", ""),

		SMTPAddr: env("SMTP_ADDR", "mailpit:1025"),
		SMTPFrom: env("SMTP_FROM", "no-reply@mooc.local"),

		SessionTTL:      time.Duration(envInt("SESSION_TTL_HOURS", 24)) * time.Hour,
		AuthTokenTTL:    time.Duration(envInt("AUTH_TOKEN_TTL_HOURS", 24)) * time.Hour,
		UploadTTL:       time.Duration(envInt("UPLOAD_TTL_HOURS", 24)) * time.Hour,
		SignedURLTTL:    time.Duration(envInt("SIGNED_URL_TTL_MINUTES", 15)) * time.Minute,
		RateLimitPerMin: envInt("RATE_LIMIT_PER_MIN", 120),
		MaxUploadBytes:  int64(envInt("MAX_UPLOAD_MB", 2048)) * 1024 * 1024,

		WorkerConcurrency: envInt("WORKER_CONCURRENCY", 4),
		MaxRetries:        envInt("MAX_RETRIES", 3),
		// 8 s es el valor por defecto de asynq: asi local y la Entrega 2 no
		// cambian. En los grupos de la Entrega 3 se sube (worker.env).
		WorkerShutdownTimeout: time.Duration(envInt("WORKER_SHUTDOWN_TIMEOUT_SECONDS", 8)) * time.Second,

		TrustedProxies: envList("TRUSTED_PROXIES"),
		InstanceID:     env("INSTANCE_ID", hostname()),

		// 30 s: el autoescalador agrega por minuto, y Cloud Monitoring no
		// admite puntos de una misma serie a menos de 5 s.
		QueueMetricsInterval: time.Duration(envInt("QUEUE_METRICS_INTERVAL_SECONDS", 30)) * time.Second,
		QueueMetricsProject:  env("QUEUE_METRICS_PROJECT", ""),

		AntimalwareMode: env("ANTIMALWARE_MODE", "stub"),
		ClamAVAddr:      env("CLAMAV_ADDR", ""),

		AdminEmail:    env("ADMIN_EMAIL", "admin@mooc.local"),
		AdminPassword: env("ADMIN_PASSWORD", "Admin123!"),

		OTelEndpoint:    env("OTEL_EXPORTER_OTLP_ENDPOINT", ""),
		OTelSampleRatio: envFloat("OTEL_SAMPLE_RATIO", 1.0),
	}
	return cfg
}

// Describe se imprime al arrancar. Nunca incluye secretos.
func (c Config) Describe() string {
	return fmt.Sprintf(
		"env=%s instancia=%s addr=%s db_pool=%d redis=%s bucket=%s cdn=%q antimalware=%s",
		c.AppEnv, c.InstanceID, c.HTTPAddr, c.DBMaxOpenConns, c.RedisAddr, c.S3Bucket, c.CDNBaseURL, c.AntimalwareMode,
	)
}

// envList lee una lista separada por comas, ignorando espacios y vacios.
func envList(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// hostname es el valor por defecto de INSTANCE_ID. En Docker es el id del
// contenedor; en la nube startup.sh pasa el nombre de la VM.
func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "desconocida"
	}
	return h
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("config: %s='%s' no es un entero, se usa %d", key, v, def)
		return def
	}
	return n
}

func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		log.Printf("config: %s='%s' no es un numero, se usa %g", key, v, def)
		return def
	}
	return f
}

func envBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
