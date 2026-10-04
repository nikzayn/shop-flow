// Package api exposes the catalog over HTTP.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/nikzayn/shop-flow/services/catalog/internal/product"
)

// ProductStore is the source of truth for products (Postgres in production).
type ProductStore interface {
	GetProduct(ctx context.Context, id int64) (product.Product, error)
	ListProducts(ctx context.Context, limit int) ([]product.Product, error)
	Ping(ctx context.Context) error
}

// ProductCache is an optional speed-up in front of the store (Redis in production).
type ProductCache interface {
	GetProduct(ctx context.Context, id int64) (product.Product, bool, error)
	SetProduct(ctx context.Context, p product.Product) error
	Ping(ctx context.Context) error
}

const (
	defaultLimit = 20
	maxLimit     = 100
	queryTimeout = 2 * time.Second
	pingTimeout  = time.Second
)

// Handler serves the catalog HTTP API.
type Handler struct {
	store ProductStore
	cache ProductCache
	log   *slog.Logger
}

// New returns a Handler.
func New(store ProductStore, cache ProductCache, log *slog.Logger) *Handler {
	return &Handler{store: store, cache: cache, log: log}
}

// Routes returns the HTTP handler with all routes and request logging.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /readyz", h.readyz)
	mux.HandleFunc("GET /products", h.listProducts)
	mux.HandleFunc("GET /products/{id}", h.getProduct)
	return h.logRequests(mux)
}

// healthz is the liveness probe: it only proves the process can serve HTTP.
// It must never check dependencies, or a database outage would make Kubernetes
// restart every healthy pod.
func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readyz is the readiness probe. Postgres is required. Redis is not: without it
// the catalog still works, just slower, so it reports "degraded" but stays ready.
func (h *Handler) readyz(w http.ResponseWriter, r *http.Request) {
	status := map[string]string{"postgres": "ok", "redis": "ok"}
	code := http.StatusOK

	if err := ping(r.Context(), h.store.Ping); err != nil {
		h.log.Warn("readiness: postgres unreachable", "err", err)
		status["postgres"] = "down"
		code = http.StatusServiceUnavailable
	}
	if err := ping(r.Context(), h.cache.Ping); err != nil {
		h.log.Warn("readiness: redis unreachable", "err", err)
		status["redis"] = "degraded"
	}
	writeJSON(w, code, status)
}

func ping(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	return fn(ctx)
}

func (h *Handler) listProducts(w http.ResponseWriter, r *http.Request) {
	limit := defaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxLimit {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = n
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	products, err := h.store.ListProducts(ctx, limit)
	if err != nil {
		h.log.Error("list products", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if products == nil {
		products = []product.Product{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": products})
}

// getProduct uses cache-aside: try Redis, fall back to Postgres, then fill Redis.
// Cache failures are logged and ignored so a Redis outage never fails a request.
// The X-Cache response header (HIT/MISS) shows which path served the request.
func (h *Handler) getProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), queryTimeout)
	defer cancel()

	p, hit, cacheErr := h.cache.GetProduct(ctx, id)
	if cacheErr != nil {
		h.log.Warn("cache read failed", "product_id", id, "err", cacheErr)
	}
	if hit {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, http.StatusOK, p)
		return
	}
	w.Header().Set("X-Cache", "MISS")

	p, err = h.store.GetProduct(ctx, id)
	if errors.Is(err, product.ErrNotFound) {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}
	if err != nil {
		h.log.Error("get product", "product_id", id, "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Skip the write if the read just failed: Redis is likely down, and waiting
	// on it twice would double the latency of every request.
	if cacheErr == nil {
		if err := h.cache.SetProduct(ctx, p); err != nil {
			h.log.Warn("cache write failed", "product_id", id, "err", err)
		}
	}
	writeJSON(w, http.StatusOK, p)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
