// Package database abre y verifica las conexiones a los almacenes de estado.
package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // driver de PostgreSQL registrado por su efecto secundario
)

// OpenPostgres espera a que la base este lista. Docker Compose arranca los
// contenedores casi al mismo tiempo, asi que la API puede intentar conectarse
// antes de que Postgres termine de inicializarse; por eso reintentamos.
func OpenPostgres(url string) (*sql.DB, error) {
	db, err := sql.Open("postgres", url)
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
