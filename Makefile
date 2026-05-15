.PHONY: help install dev-frontend dev-backend build-frontend build run docker clean test

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

install: ## Install frontend deps
	cd frontend && npm install

dev-frontend: ## Run SvelteKit dev server (proxies /api to :8090)
	cd frontend && npm run dev

dev-backend: ## Run PocketBase backend on :8090
	go run . serve --http=127.0.0.1:8090 --dir=./pb_data

build-frontend: ## Build the SPA into internal/web/build (cleaned first)
	rm -rf internal/web/build && mkdir -p internal/web/build && touch internal/web/build/.gitkeep
	cd frontend && npm run build

build: build-frontend ## Build the single binary (frontend embedded)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o wm-pickems .

run: build ## Build then run the single binary
	./wm-pickems serve --http=127.0.0.1:8090 --dir=./pb_data

test: ## Run Go tests
	go test ./...

docker: ## Build the production Docker image
	docker build -t wm-pickems:latest .

reset: ## Wipe the local dev database (pb_data is disposable — re-seeded on boot)
	rm -rf pb_data
	@echo "pb_data removed — next 'make run'/'make dev-backend' re-seeds a fresh DB."

clean: ## Remove build artifacts (keeps the embed .gitkeep so go build works)
	rm -f wm-pickems
	rm -rf frontend/.svelte-kit frontend/build
	find internal/web/build -mindepth 1 ! -name .gitkeep -exec rm -rf {} + 2>/dev/null || true
