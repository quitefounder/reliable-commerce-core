package commerce_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/quitefounder/reliable-commerce-core/api/internal/commerce"
	"github.com/quitefounder/reliable-commerce-core/api/internal/migrate"
	"github.com/quitefounder/reliable-commerce-core/api/migrations"
)

func TestCheckoutIdempotentSamePayload(t *testing.T) {
	svc, variant := setup(t)
	ctx := context.Background()
	in := checkout(variant, "replay-same-payload-key", "buyer@example.com", 1)

	first, err := svc.Checkout(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Checkout(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same order id, got %s and %s", first.ID, second.ID)
	}
	assertReserved(t, svc, variant, 1)
}

func TestCheckoutIdempotentConflict(t *testing.T) {
	svc, variant := setup(t)
	ctx := context.Background()
	key := "replay-conflict-key-01"
	_, err := svc.Checkout(ctx, checkout(variant, key, "one@example.com", 1))
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Checkout(ctx, checkout(variant, key, "two@example.com", 1))
	if !errors.Is(err, commerce.ErrIdempotencyConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	assertReserved(t, svc, variant, 1)
}

func TestCheckoutDoesNotOversellUnderRace(t *testing.T) {
	svc, variant := setupStock(t, 1)
	ctx := context.Background()

	const workers = 20
	var ok atomic.Int64
	var fail atomic.Int64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		i := i
		go func() {
			defer wg.Done()
			_, err := svc.Checkout(ctx, checkout(variant, fmt.Sprintf("race-key-%02d-xxxx", i), fmt.Sprintf("r%d@example.com", i), 1))
			if err == nil {
				ok.Add(1)
				return
			}
			if !errors.Is(err, commerce.ErrInsufficientInventory) {
				t.Errorf("unexpected error: %v", err)
			}
			fail.Add(1)
		}()
	}
	wg.Wait()

	if ok.Load() != 1 {
		t.Fatalf("expected exactly 1 successful checkout, got %d", ok.Load())
	}
	if fail.Load() != workers-1 {
		t.Fatalf("expected %d inventory failures, got %d", workers-1, fail.Load())
	}
	assertReserved(t, svc, variant, 1)
}

func TestCancelReleasesReservation(t *testing.T) {
	svc, variant := setup(t)
	ctx := context.Background()
	order, err := svc.Checkout(ctx, checkout(variant, "cancel-release-key", "buyer@example.com", 2))
	if err != nil {
		t.Fatal(err)
	}
	assertReserved(t, svc, variant, 2)

	cancelled, err := svc.Cancel(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != commerce.StatusCancelled {
		t.Fatalf("status %s", cancelled.Status)
	}
	assertReserved(t, svc, variant, 0)
}

func TestShipConsumesInventory(t *testing.T) {
	svc, variant := setupStock(t, 5)
	ctx := context.Background()
	order, err := svc.Checkout(ctx, checkout(variant, "ship-consume-key", "buyer@example.com", 2))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkPaid(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartPrint(ctx, order.ID); err != nil {
		t.Fatal(err)
	}
	shipped, err := svc.MarkShipped(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if shipped.Status != commerce.StatusShipped {
		t.Fatalf("status %s", shipped.Status)
	}
	v := mustVariant(t, svc, variant)
	if v.Reserved != 0 || v.OnHand != 3 {
		t.Fatalf("after ship reserved=%d on_hand=%d", v.Reserved, v.OnHand)
	}
}

func TestMoneyStaysIntegerCents(t *testing.T) {
	svc, variant := setup(t)
	ctx := context.Background()
	order, err := svc.Checkout(ctx, checkout(variant, "money-cents-key", "buyer@example.com", 3))
	if err != nil {
		t.Fatal(err)
	}
	if order.Total.Currency != "USD" {
		t.Fatalf("currency %s", order.Total.Currency)
	}
	if order.Total.AmountCents != 1500 {
		t.Fatalf("expected 3 × 500 = 1500 cents, got %d", order.Total.AmountCents)
	}
}

func setup(t *testing.T) (*commerce.Service, string) {
	t.Helper()
	return setupStock(t, 10)
}

func setupStock(t *testing.T, onHand int) (*commerce.Service, string) {
	t.Helper()
	db := testDB(t)
	svc := commerce.New(db)
	variant := insertVariant(t, db, onHand, 500)
	return svc, variant
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://commerce:commerce@127.0.0.1:5432/commerce_test?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("postgres required for this suite: %v", err)
	}
	if err := migrate.Up(db, migrations.Files); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertVariant(t *testing.T, db *sql.DB, onHand, cents int) string {
	t.Helper()
	productID := uuid.NewString()
	variantID := uuid.NewString()
	if _, err := db.Exec(`
		INSERT INTO products (id, name, description) VALUES ($1, $2, $3)`,
		productID, "Test Print", "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO variants (
			id, product_id, sku, size, finish,
			unit_amount_cents, currency, on_hand, reserved
		) VALUES ($1, $2, $3, '8×10', 'Smooth', $4, 'USD', $5, 0)`,
		variantID, productID, "SKU-"+variantID[:8], cents, onHand); err != nil {
		t.Fatal(err)
	}
	return variantID
}

func checkout(variantID, key, email string, qty int) commerce.CheckoutInput {
	return commerce.CheckoutInput{
		IdempotencyKey: key,
		Email:          email,
		Lines:          []commerce.LineInput{{VariantID: variantID, Quantity: qty}},
	}
}

func assertReserved(t *testing.T, svc *commerce.Service, variantID string, want int) {
	t.Helper()
	v := mustVariant(t, svc, variantID)
	if v.Reserved != want {
		t.Fatalf("reserved=%d want %d", v.Reserved, want)
	}
}

func mustVariant(t *testing.T, svc *commerce.Service, id string) commerce.Variant {
	t.Helper()
	products, err := svc.Products(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range products {
		for _, v := range p.Variants {
			if v.ID == id {
				return v
			}
		}
	}
	t.Fatalf("variant %s not found", id)
	return commerce.Variant{}
}
