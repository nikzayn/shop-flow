// Package product holds the catalog's domain types, shared by the store, cache and API layers.
package product

import "errors"

// ErrNotFound is returned when a product does not exist.
var ErrNotFound = errors.New("product not found")

// Product is a sellable item. Field order matches the products table columns,
// so rows can be scanned positionally.
type Product struct {
	ID         int64  `json:"id"`
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	PriceCents int64  `json:"price_cents"`
	Stock      int32  `json:"stock"`
}
