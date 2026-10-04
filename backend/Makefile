APP_NAME := api
MODULE   := github.com/furkanmeclis/olexfilms-ecosystem/backend
MIGRATE  := migrate
SHELL    := /bin/bash

# Repo-root .env is canonical; backend/.env is fallback only.
ENV_FILE ?= $(firstword $(wildcard ../.env .env))

DB_USER     ?= app
DB_PASSWORD ?= change_me
DB_HOST     ?= 127.0.0.1
DB_PORT     ?= 5432
DB_NAME     ?= app

define load_env
if [ -f "$(ENV_FILE)" ]; then \
	set -a; \
	while IFS= read -r line || [ -n "$$line" ]; do \
		case "$$line" in \#*|'') continue ;; esac; \
		if [[ "$$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$$ ]]; then \
			export "$${BASH_REMATCH[1]}=$${BASH_REMATCH[2]}"; \
		fi; \
	done < "$(ENV_FILE)"; \
	set +a; \
fi;
endef

DB_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable

.PHONY: tools run dev worker ping-job search-reindex inventory-rebuild roles-sync normalize-org-phones build test lint fmt tidy sqlc migrate-up migrate-down migrate-create health create-super-admin openapi-lint

tools:
	@echo "go:            $$(go version)"
	@echo "migrate:       $$(migrate -version 2>&1)"
	@echo "sqlc:          $$(sqlc version 2>&1)"
	@echo "golangci-lint: $$(golangci-lint version 2>&1 | head -1)"
	@echo "air:           $$(air -v 2>&1)"
	@echo "env file:      $(ENV_FILE)"

run:
	@$(load_env) go run ./cmd/server

dev:
	@$(load_env) air -c .air.toml

worker:
	@$(load_env) go run ./cmd/worker

ping-job:
	@$(load_env) go run ./cmd/enqueue-ping

search-reindex:
	@$(load_env) go run ./cmd/search-reindex

# TEC-156: replay stock_movements and report projection drift (dry run).
# ARGS="-org 5 -json"; ARGS=-apply repairs in one locked transaction.
inventory-rebuild:
	@$(load_env) go run ./cmd/inventory-rebuild $(ARGS)

# TEC-272: read-only Glorian drift report (exit 3 on drift).
# ARGS="--connection=glorian --limit=20 --json".
.PHONY: inventory-reconcile
inventory-reconcile:
	@$(load_env) go run ./cmd/inventory-reconcile $(ARGS)

# Reconcile permissions and system role packages with the Go catalog.
roles-sync:
	@$(load_env) go run ./cmd/roles-sync $(ARGS)

# K29 (TEC-159): parse organization phones moved aside by migration 000048
# into E.164. ARGS=--dry-run reports without writing.
normalize-org-phones:
	@$(load_env) go run ./cmd/normalize-org-phones $(ARGS)

# TEC-294: normalize measurement_results rows with parsed_at NULL (NexPTG
# parser backfill, migrator rows included). Idempotent.
.PHONY: measurement-reparse
measurement-reparse:
	@$(load_env) go run ./cmd/measurement-reparse $(ARGS)

build:
	go build -o bin/$(APP_NAME) ./cmd/server
	go build -o bin/$(APP_NAME)-worker ./cmd/worker

test:
	go test ./...

lint:
	golangci-lint run ./...

fmt:
	gofumpt -w .

tidy:
	go mod tidy

sqlc:
	sqlc generate

db-create:
	@$(load_env) \
	PGPASSWORD="$${DB_PASSWORD:-$(DB_PASSWORD)}" psql -h "$${DB_HOST:-$(DB_HOST)}" -p "$${DB_PORT:-$(DB_PORT)}" -U "$${DB_USER:-$(DB_USER)}" -d postgres -v ON_ERROR_STOP=1 -tAc "SELECT 1 FROM pg_database WHERE datname='$${DB_NAME:-$(DB_NAME)}'" | grep -q 1 || \
	PGPASSWORD="$${DB_PASSWORD:-$(DB_PASSWORD)}" psql -h "$${DB_HOST:-$(DB_HOST)}" -p "$${DB_PORT:-$(DB_PORT)}" -U "$${DB_USER:-$(DB_USER)}" -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE $${DB_NAME:-$(DB_NAME)}"

migrate-up: db-create
	@$(load_env) $(MIGRATE) -path migrations -database "postgres://$${DB_USER:-$(DB_USER)}:$${DB_PASSWORD:-$(DB_PASSWORD)}@$${DB_HOST:-$(DB_HOST)}:$${DB_PORT:-$(DB_PORT)}/$${DB_NAME:-$(DB_NAME)}?sslmode=$${DB_SSLMODE:-disable}" up

migrate-down:
	@$(load_env) $(MIGRATE) -path migrations -database "postgres://$${DB_USER:-$(DB_USER)}:$${DB_PASSWORD:-$(DB_PASSWORD)}@$${DB_HOST:-$(DB_HOST)}:$${DB_PORT:-$(DB_PORT)}/$${DB_NAME:-$(DB_NAME)}?sslmode=$${DB_SSLMODE:-disable}" down 1

migrate-create:
	@test -n "$(NAME)" || (echo "Usage: make migrate-create NAME=create_users"; exit 1)
	$(MIGRATE) create -ext sql -dir migrations -seq $(NAME)

health:
	curl -sS http://127.0.0.1:8080/healthz && echo && curl -sS http://127.0.0.1:8080/readyz && echo

SA_EMAIL    ?= admin@example.com
SA_PASSWORD ?= Password1
SA_NAME     ?= Platform
SA_SURNAME  ?= Admin

create-super-admin:
	@$(load_env) go run ./cmd/create-super-admin \
		-email "$(SA_EMAIL)" \
		-password "$(SA_PASSWORD)" \
		-name "$(SA_NAME)" \
		-surname "$(SA_SURNAME)"

openapi-lint:
	npx --yes @redocly/cli@1 lint ../docs/openapi.yaml
