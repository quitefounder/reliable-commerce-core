"""Checkout and fulfillment. Postgres enforces the two invariants."""

from __future__ import annotations

import hashlib
import uuid
from collections import defaultdict

from sqlalchemy import select, text, update
from sqlalchemy.exc import IntegrityError
from sqlalchemy.orm import Session, selectinload

from app.errors import (
    CurrencyMismatch,
    IdempotencyConflict,
    InsufficientInventory,
    InvalidInput,
    InvalidTransition,
    NotFound,
)
from app.models import ALLOWED, Order, OrderLine, OrderStatus, Product, Variant


def payload_hash(email: str, lines: list[tuple[uuid.UUID, int]]) -> str:
    ordered = sorted(lines, key=lambda item: (str(item[0]), item[1]))
    raw = email.strip().lower() + "\n" + "".join(f"{vid}:{qty}\n" for vid, qty in ordered)
    return hashlib.sha256(raw.encode()).hexdigest()


def advisory_key(key: str) -> tuple[int, int]:
    digest = hashlib.sha256(key.encode()).digest()
    k1 = int.from_bytes(digest[0:4], "big", signed=True)
    k2 = int.from_bytes(digest[4:8], "big", signed=True)
    return k1, k2


def list_products(session: Session) -> list[Product]:
    return list(session.scalars(select(Product).options(selectinload(Product.variants)).order_by(Product.name)))


def get_product(session: Session, product_id: uuid.UUID) -> Product:
    product = session.scalars(
        select(Product).options(selectinload(Product.variants)).where(Product.id == product_id)
    ).first()
    if product is None:
        raise NotFound("product not found")
    return product


def get_order(session: Session, order_id: uuid.UUID) -> Order:
    order = session.scalars(
        select(Order)
        .options(selectinload(Order.lines).selectinload(OrderLine.variant))
        .where(Order.id == order_id)
    ).first()
    if order is None:
        raise NotFound("order not found")
    return order


def checkout(session: Session, key: str, email: str, lines: list[tuple[uuid.UUID, int]]) -> Order:
    _validate(key, email, lines)
    merged = _merge(lines)
    digest = payload_hash(email, merged)
    k1, k2 = advisory_key(key)
    session.execute(text("SELECT pg_advisory_xact_lock(:k1, :k2)"), {"k1": k1, "k2": k2})

    existing = session.scalars(select(Order).where(Order.idempotency_key == key)).first()
    if existing is not None:
        if existing.payload_hash != digest:
            raise IdempotencyConflict("idempotency key reused with a different payload")
        return get_order(session, existing.id)

    try:
        return _place(session, key, email, digest, merged)
    except IntegrityError:
        session.rollback()
        replayed = session.scalars(select(Order).where(Order.idempotency_key == key)).first()
        if replayed is None:
            raise
        if replayed.payload_hash != digest:
            raise IdempotencyConflict("idempotency key reused with a different payload") from None
        return get_order(session, replayed.id)


def mark_paid(session: Session, order_id: uuid.UUID) -> Order:
    return _transition(session, order_id, OrderStatus.PAID)


def start_print(session: Session, order_id: uuid.UUID) -> Order:
    return _transition(session, order_id, OrderStatus.PRINTING)


def mark_shipped(session: Session, order_id: uuid.UUID) -> Order:
    return _transition(session, order_id, OrderStatus.SHIPPED)


def cancel_order(session: Session, order_id: uuid.UUID) -> Order:
    return _transition(session, order_id, OrderStatus.CANCELLED)


def _place(
    session: Session,
    key: str,
    email: str,
    digest: str,
    lines: list[tuple[uuid.UUID, int]],
) -> Order:
    built: list[OrderLine] = []
    total = 0
    currency: str | None = None

    for variant_id, qty in lines:
        variant = session.scalars(select(Variant).where(Variant.id == variant_id).with_for_update()).first()
        if variant is None:
            raise NotFound("variant not found")
        result = session.execute(
            update(Variant)
            .where(Variant.id == variant_id, (Variant.on_hand - Variant.reserved) >= qty)
            .values(reserved=Variant.reserved + qty)
        )
        if result.rowcount != 1:
            raise InsufficientInventory("insufficient inventory")
        if currency is None:
            currency = variant.currency
        elif currency != variant.currency:
            raise CurrencyMismatch("currency mismatch")
        total += variant.unit_amount_cents * qty
        built.append(
            OrderLine(
                variant_id=variant.id,
                quantity=qty,
                unit_amount_cents=variant.unit_amount_cents,
                currency=variant.currency,
            )
        )

    assert currency is not None
    order = Order(
        idempotency_key=key,
        payload_hash=digest,
        email=email,
        status=OrderStatus.PENDING.value,
        total_amount_cents=total,
        currency=currency,
        lines=built,
    )
    session.add(order)
    session.flush()
    return get_order(session, order.id)


def _transition(session: Session, order_id: uuid.UUID, nxt: OrderStatus) -> Order:
    order = session.scalars(select(Order).where(Order.id == order_id).with_for_update()).first()
    if order is None:
        raise NotFound("order not found")
    current = OrderStatus(order.status)
    if nxt not in ALLOWED[current]:
        raise InvalidTransition(f"invalid status transition: {current} → {nxt}")

    order = get_order(session, order_id)
    if nxt is OrderStatus.CANCELLED:
        _release(session, order)
    elif nxt is OrderStatus.SHIPPED:
        _consume(session, order)

    order.status = nxt.value
    session.flush()
    return get_order(session, order_id)


def _release(session: Session, order: Order) -> None:
    for line in order.lines:
        session.execute(
            update(Variant)
            .where(Variant.id == line.variant_id, Variant.reserved >= line.quantity)
            .values(reserved=Variant.reserved - line.quantity)
        )


def _consume(session: Session, order: Order) -> None:
    for line in order.lines:
        result = session.execute(
            update(Variant)
            .where(
                Variant.id == line.variant_id,
                Variant.reserved >= line.quantity,
                Variant.on_hand >= line.quantity,
            )
            .values(
                reserved=Variant.reserved - line.quantity,
                on_hand=Variant.on_hand - line.quantity,
            )
        )
        if result.rowcount != 1:
            raise InvalidTransition(f"cannot ship variant {line.variant_id}")


def _merge(lines: list[tuple[uuid.UUID, int]]) -> list[tuple[uuid.UUID, int]]:
    qty: dict[uuid.UUID, int] = defaultdict(int)
    order: list[uuid.UUID] = []
    for variant_id, n in lines:
        if variant_id not in qty:
            order.append(variant_id)
        qty[variant_id] += n
    return [(vid, qty[vid]) for vid in order]


def _validate(key: str, email: str, lines: list[tuple[uuid.UUID, int]]) -> None:
    if not 8 <= len(key) <= 128:
        raise InvalidInput("idempotency key must be 8–128 characters")
    if email.count("@") != 1 or not 5 <= len(email) <= 254:
        raise InvalidInput("email")
    if not lines:
        raise InvalidInput("at least one line")
    for _, qty in lines:
        if not 1 <= qty <= 20:
            raise InvalidInput("quantity must be 1–20")
