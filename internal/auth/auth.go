// Package auth resuelve identidad: contrasenas, tokens de un solo uso y
// sesiones revocables.
//
// DECISION DE ARQUITECTURA: usamos tokens OPACOS (cadenas aleatorias) en vez
// de JWT. Un JWT es autocontenido y por eso no se puede invalidar antes de que
// expire sin montar una lista negra; el enunciado exige "sesiones revocables"
// y "revocacion inmediata", de modo que un token opaco respaldado por una fila
// en Postgres es la opcion correcta y ademas la mas simple de explicar.
//
// En la base guardamos solo el SHA-256 del token. Quien lea la tabla no puede
// suplantar a nadie: el valor original solo lo tiene el cliente.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"mooc-platform/internal/config"
)

var (
	ErrInvalidSession = errors.New("sesion invalida o expirada")
	ErrInvalidToken   = errors.New("token invalido, usado o expirado")
)

// Identity es lo que el resto de la API sabe de quien hace la peticion.
type Identity struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	FullName  string `json:"full_name"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	SessionID string `json:"session_id"`
}

func (i Identity) IsAdmin() bool   { return i.Role == "admin" }
func (i Identity) IsTeacher() bool { return i.Role == "teacher" }

type Service struct {
	db  *sql.DB
	rdb *redis.Client
	cfg config.Config
}

func NewService(db *sql.DB, rdb *redis.Client, cfg config.Config) *Service {
	return &Service{db: db, rdb: rdb, cfg: cfg}
}

// ------------------------- Contrasenas -------------------------

func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("cifrar contrasena: %w", err)
	}
	return string(h), nil
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

// ValidatePasswordPolicy aplica una politica minima explicita. No pretende ser
// exhaustiva; su valor es que el rechazo sea consistente y auditable.
func ValidatePasswordPolicy(p string) error {
	if len(p) < 10 {
		return errors.New("la contrasena debe tener al menos 10 caracteres")
	}
	var hasLetter, hasDigit bool
	for _, r := range p {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("la contrasena debe combinar letras y numeros")
	}
	return nil
}

// ------------------------- Tokens opacos -------------------------

// NewOpaqueToken devuelve el token en claro (para el cliente) y su hash
// (para la base de datos).
func NewOpaqueToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generar aleatorio: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashToken(raw), nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// ------------------------- Sesiones -------------------------

func (s *Service) CreateSession(ctx context.Context, userID, userAgent, ip string) (string, string, error) {
	raw, hash, err := NewOpaqueToken()
	if err != nil {
		return "", "", err
	}
	var sessionID string
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, now() + $5::interval)
		RETURNING id`,
		userID, hash, truncate(userAgent, 300), ip,
		fmt.Sprintf("%d seconds", int(s.cfg.SessionTTL.Seconds())),
	).Scan(&sessionID)
	if err != nil {
		return "", "", fmt.Errorf("crear sesion: %w", err)
	}
	return raw, sessionID, nil
}

// ValidateSession traduce un token en una identidad.
//
// Primero consulta Redis (cache) y solo si falla va a Postgres. Postgres
// sigue siendo la fuente de verdad: si Redis se vacia, el sistema funciona
// igual, apenas un poco mas lento.
func (s *Service) ValidateSession(ctx context.Context, rawToken string) (*Identity, error) {
	hash := HashToken(rawToken)
	cacheKey := "sess:" + hash

	if data, err := s.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
		var id Identity
		if json.Unmarshal(data, &id) == nil {
			return &id, nil
		}
	}

	var id Identity
	err := s.db.QueryRowContext(ctx, `
		SELECT s.id, u.id, u.email, u.full_name, u.role::text, u.status::text
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()`,
		hash,
	).Scan(&id.SessionID, &id.UserID, &id.Email, &id.FullName, &id.Role, &id.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInvalidSession
	}
	if err != nil {
		return nil, fmt.Errorf("validar sesion: %w", err)
	}
	if id.Status != "active" {
		// Cuenta suspendida o sin verificar: la sesion no sirve.
		return nil, ErrInvalidSession
	}

	// Cache corta. La revocacion borra esta llave explicitamente, de modo que
	// sigue siendo inmediata.
	if b, err := json.Marshal(id); err == nil {
		s.rdb.Set(ctx, cacheKey, b, 5*time.Minute)
	}

	// Actualizacion oportunista de last_seen_at (no bloquea la peticion).
	go func(sid string) {
		ctx2, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := s.db.ExecContext(ctx2, `UPDATE sessions SET last_seen_at = now() WHERE id = $1`, sid); err != nil {
			log.Printf("auth: no se pudo actualizar last_seen_at: %v", err)
		}
	}(id.SessionID)

	return &id, nil
}

