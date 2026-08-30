from concurrent.futures import ThreadPoolExecutor, as_completed
from uuid import uuid4

import pytest
from sqlalchemy.orm import Session

from app import commerce
from app.errors import IdempotencyConflict, InsufficientInventory
from app.models import OrderStatus, Product, Variant


def _variant(session: Session, on_hand: int = 10, cents: int = 500) -> Variant:
    product = Product(id=uuid4(), name="Test Print", description="fixture")
    variant = Variant(
        id=uuid4(),
        product_id=product.id,
        sku=f"SKU-{uuid4().hex[:8]}",
        size="8×10",
        finish="Smooth",
        unit_amount_cents=cents,
        currency="USD",
        on_hand=on_hand,
        reserved=0,
    )
    session.add_all([product, variant])
    session.commit()
    session.refresh(variant)
    return variant


def _reserved(session: Session, variant_id) -> int:
    session.expire_all()
    return session.get(Variant, variant_id).reserved


def test_checkout_idempotent_same_payload(session: Session):
    variant = _variant(session)
    lines = [(variant.id, 1)]
    first = commerce.checkout(session, "replay-same-payload-key", "buyer@example.com", lines)
    session.commit()
    second = commerce.checkout(session, "replay-same-payload-key", "buyer@example.com", lines)
    session.commit()
    assert first.id == second.id
    assert _reserved(session, variant.id) == 1


def test_checkout_idempotent_conflict(session: Session):
    variant = _variant(session)
    commerce.checkout(session, "replay-conflict-key-01", "one@example.com", [(variant.id, 1)])
    session.commit()
    with pytest.raises(IdempotencyConflict):
        commerce.checkout(session, "replay-conflict-key-01", "two@example.com", [(variant.id, 1)])
    assert _reserved(session, variant.id) == 1


def test_checkout_does_not_oversell_under_race(engine, session: Session):
    variant = _variant(session, on_hand=1)
    variant_id = variant.id

    def worker(i: int) -> str:
        with Session(engine) as own:
            try:
                commerce.checkout(
                    own,
                    f"race-key-{i:02d}-xxxx",
                    f"r{i}@example.com",
                    [(variant_id, 1)],
                )
                own.commit()
                return "ok"
            except InsufficientInventory:
                own.rollback()
                return "fail"

    with ThreadPoolExecutor(max_workers=20) as pool:
        results = [fut.result() for fut in as_completed(pool.submit(worker, i) for i in range(20))]

    assert results.count("ok") == 1
    assert results.count("fail") == 19
    assert _reserved(session, variant_id) == 1


def test_cancel_releases_reservation(session: Session):
    variant = _variant(session)
    order = commerce.checkout(session, "cancel-release-key", "buyer@example.com", [(variant.id, 2)])
    session.commit()
    assert _reserved(session, variant.id) == 2
    cancelled = commerce.cancel_order(session, order.id)
    session.commit()
    assert cancelled.status == OrderStatus.CANCELLED
    assert _reserved(session, variant.id) == 0


def test_ship_consumes_inventory(session: Session):
    variant = _variant(session, on_hand=5)
    order = commerce.checkout(session, "ship-consume-key", "buyer@example.com", [(variant.id, 2)])
    session.commit()
    commerce.mark_paid(session, order.id)
    commerce.start_print(session, order.id)
    shipped = commerce.mark_shipped(session, order.id)
    session.commit()
    assert shipped.status == OrderStatus.SHIPPED
    session.expire_all()
    stock = session.get(Variant, variant.id)
    assert stock.reserved == 0
    assert stock.on_hand == 3


def test_money_stays_integer_cents(session: Session):
    variant = _variant(session, cents=500)
    order = commerce.checkout(session, "money-cents-key", "buyer@example.com", [(variant.id, 3)])
    session.commit()
    assert order.currency == "USD"
    assert order.total_amount_cents == 1500
