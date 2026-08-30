# reliable-commerce-core

Public sample: a small, reliable commerce order spine (catalog, idempotent checkout, inventory reservation, fulfillment).

This is a hire-ready slice, not a store platform. A reviewer should be able to clone it, run it, and understand the two invariants in half an hour.

## What this is

A print/merch shop's order path, isolated:

1. **Catalog** — a few products, size/finish variants, prices in integer cents plus a currency.
2. **Checkout** — create an order under an idempotency key, reserve inventory, return a stable order id.
3. **Fulfillment** — `PENDING → PAID → PRINTING → SHIPPED`, or `CANCELLED`. Cancel releases the reservation; ship consumes it.

One GraphQL API (Go + gqlgen + Postgres). One React app (Vite) for catalog, checkout, and status.

## What this is not

Not Shopify. Not a payments integration. Not a multi-tenant platform. There is no cart session, tax, shipping, auth, or search. Those are product, not the spine. The interesting work is “the same request does not create two orders” and “two buyers cannot take the last unit.”

## Run locally

**Prereqs:** Docker, or Go 1.23 + Node 22 + Postgres 16.

```sh
make up          # docker compose: Postgres, API :8080, web :5173
# then open http://localhost:5173
# GraphQL playground: http://localhost:8080/playground
```

The API migrates on boot and seeds the catalog when `products` is empty. No extra seed command.

Without Compose (Postgres already on `:5432`):

```sh
export DATABASE_URL=postgres://commerce:commerce@127.0.0.1:5432/commerce?sslmode=disable
cd api && go generate ./... && go run ./cmd/server
cd web && npm ci && npm run dev
```

```sh
make test        # go generate + Go tests (needs Postgres) + web typecheck/lint
```

CI uses a Postgres service and the same commands.

## Architecture

```
web (Vite/React)
  └─ POST /graphql
       api/cmd/server          HTTP, CORS, playground
         graph/                schema + thin resolvers
         internal/commerce     checkout, reservation, status machine
         internal/migrate      numbered SQL, applied once
         Postgres              source of truth
```

- Schema: `api/graph/schema.graphqls`
- Domain: `api/internal/commerce`
- Migrations: `api/migrations/*.sql` (embedded, run on process start)
- gqlgen's `generated.go` is committed and marked `DO NOT EDIT`. It is a dump. See `api/graph/GENERATED.md`.

## The two invariants

**Idempotent checkout.** Same key + same payload → the original order. Same key + different payload → conflict. The durable guard is `orders.idempotency_key UNIQUE`. Concurrent first-use of a key is serialized with `pg_advisory_xact_lock` so a retry does not lose a race to `INSUFFICIENT_INVENTORY`. Payload identity is a SHA-256 of email + canonical lines (`payloadHash` in `hash.go`).

**No oversell.** Reservation is one conditional update:

```sql
UPDATE variants
SET reserved = reserved + $qty
WHERE id = $id AND (on_hand - reserved) >= $qty
```

Zero rows means the unit is gone. `CHECK (reserved <= on_hand)` is a second belt. Cancel decrements `reserved`. Ship decrements both `reserved` and `on_hand`.

Both live in `api/internal/commerce/store.go`. Proof: `TestCheckoutIdempotent*` and `TestCheckoutDoesNotOversellUnderRace` in `commerce_test.go`.

## Tradeoffs

Chosen:

- Integer cents everywhere, including the UI formatter (`web/src/money.ts`). No `amount / 100` through a float.
- One GraphQL surface. No REST alongside it.
- Fulfillment verbs are mutations (`markPaid`, `startPrint`, …). There is no payment processor; `markPaid` is the seam.
- USD only. Mixed-currency carts fail closed.
- Advisory lock + unique index, not a separate idempotency table. The order row *is* the record.

Skipped on purpose:

- Auth, roles, audit log
- Carts, sessions, promotions, tax, shipping
- Outbox / domain events
- Multi-warehouse, backorders, holds that expire
- Expo / mobile
- Reviewing gqlgen dumps (they are marked generated; start at the schema)

## Where to start

Three files:

1. `api/internal/commerce/store.go` — checkout and the status machine
2. `api/migrations/001_init.sql` — the data model the code is not allowed to violate
3. `api/graph/schema.graphqls` — the public contract

Then `docs/REVIEW.md` for a 10-minute click path, and `api/internal/commerce/commerce_test.go` for the proofs.

MIT licensed. First commit on `main` was this file's line of intent; everything else lands through a PR.
