# reliable-commerce-core

Public sample: a small, reliable commerce order spine (catalog, idempotent checkout, inventory reservation, fulfillment).

This is a hire-ready slice in the stack Dipin actually ships: **FastAPI + Postgres + TypeScript/React**. It is not a Go rewrite of someone else's shop, and it is not a client product. A reviewer should clone it, run it, and understand the two invariants in half an hour.

## What this is

A print/merch shop's order path, isolated:

1. **Catalog** — a few products, size/finish variants, prices in integer cents plus a currency.
2. **Checkout** — create an order under an idempotency key, reserve inventory, return a stable order id.
3. **Fulfillment** — `PENDING → PAID → PRINTING → SHIPPED`, or `CANCELLED`. Cancel releases the reservation; ship consumes it.

One FastAPI service. Typed JSON (Pydantic), one contract, OpenAPI at `/docs`. One React app (Vite) for catalog, checkout, and status.

## What this is not

Not Shopify. Not a payments integration. Not a multi-tenant platform. There is no cart session, tax, shipping, auth, or search. Those are product, not the spine. The interesting work is “the same request does not create two orders” and “two buyers cannot take the last unit.”

It is also not a Sticker Mule clone and not a Go sample. The language choice is honest: this is how the work gets done.

## Run locally

**Prereqs:** Docker, or Python 3.12 + Node 22 + Postgres 16.

```sh
make up          # docker compose: Postgres, API :8080, web :5173
# then open http://localhost:5173
# OpenAPI: http://localhost:8080/docs
```

The API migrates (Alembic) on boot and seeds the catalog when `products` is empty. No extra seed command.

Without Compose (Postgres already on `:5432`):

```sh
export DATABASE_URL=postgres://commerce:commerce@127.0.0.1:5432/commerce?sslmode=disable
cd api && python3.12 -m venv .venv && .venv/bin/pip install -e ".[dev]" && .venv/bin/uvicorn app.main:app --reload --port 8080
cd web && npm ci && npm run dev
```

```sh
make test        # pytest against Postgres + web typecheck/lint
```

CI uses a Postgres service and the same commands.

## Architecture

```
web (Vite/React)
  └─ REST JSON
       api/app/main.py         routes, CORS, OpenAPI
         app/commerce.py       checkout, reservation, status machine
         app/models.py         SQLAlchemy 2.0
         alembic/versions      real SQL migrations
         Postgres              source of truth
```

- HTTP contract: `api/app/schemas.py` and `/docs`
- Domain: `api/app/commerce.py`
- Migration: `api/alembic/versions/001_init.py` (applied on process start)

REST instead of GraphQL: FastAPI's typed models *are* the contract, and a reviewer can read every endpoint in one file. One API style only.

## The two invariants

**Idempotent checkout.** Same key + same payload → the original order. Same key + different payload → conflict. The durable guard is `orders.idempotency_key UNIQUE`. Concurrent first-use of a key is serialized with `pg_advisory_xact_lock` so a retry does not lose a race to `INSUFFICIENT_INVENTORY`. Payload identity is a SHA-256 of email + canonical lines.

**No oversell.** Reservation is one conditional update:

```sql
UPDATE variants
SET reserved = reserved + :qty
WHERE id = :id AND (on_hand - reserved) >= :qty
```

Zero rows means the unit is gone. `CHECK (reserved <= on_hand)` is a second belt. Cancel decrements `reserved`. Ship decrements both `reserved` and `on_hand`.

Both live in `api/app/commerce.py`. Proof: `test_checkout_idempotent_*` and `test_checkout_does_not_oversell_under_race` in `api/tests/test_commerce.py`.

## Tradeoffs

Chosen:

- Integer cents everywhere, including the UI formatter (`web/src/money.ts`). No `amount / 100` through a float.
- FastAPI REST + Pydantic. OpenAPI is generated; there is no second API style.
- Fulfillment verbs are POST actions (`/orders/{id}/paid`, …). There is no payment processor; `paid` is the seam.
- USD only. Mixed-currency carts fail closed.
- Advisory lock + unique index, not a separate idempotency table. The order row *is* the record.
- Sync SQLAlchemy sessions. The invariants are transactional; async would add noise without changing the SQL.

Skipped on purpose:

- Auth, roles, audit log
- Carts, sessions, promotions, tax, shipping
- Outbox / domain events
- Multi-warehouse, backorders, holds that expire
- GraphQL, a second backend, Expo / mobile
- Go

## Where to start

Three files:

1. `api/app/commerce.py` — checkout and the status machine
2. `api/alembic/versions/001_init.py` — the data model the code is not allowed to violate
3. `api/app/schemas.py` — the public contract

Then `docs/REVIEW.md` for a 10-minute click path, and `api/tests/test_commerce.py` for the proofs.

MIT licensed. First commit on `main` was this file's line of intent; everything else lands through a PR.
