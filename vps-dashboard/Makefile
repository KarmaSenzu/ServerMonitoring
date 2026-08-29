# Operational commands untuk monitoring stack.
# Pakai: `make <target>`. Untuk Windows tanpa make, lihat alternatif PowerShell di start.ps1.

SHELL := /bin/bash
COMPOSE := docker compose

.PHONY: help
help: ## Daftar command yang tersedia
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: env
env: ## Buat .env dari template kalau belum ada
	@if [ ! -f .env ]; then cp .env.docker.example .env && echo "✓ .env created. Edit it before continuing."; else echo ".env already exists"; fi

.PHONY: build
build: ## Build semua image
	$(COMPOSE) build --pull

.PHONY: up
up: ## Start stack (detached)
	$(COMPOSE) up -d

.PHONY: down
down: ## Stop stack (preserve volumes)
	$(COMPOSE) down

.PHONY: down-clean
down-clean: ## Stop stack DAN hapus volume (data hilang!)
	$(COMPOSE) down -v

.PHONY: restart
restart: ## Restart semua service
	$(COMPOSE) restart

.PHONY: restart-api
restart-api: ## Restart hanya backend Go
	$(COMPOSE) restart api

.PHONY: restart-web
restart-web: ## Restart hanya nginx
	$(COMPOSE) restart web

.PHONY: logs
logs: ## Tail log semua service
	$(COMPOSE) logs -f --tail=100

.PHONY: logs-api
logs-api: ## Tail log backend
	$(COMPOSE) logs -f --tail=200 api

.PHONY: logs-tunnel
logs-tunnel: ## Tail log cloudflared
	$(COMPOSE) logs -f --tail=200 cloudflared

.PHONY: ps
ps: ## Status container
	$(COMPOSE) ps

.PHONY: shell-api
shell-api: ## Masuk ke shell container api
	$(COMPOSE) exec api sh

.PHONY: shell-web
shell-web: ## Masuk ke shell container web
	$(COMPOSE) exec web sh

.PHONY: backup
backup: ## Snapshot manual SQLite ke ./data/backups
	$(COMPOSE) exec api sh -c 'cp /data/vps-dashboard.db /data/backups/manual-$$(date +%Y%m%d-%H%M%S).db && echo "✓ snapshot saved"'

.PHONY: pull
pull: ## Pull image upstream (cloudflared, dockerproxy)
	$(COMPOSE) pull

.PHONY: update
update: pull build up ## Pull + rebuild + restart

.PHONY: status
status: ## Cek status tunnel + service
	@echo "=== Stack status ==="
	@$(COMPOSE) ps
	@echo ""
	@echo "=== Tunnel logs (last 20) ==="
	@$(COMPOSE) logs --tail=20 cloudflared 2>&1 | tail -20
