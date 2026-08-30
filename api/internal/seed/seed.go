package seed

import (
	"context"
	"database/sql"
)

// Catalog is the storefront the UI boots against. IDs are stable so
// docs and the review script can name them.
const (
	ProductStudioPrint  = "01990000-0000-7000-8000-000000000001"
	ProductUtilityTee   = "01990000-0000-7000-8000-000000000002"
	ProductFieldJournal = "01990000-0000-7000-8000-000000000003"

	VariantPrint8x10     = "01990000-0000-7000-8000-000000000011"
	VariantPrint11x14    = "01990000-0000-7000-8000-000000000012"
	VariantPrint16x20    = "01990000-0000-7000-8000-000000000013"
	VariantTeeMNatural   = "01990000-0000-7000-8000-000000000021"
	VariantTeeLNatural   = "01990000-0000-7000-8000-000000000022"
	VariantTeeMWashed    = "01990000-0000-7000-8000-000000000023"
	VariantJournalPocket = "01990000-0000-7000-8000-000000000031"
	VariantJournalDesk   = "01990000-0000-7000-8000-000000000032"
	VariantJournalWire   = "01990000-0000-7000-8000-000000000033"
)

func IfEmpty(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM products`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	products := []struct {
		id, name, desc string
	}{
		{ProductStudioPrint, "Studio Print", "Archival rag paper. Size and surface are the variants."},
		{ProductUtilityTee, "Utility Tee", "Midweight cotton. Cut first, wash second."},
		{ProductFieldJournal, "Field Journal", "Threadbound or wire. Pocket or desk."},
	}
	for _, p := range products {
		if _, err := db.ExecContext(ctx,
			`INSERT INTO products (id, name, description) VALUES ($1, $2, $3)`,
			p.id, p.name, p.desc); err != nil {
			return err
		}
	}

	type v struct {
		id, product, sku, size, finish string
		cents, onHand                  int
	}
	variants := []v{
		{VariantPrint8x10, ProductStudioPrint, "PRINT-8X10-SMOOTH", "8×10", "Smooth", 1800, 25},
		{VariantPrint11x14, ProductStudioPrint, "PRINT-11X14-SMOOTH", "11×14", "Smooth", 2800, 25},
		{VariantPrint16x20, ProductStudioPrint, "PRINT-16X20-TEXTURED", "16×20", "Textured", 4200, 8},
		{VariantTeeMNatural, ProductUtilityTee, "TEE-M-NATURAL", "M", "Natural", 3200, 12},
		{VariantTeeLNatural, ProductUtilityTee, "TEE-L-NATURAL", "L", "Natural", 3200, 12},
		{VariantTeeMWashed, ProductUtilityTee, "TEE-M-WASHED", "M", "Washed", 3600, 4},
		{VariantJournalPocket, ProductFieldJournal, "JOURNAL-POCKET-THREAD", "Pocket", "Threadbound", 1400, 30},
		{VariantJournalDesk, ProductFieldJournal, "JOURNAL-DESK-THREAD", "Desk", "Threadbound", 2200, 20},
		{VariantJournalWire, ProductFieldJournal, "JOURNAL-DESK-WIRE", "Desk", "Wire", 2000, 2},
	}
	for _, row := range variants {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO variants (
				id, product_id, sku, size, finish,
				unit_amount_cents, currency, on_hand, reserved
			) VALUES ($1, $2, $3, $4, $5, $6, 'USD', $7, 0)`,
			row.id, row.product, row.sku, row.size, row.finish, row.cents, row.onHand); err != nil {
			return err
		}
	}
	return nil
}
