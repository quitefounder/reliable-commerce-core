package graph

import (
	"errors"

	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/quitefounder/reliable-commerce-core/api/graph/model"
	"github.com/quitefounder/reliable-commerce-core/api/internal/commerce"
)

func mapMoney(m commerce.Money) *model.Money {
	return &model.Money{AmountCents: m.AmountCents, Currency: m.Currency}
}

func mapVariant(v commerce.Variant) *model.Variant {
	return &model.Variant{
		ID:        v.ID,
		Sku:       v.SKU,
		Size:      v.Size,
		Finish:    v.Finish,
		UnitPrice: mapMoney(v.UnitPrice),
		Available: v.Available(),
	}
}

func mapProduct(p commerce.Product) *model.Product {
	vs := make([]*model.Variant, len(p.Variants))
	for i, v := range p.Variants {
		vs[i] = mapVariant(v)
	}
	return &model.Product{
		ID:          p.ID,
		Name:        p.Name,
		Description: p.Description,
		Variants:    vs,
	}
}

func mapOrder(o *commerce.Order) *model.Order {
	lines := make([]*model.OrderLine, len(o.Lines))
	for i, l := range o.Lines {
		lines[i] = &model.OrderLine{
			Variant:   mapVariant(l.Variant),
			Quantity:  l.Quantity,
			UnitPrice: mapMoney(l.UnitPrice),
		}
	}
	return &model.Order{
		ID:        o.ID,
		Status:    model.OrderStatus(o.Status),
		Email:     o.Email,
		Lines:     lines,
		Total:     mapMoney(o.Total),
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
	}
}

func gqlErr(err error) error {
	code := "INTERNAL"
	switch {
	case errors.Is(err, commerce.ErrInvalidInput):
		code = "INVALID_INPUT"
	case errors.Is(err, commerce.ErrInsufficientInventory):
		code = "INSUFFICIENT_INVENTORY"
	case errors.Is(err, commerce.ErrIdempotencyConflict):
		code = "IDEMPOTENCY_CONFLICT"
	case errors.Is(err, commerce.ErrNotFound):
		code = "NOT_FOUND"
	case errors.Is(err, commerce.ErrInvalidTransition):
		code = "INVALID_TRANSITION"
	case errors.Is(err, commerce.ErrCurrencyMismatch):
		code = "CURRENCY_MISMATCH"
	}
	return &gqlerror.Error{Message: err.Error(), Extensions: map[string]any{"code": code}}
}
