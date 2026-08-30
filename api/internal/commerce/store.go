package commerce

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

type Service struct {
	db *sql.DB
}

func New(db *sql.DB) *Service { return &Service{db: db} }

func (s *Service) Products(ctx context.Context) ([]Product, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.description,
		       v.id, v.product_id, v.sku, v.size, v.finish,
		       v.unit_amount_cents, v.currency, v.on_hand, v.reserved
		FROM products p
		JOIN variants v ON v.product_id = p.id
		ORDER BY p.name, v.sku`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectProducts(rows)
}

func (s *Service) Product(ctx context.Context, id string) (*Product, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.name, p.description,
		       v.id, v.product_id, v.sku, v.size, v.finish,
		       v.unit_amount_cents, v.currency, v.on_hand, v.reserved
		FROM products p
		JOIN variants v ON v.product_id = p.id
		WHERE p.id = $1
		ORDER BY v.sku`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	products, err := collectProducts(rows)
	if err != nil {
		return nil, err
	}
	if len(products) == 0 {
		return nil, ErrNotFound
	}
	return &products[0], nil
}

func (s *Service) Order(ctx context.Context, id string) (*Order, error) {
	return s.loadOrder(ctx, s.db, `o.id = $1`, id)
}

// Checkout reserves inventory and inserts an order.
//
// Same idempotency key + same payload returns the original order.
// Same key + different payload is a conflict.
// Concurrent first-use of a key is serialized with an advisory lock;
// the unique index is the durable invariant.
func (s *Service) Checkout(ctx context.Context, in CheckoutInput) (*Order, error) {
	if err := validateCheckout(in); err != nil {
		return nil, err
	}
	hash := payloadHash(in)
	k1, k2 := advisoryKey(in.IdempotencyKey)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1, $2)`, k1, k2); err != nil {
		return nil, err
	}

	existing, err := s.loadOrder(ctx, tx, `o.idempotency_key = $1`, in.IdempotencyKey)
	if err == nil {
		if existing.PayloadHash != hash {
			return nil, ErrIdempotencyConflict
		}
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	order, err := s.placeOrder(ctx, tx, in, hash)
	if err != nil {
		if errors.Is(err, errUniqueReplay) {
			_ = tx.Rollback()
			return s.replayOrConflict(ctx, in.IdempotencyKey, hash)
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Service) MarkPaid(ctx context.Context, id string) (*Order, error) {
	return s.transition(ctx, id, StatusPaid)
}

func (s *Service) StartPrint(ctx context.Context, id string) (*Order, error) {
	return s.transition(ctx, id, StatusPrinting)
}

func (s *Service) MarkShipped(ctx context.Context, id string) (*Order, error) {
	return s.transition(ctx, id, StatusShipped)
}

func (s *Service) Cancel(ctx context.Context, id string) (*Order, error) {
	return s.transition(ctx, id, StatusCancelled)
}

func (s *Service) placeOrder(ctx context.Context, tx *sql.Tx, in CheckoutInput, hash string) (*Order, error) {
	merged := mergeLines(in.Lines)
	lines := make([]OrderLine, 0, len(merged))
	var total Money

	for _, line := range merged {
		v, err := lockVariant(ctx, tx, line.VariantID)
		if err != nil {
			return nil, err
		}
		res, err := tx.ExecContext(ctx, `
			UPDATE variants
			SET reserved = reserved + $1
			WHERE id = $2 AND (on_hand - reserved) >= $1`,
			line.Quantity, line.VariantID)
		if err != nil {
			return nil, err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n != 1 {
			return nil, ErrInsufficientInventory
		}
		unit := v.UnitPrice
		if total.Currency == "" {
			total = Money{Currency: unit.Currency}
		}
		add, err := total.Add(unit.Times(line.Quantity))
		if err != nil {
			return nil, err
		}
		total = add
		v.Reserved += line.Quantity
		lines = append(lines, OrderLine{Variant: *v, Quantity: line.Quantity, UnitPrice: unit})
	}

	orderID := uuid.NewString()
	_, err := tx.ExecContext(ctx, `
		INSERT INTO orders (
			id, idempotency_key, payload_hash, email, status,
			total_amount_cents, currency
		) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		orderID, in.IdempotencyKey, hash, in.Email, StatusPending,
		total.AmountCents, total.Currency)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, errUniqueReplay
		}
		return nil, err
	}

	for _, line := range lines {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO order_lines (
				id, order_id, variant_id, quantity, unit_amount_cents, currency
			) VALUES ($1, $2, $3, $4, $5, $6)`,
			uuid.NewString(), orderID, line.Variant.ID, line.Quantity,
			line.UnitPrice.AmountCents, line.UnitPrice.Currency); err != nil {
			return nil, err
		}
	}

	return s.loadOrder(ctx, tx, `o.id = $1`, orderID)
}

var errUniqueReplay = errors.New("unique idempotency replay")

func (s *Service) replayOrConflict(ctx context.Context, key, hash string) (*Order, error) {
	existing, err := s.loadOrder(ctx, s.db, `o.idempotency_key = $1`, key)
	if err != nil {
		return nil, err
	}
	if existing.PayloadHash != hash {
		return nil, ErrIdempotencyConflict
	}
	return existing, nil
}

func (s *Service) transition(ctx context.Context, id string, next Status) (*Order, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var current Status
	err = tx.QueryRowContext(ctx, `SELECT status FROM orders WHERE id = $1 FOR UPDATE`, id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !allowedTransitions[current][next] {
		return nil, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, current, next)
	}

	order, err := s.loadOrder(ctx, tx, `o.id = $1`, id)
	if err != nil {
		return nil, err
	}
	switch next {
	case StatusCancelled:
		if err := releaseReservations(ctx, tx, order); err != nil {
			return nil, err
		}
	case StatusShipped:
		if err := consumeReservations(ctx, tx, order); err != nil {
			return nil, err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE orders SET status = $2, updated_at = now() WHERE id = $1`,
		id, next); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Order(ctx, id)
}

