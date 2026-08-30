package commerce

import "errors"

var (
	ErrInvalidInput          = errors.New("invalid input")
	ErrCurrencyMismatch      = errors.New("currency mismatch")
	ErrInsufficientInventory = errors.New("insufficient inventory")
	ErrIdempotencyConflict   = errors.New("idempotency key reused with a different payload")
	ErrNotFound              = errors.New("not found")
	ErrInvalidTransition     = errors.New("invalid status transition")
)
