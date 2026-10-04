package api

type ProductStore interface {
	GetProduct(ctx, id string) (Product, error)
}

type Product struct {
	ID    string `json:"id"`
	SKU   string `json:"sku"`
	Name  string `json:"name"`
	Price int    `json:"price"`
}
