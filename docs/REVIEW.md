# Review path

For a hiring manager or a reviewer who has thirty minutes, not a weekend.

This repo is FastAPI + TypeScript/React + Postgres. That is the production style, not a language chosen to match a job post.

## Clone and run

```sh
git clone https://github.com/quitefounder/reliable-commerce-core.git
cd reliable-commerce-core
make up
```

Open [http://localhost:5173](http://localhost:5173). The catalog is seeded on API boot. Typed contract: [http://localhost:8080/docs](http://localhost:8080/docs).

If Compose is unavailable: Postgres 16, `DATABASE_URL=postgres://commerce:commerce@127.0.0.1:5432/commerce?sslmode=disable`, then `uvicorn` in `api/` and `npm ci && npm run dev` in `web/`.

## What to click

1. Add a **Field Journal · Desk · Wire** (2 on hand — the scarce SKU).
2. Place the order. Note the order id and the idempotency key.
3. Click **Place order** again. Same id. Available count does not drop a second time.
4. Change the email, keep the key, place again. You should see `IDEMPOTENCY_CONFLICT`.
5. **Mark paid → Start print → Ship**, or **Cancel**. Cancel puts availability back. Ship does not.

## What the tests prove

```sh
make test
```

| Test | Claim |
| --- | --- |
| `test_checkout_idempotent_same_payload` | Replay returns the same order id; reserved stays 1 |
| `test_checkout_idempotent_conflict` | Same key, different email → conflict; reserved stays 1 |
| `test_checkout_does_not_oversell_under_race` | 20 concurrent checkouts, 1 unit on hand → exactly 1 success |
| `test_cancel_releases_reservation` | Cancel returns reserved to 0 |
| `test_ship_consumes_inventory` | Ship moves reserved into on-hand |
| `test_money_stays_integer_cents` | 3 × 500 = 1500, currency `USD` |

These hit a real Postgres. They are the reason this repo exists.

## What to read

`api/app/commerce.py`, `api/alembic/versions/001_init.py`, `api/app/schemas.py`.

This sample is original. It is not a client codebase and it is not a Sticker Mule clone.
