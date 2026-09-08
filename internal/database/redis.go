package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// OpenRedis devuelve un cliente listo. Redis cumple tres papeles distintos
// en esta arquitectura y conviene tenerlos presentes:
//
//  1. cache de sesiones  -> evita ir a Postgres en cada peticion
//  2. rate limiting      -> contadores por minuto
//  3. cola de trabajos   -> asynq guarda sus colas aqui
//
// Ninguno de los tres es fuente de verdad. Si Redis se pierde, el sistema
// sigue siendo correcto: las sesiones se releen de Postgres y los trabajos
// sin heartbeat se reencolan (ver tasks.ReapStaleWork).
func OpenRedis(addr, password string) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	var lastErr error
	for i := 1; i <= 30; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		lastErr = rdb.Ping(ctx).Err()
		cancel()
		if lastErr == nil {
			log.Printf("database: conectado a redis (intento %d)", i)
			return rdb, nil
		}
		log.Printf("database: redis no responde (intento %d/30): %v", i, lastErr)
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("redis no respondio: %w", lastErr)
}
