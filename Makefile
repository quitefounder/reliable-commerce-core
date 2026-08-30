.PHONY: up down test

up:
	docker compose up --build

down:
	docker compose down

# Postgres must be reachable at DATABASE_URL (compose postgres or local).
test:
	cd api && python3.12 -m venv .venv && .venv/bin/pip install -q -e ".[dev]" && \
		DATABASE_URL=$${DATABASE_URL:-postgres://commerce:commerce@127.0.0.1:5432/commerce_test?sslmode=disable} \
		.venv/bin/pytest
	cd web && npm ci && npm run typecheck && npm run lint