// RevokeSession invalida una sesion concreta y limpia su cache.
func (s *Service) RevokeSession(ctx context.Context, sessionID string) error {
	var hash string
	err := s.db.QueryRowContext(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE id = $1 AND revoked_at IS NULL
		RETURNING token_hash`, sessionID).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // ya estaba revocada: operacion idempotente
	}
	if err != nil {
		return fmt.Errorf("revocar sesion: %w", err)
	}
	s.rdb.Del(ctx, "sess:"+hash)
	return nil
}

// RevokeAllSessions se usa al suspender una cuenta o al cambiar la contrasena.
func (s *Service) RevokeAllSessions(ctx context.Context, userID string) (int, error) {
	rows, err := s.db.QueryContext(ctx, `
		UPDATE sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL
		RETURNING token_hash`, userID)
	if err != nil {
		return 0, fmt.Errorf("revocar sesiones: %w", err)
	}
	defer rows.Close()

	var keys []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return 0, err
		}
		keys = append(keys, "sess:"+h)
	}
	if len(keys) > 0 {
		s.rdb.Del(ctx, keys...)
	}
	return len(keys), rows.Err()
}

// ------------------------- Tokens de correo -------------------------

// IssueAuthToken crea un token de un solo uso (verificacion o restablecimiento).
func (s *Service) IssueAuthToken(ctx context.Context, userID, purpose string) (string, error) {
	raw, hash, err := NewOpaqueToken()
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO auth_tokens (user_id, token_hash, purpose, expires_at)
		VALUES ($1, $2, $3::token_purpose, now() + $4::interval)`,
		userID, hash, purpose, fmt.Sprintf("%d seconds", int(s.cfg.AuthTokenTTL.Seconds())))
	if err != nil {
		return "", fmt.Errorf("emitir token %s: %w", purpose, err)
	}
	return raw, nil
}

// ConsumeAuthToken valida y marca el token como usado en una sola sentencia.
// Hacerlo atomico evita que dos peticiones simultaneas lo canjeen dos veces.
func (s *Service) ConsumeAuthToken(ctx context.Context, rawToken, purpose string) (string, error) {
	var userID string
	err := s.db.QueryRowContext(ctx, `
		UPDATE auth_tokens SET used_at = now()
		WHERE token_hash = $1
		  AND purpose = $2::token_purpose
		  AND used_at IS NULL
		  AND expires_at > now()
		RETURNING user_id`, HashToken(rawToken), purpose).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidToken
	}
	if err != nil {
		return "", fmt.Errorf("consumir token: %w", err)
	}
	return userID, nil
}

// ------------------------- Bootstrap -------------------------

// EnsureAdmin crea el administrador inicial si no existe. Se ejecuta al
// arrancar la API para que el sistema quede usable con un solo comando,
// sin dejar hashes fijos en el repositorio.
func EnsureAdmin(ctx context.Context, db *sql.DB, cfg config.Config) error {
	hash, err := HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO users (email, password_hash, full_name, role, status, email_verified_at)
		VALUES ($1, $2, 'Administrador del sistema', 'admin', 'active', now())
		ON CONFLICT (email) DO NOTHING`,
		strings.ToLower(cfg.AdminEmail), hash)
	if err != nil {
		return fmt.Errorf("crear admin inicial: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("auth: administrador inicial creado (%s)", cfg.AdminEmail)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
