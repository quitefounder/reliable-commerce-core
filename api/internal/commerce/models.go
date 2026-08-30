package commerce

import "time"

// Money is integer minor units. Never a float.
type Money struct {
	AmountCents int
	Currency    string
}

func (m Money) Add(n Money) (Money, error) {
	if m.Currency != n.Currency {
		return Money{}, ErrCurrencyMismatch
	}
	return Money{AmountCents: m.AmountCents + n.AmountCents, Currency: m.Currency}, nil
}

func (m Money) Times(qty int) Money {
	return Money{AmountCents: m.AmountCents * qty, Currency: m.Currency}
}

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusPaid      Status = "PAID"
	StatusPrinting  Status = "PRINTING"
	StatusShipped   Status = "SHIPPED"
	StatusCancelled Status = "CANCELLED"
)

// allowedTransitions is the entire fulfillment machine.
var allowedTransitions = map[Status]map[Status]bool{
	StatusPending:  {StatusPaid: true, StatusCancelled: true},
	StatusPaid:     {StatusPrinting: true, StatusCancelled: true},
	StatusPrinting: {StatusShipped: true, StatusCancelled: true},
	StatusShipped:  {},
	StatusCancelled: {},
}

type Product struct {
	ID          string
	Name        string
	Description string
	Variants    []Variant
}

type Variant struct {
	ID        string
	ProductID string
	SKU       string
	Size      string
	Finish    string
	UnitPrice Money
	OnHand    int
	Reserved  int
}

func (v Variant) Available() int { return v.OnHand - v.Reserved }

type LineInput struct {
	VariantID string
	Quantity  int
}

type CheckoutInput struct {
	IdempotencyKey string
	Email          string
	Lines          []LineInput
}

type OrderLine struct {
	Variant   Variant
	Quantity  int
	UnitPrice Money
}

type Order struct {
	ID          string
	Status      Status
	Email       string
	Lines       []OrderLine
	Total       Money
	PayloadHash string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
