# =====================================================================
# Atajos del proyecto. Ejecutalos desde la raiz, en la terminal de WSL2.
# =====================================================================

.PHONY: help up down logs ps rebuild deps fmt vet scale-workers reset db psql redis-cli test-smoke

help:
	@echo "Comandos disponibles:"
	@echo "  make deps          Resuelve go.sum (go mod tidy en un contenedor Go)"
	@echo "  make up            Levanta toda la plataforma"
	@echo "  make down          Detiene los contenedores (conserva los datos)"
	@echo "  make reset         Detiene y BORRA los volumenes (datos incluidos)"
	@echo "  make rebuild       Reconstruye las imagenes y levanta"
	@echo "  make logs          Sigue los logs de api y worker"
	@echo "  make ps            Estado de los servicios"
	@echo "  make scale-workers Levanta 3 instancias del worker"
	@echo "  make psql          Abre una consola SQL en la base de datos"
	@echo "  make fmt           Formatea el codigo Go"
	@echo "  make test-smoke    Prueba de humo del flujo principal"

# go.sum se genera a partir del codigo. Se hace en un contenedor efimero para
# no depender de la version de Go instalada en la maquina.
deps:
	docker run --rm -v "$(PWD)":/src -w /src golang:1.24-alpine \
		sh -c "apk add --no-cache git >/dev/null && go mod tidy"

up:
	docker compose up --build

down:
	docker compose down

# Ojo: borra pgdata, miniodata y redisdata. Es lo que hay que hacer si
# modificaste db/init.sql, porque ese script solo corre al crear el volumen.
reset:
	docker compose down -v

rebuild:
	docker compose build --no-cache
	docker compose up

logs:
	docker compose logs -f api worker

ps:
	docker compose ps

scale-workers:
	docker compose up -d --scale worker=3

psql:
	docker compose exec postgres psql -U mooc -d mooc

redis-cli:
	docker compose exec redis redis-cli

fmt:
	docker run --rm -v "$(PWD)":/src -w /src golang:1.24-alpine gofmt -l -w .

vet:
	docker run --rm -v "$(PWD)":/src -w /src golang:1.24-alpine go vet ./...

test-smoke:
	./testdata/smoke.sh
