# reliable-commerce-core

Extract of a production order spine, stripped to catalog, checkout, inventory reservation, and fulfillment so the two invariants are visible and testable.

This is not client source and not a store. The isolated path is: create an order once, and do not sell a unit twice.

## Scope

1. **Catalog** — a few products, size/finish variants, prices in integer cents plus a currency.
2. **Checkout** — create an order under an idempotency key, reserve inventory, return a stable order id.
3. **Fulfillment** — `PENDING → PAID → PRINTING → SHIPPED`, or `CANCELLED`. Cancel releases the reservation; ship consumes it.

One FastAPI service (Pydantic JSON, OpenAPI at `/docs`). One Vite/React app for catalog, checkout, and status.

## Out of scope

Payments, carts, tax, shipping, auth, search, multi-tenant. Those sit around this spine, not inside it.

## Run

**Prereqs:** Docker, or Python 3.12 + Node 22 + Postgres 16.

```sh
make up          # docker compose: Postgres, API :8080, web :5173
# then open http://localhost:5173
# OpenAPI: http://localhost:8080/docs
```

The API migrates (Alembic) on boot and seeds the catalog when `products` is empty.

Without Compose (Postgres already on `:5432`):

```sh
export DATABASE_URL=postgres://commerce:commerce@127.0.0.1:5432/commerce?sslmode=disable
cd api && python3.12 -m venv .venv && .venv/bin/pip install -e ".[dev]" && .venv/bin/uvicorn app.main:app --reload --port 8080
cd web && npm ci && npm run dev
```

```sh
make test        # pytest against Postgres + web typecheck/lint
```

CI uses a Postgres service and the same commands. Step-by-step: `docs/RUN.md`.

## Layout

```
web (Vite/React)
  └─ REST JSON
       api/app/main.py         routes, CORS, OpenAPI
         app/commerce.py       checkout, reservation, status machine
         app/models.py         SQLAlchemy 2.0
         alembic/versions      SQL migrations
         Postgres              source of truth
```

- HTTP contract: `api/app/schemas.py` and `/docs`
- Domain: `api/app/commerce.py`
- Migration: `api/alembic/versions/001_init.py` (applied on process start)

One API style. The Pydantic models are the contract.

## The two invariants

**Idempotent checkout.** Same key + same payload → the original order. Same key + different payload → conflict. The durable guard is `orders.idempotency_key UNIQUE`. Concurrent first-use of a key is serialized with `pg_advisory_xact_lock` so a retry does not lose a race to `INSUFFICIENT_INVENTORY`. Payload identity is a SHA-256 of email + canonical lines.

**No oversell.** Reservation is one conditional update:

```sql
UPDATE variants
SET reserved = reserved + :qty
WHERE id = :id AND (on_hand - reserved) >= :qty
```

Zero rows means the unit is gone. `CHECK (reserved <= on_hand)` is a second belt. Cancel decrements `reserved`. Ship decrements both `reserved` and `on_hand`.

Both live in `api/app/commerce.py`. Tests: `test_checkout_idempotent_*` and `test_checkout_does_not_oversell_under_race` in `api/tests/test_commerce.py`.

## Tradeoffs

Chosen:

- Integer cents everywhere, including the UI formatter (`web/src/money.ts`). No `amount / 100` through a float.
- FastAPI REST + Pydantic. OpenAPI is generated; there is no second API style.
- Fulfillment verbs are POST actions (`/orders/{id}/paid`, …). There is no payment processor; `paid` is the seam.
- USD only. Mixed-currency carts fail closed.
- Advisory lock + unique index, not a separate idempotency table. The order row *is* the record.
- Sync SQLAlchemy sessions. The invariants are transactional; async would add noise without changing the SQL.

Left out: auth, carts, promotions, tax, shipping, outbox events, multi-warehouse, holds that expire.

MIT licensed.
