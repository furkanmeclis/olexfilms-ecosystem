SHELL := /bin/bash

ENV_FILE ?= $(firstword $(wildcard .env))
PROD_ENV_FILE ?= .env.server

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

FRONTEND_PORT ?= 3000

.PHONY: infra infra-down infra-logs infra-ps migrate-up backend-dev frontend-dev local-dev \
	free-dev-ports prod-config prod-up prod-down prod-create-super-admin tools create-super-admin \
	gen-env-server backup restore dr-drill \
	search-reindex normalize-org-phones dev-pii-key check-i18n check-i18n-translations openapi-sync openapi-lint api-generate

infra:
	@if [ -n "$(ENV_FILE)" ]; then \
		docker compose --env-file "$(ENV_FILE)" -f compose.local.yml up -d; \
	else \
		docker compose -f compose.local.yml up -d; \
	fi

infra-down:
	@if [ -n "$(ENV_FILE)" ]; then \
		docker compose --env-file "$(ENV_FILE)" -f compose.local.yml down; \
	else \
		docker compose -f compose.local.yml down; \
	fi

infra-logs:
	docker compose -f compose.local.yml logs -f

infra-ps:
	docker compose -f compose.local.yml ps

migrate-up:
	@$(load_env) $(MAKE) -C backend migrate-up

backend-dev:
	@$(load_env) $(MAKE) -C backend dev

frontend-dev:
	@$(load_env) \
	cd frontend && \
	if [ ! -d node_modules ]; then pnpm install; fi && \
	pnpm dev

# Host ports bound by air + Next.js. Leaves Docker-mapped infra ports alone.
free-dev-ports:
	@$(load_env) \
	free_listen_port() { \
		local port="$$1"; \
		if ! command -v lsof >/dev/null 2>&1; then \
			echo "lsof not found; skip :$$port"; \
			return 0; \
		fi; \
		local attempt pids pid comm ppid pcomm sig; \
		for attempt in 1 2; do \
			pids=$$(lsof -nP -iTCP:"$$port" -sTCP:LISTEN -t 2>/dev/null | sort -u); \
			if [ -z "$$pids" ]; then \
				if [ "$$attempt" -eq 1 ]; then echo ":$$port is free"; fi; \
				break; \
			fi; \
			for pid in $$pids; do \
				comm=$$(ps -o comm= -p "$$pid" 2>/dev/null | tr -d '[:space:]'); \
				case "$$comm" in \
					*docker*|*com.docker*|dockerd) echo "skipping docker on :$$port (pid $$pid)"; continue ;; \
				esac; \
				ppid=$$(ps -o ppid= -p "$$pid" 2>/dev/null | tr -d '[:space:]'); \
				pcomm=$$(ps -o comm= -p "$$ppid" 2>/dev/null | tr -d '[:space:]'); \
				sig="-TERM"; \
				if [ "$$attempt" -eq 2 ]; then sig="-KILL"; fi; \
				echo "freeing :$$port pid $$pid ($$comm) $$sig"; \
				case "$$pcomm" in \
					*air*|*next*|*node*|*pnpm*|*npm*) kill "$$sig" "$$ppid" 2>/dev/null || true ;; \
				esac; \
				kill "$$sig" "$$pid" 2>/dev/null || true; \
			done; \
			sleep 0.4; \
		done; \
	}; \
	http_addr="$${APP_HTTP_ADDR:-:8080}"; \
	backend_port="$${http_addr##*:}"; \
	frontend_port="$${PORT:-$(FRONTEND_PORT)}"; \
	echo "Checking host ports $$backend_port (api) and $$frontend_port (frontend)..."; \
	free_listen_port "$$backend_port"; \
	free_listen_port "$$frontend_port"

# Infra in Docker; backend (air) + frontend (pnpm) on the host. No app image builds.
local-dev: infra
	@echo "Waiting for postgres..."
	@for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do \
		docker compose -f compose.local.yml exec -T postgres pg_isready -U $${DB_USER:-app} -d $${DB_NAME:-app} >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@$(load_env) $(MAKE) -C backend migrate-up || true
	@$(MAKE) free-dev-ports
	@echo "Starting backend (air) + frontend (pnpm) — Ctrl+C stops both"
	@trap 'kill 0' INT TERM; \
	$(MAKE) backend-dev & \
	$(MAKE) frontend-dev & \
	wait

prod-config:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml config

# make gen-env-server APP_DOMAIN=olexfilms.app RT_DOMAIN=wss.olexfilms.app [ARGS=--rotate]
APP_DOMAIN ?= olexfilms.app
RT_DOMAIN  ?= wss.olexfilms.app
gen-env-server:
	scripts/gen-env-server.sh --app $(APP_DOMAIN) --realtime $(RT_DOMAIN) --out $(PROD_ENV_FILE) $(ARGS)

