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
	"time"
)

type Config struct {
	// --- HTTP ---
	AppEnv        string // dev | prod
	HTTPAddr      string // ":8080"
	PublicBaseURL string // se usa para construir enlaces de correo e insignias

	// --- Postgres ---
	DatabaseURL string

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

	// --- Antimalware ---
	// stub  = detecta la firma de prueba EICAR (suficiente para demostrar)
	// clamav= delega en un demonio ClamAV (no se despliega en esta entrega)
	AntimalwareMode string
	ClamAVAddr      string

	// --- Bootstrap del administrador ---
	AdminEmail    string
	AdminPassword string
}

func Load() Config {
	cfg := Config{
		AppEnv:        env("APP_ENV", "dev"),
		HTTPAddr:      env("HTTP_ADDR", ":8080"),
		PublicBaseURL: env("PUBLIC_BASE_URL", "http://localhost:8080"),

		DatabaseURL: env("DATABASE_URL", "postgres://mooc:mooc@postgres:5432/mooc?sslmode=disable"),

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

		AntimalwareMode: env("ANTIMALWARE_MODE", "stub"),
		ClamAVAddr:      env("CLAMAV_ADDR", ""),

		AdminEmail:    env("ADMIN_EMAIL", "admin@mooc.local"),
		AdminPassword: env("ADMIN_PASSWORD", "Admin123!"),
	}
	return cfg
}

// Describe se imprime al arrancar. Nunca incluye secretos.
func (c Config) Describe() string {
	return fmt.Sprintf(
		"env=%s addr=%s db=ok redis=%s bucket=%s cdn=%q antimalware=%s",
		c.AppEnv, c.HTTPAddr, c.RedisAddr, c.S3Bucket, c.CDNBaseURL, c.AntimalwareMode,
	)
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
