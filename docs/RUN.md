# Run

## Local

```sh
git clone https://github.com/quitefounder/reliable-commerce-core.git
cd reliable-commerce-core
make up
```

UI: [http://localhost:5173](http://localhost:5173). The catalog is seeded on API boot. OpenAPI: [http://localhost:8080/docs](http://localhost:8080/docs).

Without Compose: Postgres 16, `DATABASE_URL=postgres://commerce:commerce@127.0.0.1:5432/commerce?sslmode=disable`, then `uvicorn` in `api/` and `npm ci && npm run dev` in `web/`.

## Exercise the spine

1. Add a **Field Journal · Desk · Wire** (2 on hand — the scarce SKU).
2. Place the order. Note the order id and the idempotency key.
3. Click **Place order** again. Same id. Available count does not drop a second time.
4. Change the email, keep the key, place again. You should see `IDEMPOTENCY_CONFLICT`.
5. **Mark paid → Start print → Ship**, or **Cancel**. Cancel puts availability back. Ship does not.

## Tests

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

These run against Postgres.

The reservation and status machine are in `api/app/commerce.py`. The schema is `api/alembic/versions/001_init.py`. The HTTP contract is `api/app/schemas.py`.
