// Package store reads products from Postgres, the catalog's source of truth.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nikzayn/shop-flow/services/catalog/internal/product"
)

const productColumns = "id, sku, name, price_cents, stock"

// Postgres is a product store backed by a pgx connection pool.
// Pool size can be tuned via the DATABASE_URL, e.g. ?pool_max_conns=10.
type Postgres struct {
	pool *pgxpool.Pool
}

// NewPostgres creates the pool. Connections are opened lazily, so the service
// starts even if Postgres is down; /readyz reports it instead of crash-looping.
func NewPostgres(ctx context.Context, url string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	return &Postgres{pool: pool}, nil
}

// Close releases all pooled connections.
func (p *Postgres) Close() { p.pool.Close() }

// Ping checks that Postgres is reachable.
func (p *Postgres) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// GetProduct returns the product with the given id, or product.ErrNotFound.
func (p *Postgres) GetProduct(ctx context.Context, id int64) (product.Product, error) {
	rows, err := p.pool.Query(ctx, "SELECT "+productColumns+" FROM products WHERE id = $1", id)
	if err != nil {
		return product.Product{}, fmt.Errorf("query product %d: %w", id, err)
	}
	pr, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[product.Product])
	if errors.Is(err, pgx.ErrNoRows) {
		return product.Product{}, product.ErrNotFound
	}
	if err != nil {
		return product.Product{}, fmt.Errorf("scan product %d: %w", id, err)
	}
	return pr, nil
}

// ListProducts returns up to limit products ordered by id.
func (p *Postgres) ListProducts(ctx context.Context, limit int) ([]product.Product, error) {
	rows, err := p.pool.Query(ctx, "SELECT "+productColumns+" FROM products ORDER BY id LIMIT $1", limit)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	products, err := pgx.CollectRows(rows, pgx.RowToStructByPos[product.Product])
	if err != nil {
		return nil, fmt.Errorf("scan products: %w", err)
	}
	return products, nil
}
