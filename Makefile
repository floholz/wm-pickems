.PHONY: help install dev-frontend dev-backend build-frontend build run docker clean test tailscale tailscale-off

# Port the local app listens on (matches `make run` / `make dev-backend`).
TS_PORT ?= 8090

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

install: ## Install frontend deps
	cd frontend && npm install

dev-frontend: ## Run SvelteKit dev server (proxies /api to :8090)
	cd frontend && npm run dev

dev-backend: ## Run PocketBase backend on :8090
	go run . serve --http=127.0.0.1:8090 --dir=./pb_data

build-frontend: ## Build the SPA into internal/web/build (cleaned first)
	rm -rf internal/web/build && mkdir -p internal/web/build
	cd frontend && npm run build
	touch internal/web/build/.gitkeep

build: build-frontend ## Build the single binary (frontend embedded)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o wm-pickems .

run: build ## Build then run the single binary
	./wm-pickems serve --http=127.0.0.1:8090 --dir=./pb_data

test: ## Run Go tests
	go test ./...

tailscale: ## Expose the local app over Tailscale HTTPS for mobile dev (needs the app running, e.g. `make run`)
	@command -v tailscale >/dev/null || { echo "tailscale not found — install it first"; exit 1; }
	@# serve needs operator/root. Tries as-is, falls back to sudo. To drop the
	@# sudo prompt for good, run once: sudo tailscale set --operator=$$USER
	tailscale serve --bg $(TS_PORT) || sudo tailscale serve --bg $(TS_PORT)
	@echo
	@echo "Serving 127.0.0.1:$(TS_PORT) over HTTPS on your tailnet. Open this on your phone:"
	@tailscale serve status
	@echo "Stop with: make tailscale-off"

tailscale-off: ## Stop exposing the local app over Tailscale
	tailscale serve reset || sudo tailscale serve reset
	@echo "Tailscale serve cleared."

docker: ## Build the production Docker image
	docker build -t wm-pickems:latest .

reset: ## Wipe the local dev database (pb_data is disposable — re-seeded on boot)
	rm -rf pb_data
	@echo "pb_data removed — next 'make run'/'make dev-backend' re-seeds a fresh DB."

clean: ## Remove build artifacts (keeps the embed .gitkeep so go build works)
	rm -f wm-pickems
	rm -rf frontend/.svelte-kit frontend/build
	find internal/web/build -mindepth 1 ! -name .gitkeep -exec rm -rf {} + 2>/dev/null || true
