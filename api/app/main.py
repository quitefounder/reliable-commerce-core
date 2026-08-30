from __future__ import annotations

from contextlib import asynccontextmanager
from pathlib import Path
from uuid import UUID

from alembic import command
from alembic.config import Config
from fastapi import Depends, FastAPI
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from sqlalchemy.orm import Session

from app import commerce, seed
from app.config import settings
from app.db import SessionLocal, get_session
from app.errors import CommerceError
from app.models import Product, Variant
from app.schemas import CheckoutIn, Money, OrderLineOut, OrderOut, ProductOut, VariantOut


def _migrate() -> None:
    ini = Path(__file__).resolve().parents[1] / "alembic.ini"
    cfg = Config(str(ini))
    cfg.set_main_option("sqlalchemy.url", settings.sqlalchemy_url())
    command.upgrade(cfg, "head")


def create_app() -> FastAPI:
    @asynccontextmanager
    async def lifespan(_app: FastAPI):
        _migrate()
        with SessionLocal() as session:
            seed.if_empty(session)
        yield

    app = FastAPI(title="Reliable Commerce Core", version="0.1.0", lifespan=lifespan)
    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.origin_list(),
        allow_methods=["GET", "POST", "OPTIONS"],
        allow_headers=["Content-Type"],
    )

    @app.exception_handler(CommerceError)
    async def on_commerce_error(_request, exc: CommerceError) -> JSONResponse:
        return JSONResponse(
            status_code=exc.status,
            content={"detail": {"code": exc.code, "message": exc.message}},
        )

    @app.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/products", response_model=list[ProductOut])
    def products(session: Session = Depends(get_session)) -> list[ProductOut]:
        return [_product(p) for p in commerce.list_products(session)]

    @app.get("/products/{product_id}", response_model=ProductOut)
    def product(product_id: UUID, session: Session = Depends(get_session)) -> ProductOut:
        return _product(commerce.get_product(session, product_id))

    @app.get("/orders/{order_id}", response_model=OrderOut)
    def order(order_id: UUID, session: Session = Depends(get_session)) -> OrderOut:
        return _order(commerce.get_order(session, order_id))

    @app.post("/checkout", response_model=OrderOut)
    def checkout(body: CheckoutIn, session: Session = Depends(get_session)) -> OrderOut:
        lines = [(line.variant_id, line.quantity) for line in body.lines]
        return _order(commerce.checkout(session, body.idempotency_key, body.email, lines))

    @app.post("/orders/{order_id}/paid", response_model=OrderOut)
    def paid(order_id: UUID, session: Session = Depends(get_session)) -> OrderOut:
        return _order(commerce.mark_paid(session, order_id))

    @app.post("/orders/{order_id}/print", response_model=OrderOut)
    def printing(order_id: UUID, session: Session = Depends(get_session)) -> OrderOut:
        return _order(commerce.start_print(session, order_id))

    @app.post("/orders/{order_id}/ship", response_model=OrderOut)
    def ship(order_id: UUID, session: Session = Depends(get_session)) -> OrderOut:
        return _order(commerce.mark_shipped(session, order_id))

    @app.post("/orders/{order_id}/cancel", response_model=OrderOut)
    def cancel(order_id: UUID, session: Session = Depends(get_session)) -> OrderOut:
        return _order(commerce.cancel_order(session, order_id))

    return app


def _variant(v: Variant) -> VariantOut:
    return VariantOut(
        id=v.id,
        sku=v.sku,
        size=v.size,
        finish=v.finish,
        unit_price=Money(amount_cents=v.unit_amount_cents, currency=v.currency),
        available=v.available,
    )


def _product(p: Product) -> ProductOut:
    return ProductOut(
        id=p.id,
        name=p.name,
        description=p.description,
        variants=[_variant(v) for v in p.variants],
    )


def _order(o) -> OrderOut:
    return OrderOut(
        id=o.id,
        status=o.status,
        email=o.email,
        lines=[
            OrderLineOut(
                variant=_variant(line.variant),
                quantity=line.quantity,
                unit_price=Money(amount_cents=line.unit_amount_cents, currency=line.currency),
            )
            for line in o.lines
        ],
        total=Money(amount_cents=o.total_amount_cents, currency=o.currency),
        created_at=o.created_at,
        updated_at=o.updated_at,
    )


app = create_app()
