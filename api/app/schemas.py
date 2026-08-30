from __future__ import annotations

import uuid
from datetime import datetime

from pydantic import BaseModel, ConfigDict, Field
from pydantic.alias_generators import to_camel


class Schema(BaseModel):
    model_config = ConfigDict(alias_generator=to_camel, populate_by_name=True, from_attributes=True)


class Money(Schema):
    amount_cents: int
    currency: str


class VariantOut(Schema):
    id: uuid.UUID
    sku: str
    size: str
    finish: str
    unit_price: Money
    available: int


class ProductOut(Schema):
    id: uuid.UUID
    name: str
    description: str
    variants: list[VariantOut]


class OrderLineIn(Schema):
    variant_id: uuid.UUID
    quantity: int = Field(ge=1, le=20)


class CheckoutIn(Schema):
    idempotency_key: str = Field(min_length=8, max_length=128)
    email: str = Field(min_length=5, max_length=254)
    lines: list[OrderLineIn] = Field(min_length=1)


class OrderLineOut(Schema):
    variant: VariantOut
    quantity: int
    unit_price: Money


class OrderOut(Schema):
    id: uuid.UUID
    status: str
    email: str
    lines: list[OrderLineOut]
    total: Money
    created_at: datetime
    updated_at: datetime
