// Package database abre y verifica las conexiones a los almacenes de estado.
package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq" // driver de PostgreSQL registrado por su efecto secundario
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// OpenPostgres espera a que la base este lista. Docker Compose arranca los
// contenedores casi al mismo tiempo, asi que la API puede intentar conectarse
// antes de que Postgres termine de inicializarse; por eso reintentamos.
func OpenPostgres(url string) (*sql.DB, error) {
	// otelsql envuelve el driver "postgres": cada consulta que reciba un context
	// con un span activo (el de la peticion HTTP o el del trabajo del worker)
	// abre un span hijo con la sentencia SQL. Si las trazas estan apagadas, el
	// coste es nulo. No cambia el API de database/sql: sigue siendo un *sql.DB.
	db, err := otelsql.Open("postgres", url,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
		otelsql.WithSpanOptions(otelsql.SpanOptions{OmitConnResetSession: true, OmitRows: true}),
	)
	if err != nil {
		return nil, fmt.Errorf("abrir postgres: %w", err)
	}

	// Limites del pool. Sin esto, cada peticion concurrente puede abrir una
	// conexion nueva y agotar max_connections del servidor.
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)

	var lastErr error
	for i := 1; i <= 30; i++ {
		if lastErr = db.Ping(); lastErr == nil {
			log.Printf("database: conectado a postgres (intento %d)", i)
			return db, nil
		}
		log.Printf("database: postgres no responde (intento %d/30): %v", i, lastErr)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("postgres no respondio: %w", lastErr)
}
