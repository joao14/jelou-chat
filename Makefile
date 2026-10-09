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

JELOU ?= jelou --profile alex-merino

jelou-create: ## crea en Jelou las tools nuevas (una sola vez)
	@for n in create_reservation create_payment get_reservation cancel_reservation; do $(JELOU) tool create --name "$$n" || true; done
	$(JELOU) tool pull --all

jelou-gen:    ## regenera tools/*.json y el workflow desde scripts/gen_jelou.py
	python3 scripts/gen_jelou.py

jelou-validate: ## valida workflow y tools localmente
	$(JELOU) workflow validate --allow-warnings

jelou-push:   ## sube tools y workflow a Jelou
	$(JELOU) tool push --all --yes
	$(JELOU) push

jelou-publish: ## publica las tools (sin pasar --yes) y guarda la última versión de cada una
	@for s in listar-peliculas listar-funciones ver-asientos create-reservation create-payment get-reservation cancel-reservation; do $(JELOU) tool publish $$s || true; done
	python3 scripts/jelou_versions.py

jelou-all:    ## ciclo completo: genera, sube tools, publica, fija versiones, sube workflow
	$(MAKE) jelou-gen
	$(JELOU) tool push --all --yes
	$(MAKE) jelou-publish
	$(MAKE) jelou-gen
	$(JELOU) push

jelou-test:   ## casos de éxito y error contra las tools
	bash scripts/jelou_test_cases.sh

test:      ## tests de integración contra Postgres (-p 1: comparten la BD de test)
	TEST_DATABASE_URL="$(TEST_DB_URL)" go test -race -count=1 -p 1 ./...

up:        ## todo en contenedores
	docker compose up --build
down:
	docker compose down
