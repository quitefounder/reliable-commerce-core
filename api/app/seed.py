import uuid

from sqlalchemy import func, select
from sqlalchemy.orm import Session

from app.models import Product, Variant

PRODUCT_STUDIO = uuid.UUID("01990000-0000-7000-8000-000000000001")
PRODUCT_TEE = uuid.UUID("01990000-0000-7000-8000-000000000002")
PRODUCT_JOURNAL = uuid.UUID("01990000-0000-7000-8000-000000000003")

VARIANT_PRINT_8X10 = uuid.UUID("01990000-0000-7000-8000-000000000011")
VARIANT_PRINT_11X14 = uuid.UUID("01990000-0000-7000-8000-000000000012")
VARIANT_PRINT_16X20 = uuid.UUID("01990000-0000-7000-8000-000000000013")
VARIANT_TEE_M_NATURAL = uuid.UUID("01990000-0000-7000-8000-000000000021")
VARIANT_TEE_L_NATURAL = uuid.UUID("01990000-0000-7000-8000-000000000022")
VARIANT_TEE_M_WASHED = uuid.UUID("01990000-0000-7000-8000-000000000023")
VARIANT_JOURNAL_POCKET = uuid.UUID("01990000-0000-7000-8000-000000000031")
VARIANT_JOURNAL_DESK = uuid.UUID("01990000-0000-7000-8000-000000000032")
VARIANT_JOURNAL_WIRE = uuid.UUID("01990000-0000-7000-8000-000000000033")


def if_empty(session: Session) -> None:
    n = session.scalar(select(func.count()).select_from(Product))
    if n:
        return

    products = [
        Product(id=PRODUCT_STUDIO, name="Studio Print", description="Archival rag paper. Size and surface are the variants."),
        Product(id=PRODUCT_TEE, name="Utility Tee", description="Midweight cotton. Cut first, wash second."),
        Product(id=PRODUCT_JOURNAL, name="Field Journal", description="Threadbound or wire. Pocket or desk."),
    ]
    session.add_all(products)

    rows = [
        (VARIANT_PRINT_8X10, PRODUCT_STUDIO, "PRINT-8X10-SMOOTH", "8×10", "Smooth", 1800, 25),
        (VARIANT_PRINT_11X14, PRODUCT_STUDIO, "PRINT-11X14-SMOOTH", "11×14", "Smooth", 2800, 25),
        (VARIANT_PRINT_16X20, PRODUCT_STUDIO, "PRINT-16X20-TEXTURED", "16×20", "Textured", 4200, 8),
        (VARIANT_TEE_M_NATURAL, PRODUCT_TEE, "TEE-M-NATURAL", "M", "Natural", 3200, 12),
        (VARIANT_TEE_L_NATURAL, PRODUCT_TEE, "TEE-L-NATURAL", "L", "Natural", 3200, 12),
        (VARIANT_TEE_M_WASHED, PRODUCT_TEE, "TEE-M-WASHED", "M", "Washed", 3600, 4),
        (VARIANT_JOURNAL_POCKET, PRODUCT_JOURNAL, "JOURNAL-POCKET-THREAD", "Pocket", "Threadbound", 1400, 30),
        (VARIANT_JOURNAL_DESK, PRODUCT_JOURNAL, "JOURNAL-DESK-THREAD", "Desk", "Threadbound", 2200, 20),
        (VARIANT_JOURNAL_WIRE, PRODUCT_JOURNAL, "JOURNAL-DESK-WIRE", "Desk", "Wire", 2000, 2),
    ]
    session.add_all(
        [
            Variant(
                id=vid,
                product_id=pid,
                sku=sku,
                size=size,
                finish=finish,
                unit_amount_cents=cents,
                currency="USD",
                on_hand=on_hand,
                reserved=0,
            )
            for vid, pid, sku, size, finish, cents, on_hand in rows
        ]
    )
    session.commit()
