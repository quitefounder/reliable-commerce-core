.PHONY: up down test generate seed-hint

up:
	docker compose up --build

down:
	docker compose down

generate:
	cd api && go generate ./...

# Postgres must be reachable at DATABASE_URL (compose postgres or local).
test:
	cd api && go test ./...
	cd web && npm ci && npm run typecheck && npm run lint

seed-hint:
	@echo "API seeds the catalog on first boot when products is empty."
