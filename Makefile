.PHONY: tidy db run migrate test up down
DB_URL ?= postgres://cinebot:cinebot@localhost:5440/cinebot?sslmode=disable
TEST_DB_URL ?= postgres://cinebot:cinebot@localhost:5440/cinebot_test?sslmode=disable

tidy:      ## genera go.sum (ejecutar una vez y commitear)
	go mod tidy

db:        ## Postgres local en Docker (+ BD de test)
	docker compose up -d db
	@until docker compose exec -T db pg_isready -U cinebot >/dev/null 2>&1; do sleep 1; done
	-docker compose exec -T db psql -U cinebot -c 'CREATE DATABASE cinebot_test;'

migrate:
	DATABASE_URL="$(DB_URL)" go run ./cmd/api -migrate-only

run:       ## API local; carga .env si existe (si no, DB local y API_KEY=dev-key)
	@set -a; [ -f .env ] && . ./.env; set +a; \
	DATABASE_URL="$${DATABASE_URL:-$(DB_URL)}" API_KEY="$${API_KEY:-dev-key}" go run ./cmd/api

test:      ## tests de integración contra Postgres (-p 1: comparten la BD de test)
	TEST_DATABASE_URL="$(TEST_DB_URL)" go test -race -count=1 -p 1 ./...

up:        ## todo en contenedores
	docker compose up --build
down:
	docker compose down
