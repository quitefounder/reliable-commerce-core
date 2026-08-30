package graph

import "github.com/quitefounder/reliable-commerce-core/api/internal/commerce"

// Resolver is the gqlgen root. It is a thin map onto commerce.Service.
type Resolver struct {
	Commerce *commerce.Service
}