# Run on the Docker host. NAME_PREFIX defaults to olexfilms.
backup:
	NAME_PREFIX=$${NAME_PREFIX:-olexfilms} scripts/backup.sh

# make restore BACKUP=/srv/backups/20260101-030000  (destructive, asks first)
restore:
	@if [ -z "$(BACKUP)" ]; then echo "usage: make restore BACKUP=<backup directory>"; exit 1; fi
	NAME_PREFIX=$${NAME_PREFIX:-olexfilms} scripts/restore.sh "$(BACKUP)"

dr-drill:
	scripts/dr-drill.sh

prod-up:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml up -d --build

prod-down:
	docker compose --env-file $(PROD_ENV_FILE) -f compose.prod.yml down

# Runs the create-super-admin binary from BACKEND_IMAGE against prod Postgres.
# SA_* come from .env.server; override on the CLI if needed.
prod-create-super-admin:
	@if [ ! -f "$(PROD_ENV_FILE)" ]; then echo "missing $(PROD_ENV_FILE)"; exit 1; fi
	@set -a; \
	while IFS= read -r line || [ -n "$$line" ]; do \
		case "$$line" in \#*|'') continue ;; esac; \
		if [[ "$$line" =~ ^([A-Za-z_][A-Za-z0-9_]*)=(.*)$$ ]]; then \
			export "$${BASH_REMATCH[1]}=$${BASH_REMATCH[2]}"; \
		fi; \
	done < "$(PROD_ENV_FILE)"; \
	set +a; \
	docker compose --env-file "$(PROD_ENV_FILE)" -f compose.prod.yml --profile seed run --rm \
		-e SA_EMAIL="$${SA_EMAIL:-$(SA_EMAIL)}" \
		-e SA_PASSWORD="$${SA_PASSWORD:-$(SA_PASSWORD)}" \
		-e SA_NAME="$${SA_NAME:-$(SA_NAME)}" \
		-e SA_SURNAME="$${SA_SURNAME:-$(SA_SURNAME)}" \
		create-super-admin

tools:
	@echo "docker:  $$(docker version --format '{{.Server.Version}}' 2>&1)"
	@echo "compose: $$(docker compose version 2>&1)"
	@echo "env:     $(ENV_FILE)"
	@$(MAKE) -C backend tools

# Bootstrap platform super_admin (loads root .env). Override: SA_EMAIL SA_PASSWORD SA_NAME SA_SURNAME
SA_EMAIL    ?= admin@example.com
SA_PASSWORD ?= Password1
SA_NAME     ?= Platform
SA_SURNAME  ?= Admin

create-super-admin:
	@$(load_env) $(MAKE) -C backend create-super-admin \
		SA_EMAIL="$(SA_EMAIL)" \
		SA_PASSWORD="$(SA_PASSWORD)" \
		SA_NAME="$(SA_NAME)" \
		SA_SURNAME="$(SA_SURNAME)"

search-reindex:
	@$(load_env) $(MAKE) -C backend search-reindex

# K29 (TEC-159): organization phones to E.164. ARGS=--dry-run to preview.
normalize-org-phones:
	@$(load_env) $(MAKE) -C backend normalize-org-phones ARGS="$(ARGS)"

# Local dev: writes a random CUSTOMER_PII_KEY into .env when it is empty
# (TEC-159). Production keys come from scripts/gen-env-server.sh.
dev-pii-key:
	@test -f .env || cp .env.example .env
	@if grep -qE '^CUSTOMER_PII_KEY=.+' .env; then echo "CUSTOMER_PII_KEY already set in .env"; \
	else key="$$(openssl rand -base64 32)"; \
		if grep -q '^CUSTOMER_PII_KEY=' .env; then sed -i.bak "s|^CUSTOMER_PII_KEY=.*|CUSTOMER_PII_KEY=$$key|" .env && rm -f .env.bak; \
		else printf 'CUSTOMER_PII_KEY=%s\n' "$$key" >> .env; fi; \
		echo "CUSTOMER_PII_KEY written to .env"; fi

check-i18n:
	node scripts/check-i18n.mjs

# Fully translated languages (TEC-138): no missing keys, < 3% copy equal to en.
I18N_TRANSLATED ?= de,fr,es,it,ru,uk
check-i18n-translations:
	node scripts/check-i18n.mjs --strict --locales $(I18N_TRANSLATED)
	node scripts/i18n-untranslated-report.mjs --locales $(I18N_TRANSLATED)

# docs/openapi.yaml is canonical; backend embeds a synced copy (see backend/docs/embed.go).
openapi-sync:
	cp docs/openapi.yaml backend/docs/openapi.yaml

openapi-lint:
	$(MAKE) -C backend openapi-lint

# Regenerate frontend API types from docs/openapi.yaml (and keep the embed in sync).
api-generate: openapi-sync
	cd frontend && pnpm api:generate
