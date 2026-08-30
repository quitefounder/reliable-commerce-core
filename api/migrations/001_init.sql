-- Commerce core: catalog, inventory, orders.
-- Money is integer cents. Inventory available = on_hand - reserved.

CREATE TABLE products (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE variants (
    id                 UUID PRIMARY KEY,
    product_id         UUID NOT NULL REFERENCES products (id),
    sku                TEXT NOT NULL UNIQUE,
    size               TEXT NOT NULL,
    finish             TEXT NOT NULL,
    unit_amount_cents  INT  NOT NULL CHECK (unit_amount_cents >= 0),
    currency           TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    on_hand            INT  NOT NULL CHECK (on_hand >= 0),
    reserved           INT  NOT NULL CHECK (reserved >= 0),
    CHECK (reserved <= on_hand)
);

CREATE TABLE orders (
    id                 UUID PRIMARY KEY,
    idempotency_key    TEXT NOT NULL UNIQUE,
    payload_hash       TEXT NOT NULL,
    email              TEXT NOT NULL,
    status             TEXT NOT NULL CHECK (
        status IN ('PENDING', 'PAID', 'PRINTING', 'SHIPPED', 'CANCELLED')
    ),
    total_amount_cents INT  NOT NULL CHECK (total_amount_cents >= 0),
    currency           TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE order_lines (
    id                UUID PRIMARY KEY,
    order_id          UUID NOT NULL REFERENCES orders (id),
    variant_id        UUID NOT NULL REFERENCES variants (id),
    quantity          INT  NOT NULL CHECK (quantity > 0),
    unit_amount_cents INT  NOT NULL CHECK (unit_amount_cents >= 0),
    currency          TEXT NOT NULL CHECK (currency ~ '^[A-Z]{3}$')
);

CREATE INDEX order_lines_order_id_idx ON order_lines (order_id);
CREATE INDEX orders_created_at_idx ON orders (created_at DESC);
