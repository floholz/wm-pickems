.PHONY: help install dev-frontend dev-backend build-frontend build run docker clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

install: ## Install frontend deps
	cd frontend && npm install

dev-frontend: ## Run SvelteKit dev server (proxies /api to :8090)
	cd frontend && npm run dev

dev-backend: ## Run PocketBase backend on :8090
	go run . serve --http=127.0.0.1:8090 --dir=./pb_data

build-frontend: ## Build the SPA into internal/web/build
	cd frontend && npm run build

build: build-frontend ## Build the single binary (frontend embedded)
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o wm-pickems .

run: build ## Build then run the single binary
	./wm-pickems serve --http=127.0.0.1:8090 --dir=./pb_data

docker: ## Build the production Docker image
	docker build -t wm-pickems:latest .

clean: ## Remove build artifacts
	rm -f wm-pickems
	rm -rf frontend/.svelte-kit frontend/build
	git checkout -- internal/web/build/index.html 2>/dev/null || true