func releaseReservations(ctx context.Context, tx *sql.Tx, order *Order) error {
	for _, line := range order.Lines {
		if _, err := tx.ExecContext(ctx, `
			UPDATE variants SET reserved = reserved - $1 WHERE id = $2 AND reserved >= $1`,
			line.Quantity, line.Variant.ID); err != nil {
			return err
		}
	}
	return nil
}

func consumeReservations(ctx context.Context, tx *sql.Tx, order *Order) error {
	for _, line := range order.Lines {
		res, err := tx.ExecContext(ctx, `
			UPDATE variants
			SET reserved = reserved - $1, on_hand = on_hand - $1
			WHERE id = $2 AND reserved >= $1 AND on_hand >= $1`,
			line.Quantity, line.Variant.ID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return fmt.Errorf("%w: cannot ship variant %s", ErrInvalidTransition, line.Variant.ID)
		}
	}
	return nil
}

func lockVariant(ctx context.Context, tx *sql.Tx, id string) (*Variant, error) {
	var v Variant
	err := tx.QueryRowContext(ctx, `
		SELECT id, product_id, sku, size, finish, unit_amount_cents, currency, on_hand, reserved
		FROM variants WHERE id = $1 FOR UPDATE`, id).Scan(
		&v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Finish,
		&v.UnitPrice.AmountCents, &v.UnitPrice.Currency, &v.OnHand, &v.Reserved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: variant", ErrNotFound)
	}
	return &v, err
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func (s *Service) loadOrder(ctx context.Context, q querier, where string, arg any) (*Order, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT o.id, o.status, o.email, o.total_amount_cents, o.currency,
		       o.payload_hash, o.created_at, o.updated_at,
		       ol.quantity, ol.unit_amount_cents, ol.currency,
		       v.id, v.product_id, v.sku, v.size, v.finish,
		       v.unit_amount_cents, v.currency, v.on_hand, v.reserved
		FROM orders o
		JOIN order_lines ol ON ol.order_id = o.id
		JOIN variants v ON v.id = ol.variant_id
		WHERE `+where+`
		ORDER BY v.sku`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var order *Order
	for rows.Next() {
		var (
			o   Order
			l   OrderLine
			v   Variant
		)
		if err := rows.Scan(
			&o.ID, &o.Status, &o.Email, &o.Total.AmountCents, &o.Total.Currency,
			&o.PayloadHash, &o.CreatedAt, &o.UpdatedAt,
			&l.Quantity, &l.UnitPrice.AmountCents, &l.UnitPrice.Currency,
			&v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Finish,
			&v.UnitPrice.AmountCents, &v.UnitPrice.Currency, &v.OnHand, &v.Reserved,
		); err != nil {
			return nil, err
		}
		l.Variant = v
		if order == nil {
			order = &o
		}
		order.Lines = append(order.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if order == nil {
		return nil, ErrNotFound
	}
	return order, nil
}

func collectProducts(rows *sql.Rows) ([]Product, error) {
	byID := map[string]*Product{}
	var order []string
	for rows.Next() {
		var p Product
		var v Variant
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Description,
			&v.ID, &v.ProductID, &v.SKU, &v.Size, &v.Finish,
			&v.UnitPrice.AmountCents, &v.UnitPrice.Currency, &v.OnHand, &v.Reserved,
		); err != nil {
			return nil, err
		}
		existing, ok := byID[p.ID]
		if !ok {
			cp := p
			byID[p.ID] = &cp
			existing = &cp
			order = append(order, p.ID)
		}
		existing.Variants = append(existing.Variants, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Product, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

func mergeLines(in []LineInput) []LineInput {
	idx := map[string]int{}
	var out []LineInput
	for _, l := range in {
		if i, ok := idx[l.VariantID]; ok {
			out[i].Quantity += l.Quantity
			continue
		}
		idx[l.VariantID] = len(out)
		out = append(out, l)
	}
	return out
}

func validateCheckout(in CheckoutInput) error {
	if len(in.IdempotencyKey) < 8 || len(in.IdempotencyKey) > 128 {
		return fmt.Errorf("%w: idempotency key must be 8–128 characters", ErrInvalidInput)
	}
	if !validEmail(in.Email) {
		return fmt.Errorf("%w: email", ErrInvalidInput)
	}
	if len(in.Lines) == 0 {
		return fmt.Errorf("%w: at least one line", ErrInvalidInput)
	}
	for _, l := range in.Lines {
		if _, err := uuid.Parse(l.VariantID); err != nil {
			return fmt.Errorf("%w: variant id", ErrInvalidInput)
		}
		if l.Quantity < 1 || l.Quantity > 20 {
			return fmt.Errorf("%w: quantity must be 1–20", ErrInvalidInput)
		}
	}
	return nil
}

func validEmail(s string) bool {
	at := 0
	for _, c := range s {
		if c == '@' {
			at++
		}
	}
	return at == 1 && len(s) >= 5 && len(s) <= 254
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
