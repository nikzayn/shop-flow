// Package cache is a Redis read-through cache for products (cache-aside).
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nikzayn/shop-flow/services/catalog/internal/product"
)

// Redis caches products as JSON with a fixed TTL.
type Redis struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRedis creates the client. Timeouts are deliberately short: a slow cache
// is worse than no cache, because every request would wait on it.
func NewRedis(url string, ttl time.Duration) (*Redis, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse REDIS_URL: %w", err)
	}
	opts.DialTimeout = time.Second
	opts.ReadTimeout = 300 * time.Millisecond
	opts.WriteTimeout = 300 * time.Millisecond
	opts.MaxRetries = 1
	opts.DialerRetries = 1 // default is 5 x 100ms, which added ~1.6s per request while Redis was down
	return &Redis{client: redis.NewClient(opts), ttl: ttl}, nil
}

// key is versioned so a change to the cached JSON shape can be rolled out by bumping v1.
func key(id int64) string { return fmt.Sprintf("catalog:v1:product:%d", id) }

// GetProduct returns the cached product and whether it was found.
func (r *Redis) GetProduct(ctx context.Context, id int64) (product.Product, bool, error) {
	b, err := r.client.Get(ctx, key(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return product.Product{}, false, nil
	}
	if err != nil {
		return product.Product{}, false, fmt.Errorf("get product %d from cache: %w", id, err)
	}
	var p product.Product
	if err := json.Unmarshal(b, &p); err != nil {
		return product.Product{}, false, fmt.Errorf("decode cached product %d: %w", id, err)
	}
	return p, true, nil
}

// SetProduct caches a product for the configured TTL.
func (r *Redis) SetProduct(ctx context.Context, p product.Product) error {
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode product %d: %w", p.ID, err)
	}
	if err := r.client.Set(ctx, key(p.ID), b, r.ttl).Err(); err != nil {
		return fmt.Errorf("set product %d in cache: %w", p.ID, err)
	}
	return nil
}

// Ping checks that Redis is reachable.
func (r *Redis) Ping(ctx context.Context) error { return r.client.Ping(ctx).Err() }

// Close closes the client.
func (r *Redis) Close() error { return r.client.Close() }
