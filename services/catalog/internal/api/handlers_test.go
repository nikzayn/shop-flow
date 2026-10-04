package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nikzayn/shop-flow/services/catalog/internal/product"
)

var errDown = errors.New("connection refused")

var (
	shoe  = product.Product{ID: 1, SKU: "SHOE-001", Name: "Running Shoe", PriceCents: 4999, Stock: 25}
	shirt = product.Product{ID: 2, SKU: "TSHIRT-001", Name: "Cotton T-Shirt", PriceCents: 1499, Stock: 100}
)

type fakeStore struct {
	products []product.Product
	err      error
	pingErr  error
	calls    int
}

func (f *fakeStore) GetProduct(_ context.Context, id int64) (product.Product, error) {
	f.calls++
	if f.err != nil {
		return product.Product{}, f.err
	}
	for _, p := range f.products {
		if p.ID == id {
			return p, nil
		}
	}
	return product.Product{}, product.ErrNotFound
}

func (f *fakeStore) ListProducts(_ context.Context, limit int) ([]product.Product, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.products[:min(limit, len(f.products))], nil
}

func (f *fakeStore) Ping(context.Context) error { return f.pingErr }

type fakeCache struct {
	items   map[int64]product.Product
	getErr  error
	setErr  error
	pingErr error
	sets    int
}

func newFakeCache(products ...product.Product) *fakeCache {
	c := &fakeCache{items: map[int64]product.Product{}}
	for _, p := range products {
		c.items[p.ID] = p
	}
	return c
}

func (f *fakeCache) GetProduct(_ context.Context, id int64) (product.Product, bool, error) {
	if f.getErr != nil {
		return product.Product{}, false, f.getErr
	}
	p, ok := f.items[id]
	return p, ok, nil
}

func (f *fakeCache) SetProduct(_ context.Context, p product.Product) error {
	f.sets++
	if f.setErr != nil {
		return f.setErr
	}
	f.items[p.ID] = p
	return nil
}

func (f *fakeCache) Ping(context.Context) error { return f.pingErr }

func serve(t *testing.T, store *fakeStore, cache *fakeCache, target string) *httptest.ResponseRecorder {
	t.Helper()
	h := New(store, cache, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestGetProduct(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		store          *fakeStore
		cache          *fakeCache
		wantStatus     int
		wantCache      string
		wantStoreCalls int
		wantCached     bool // product 1 is in the cache after the request
	}{
		{
			name:  "cache hit skips the store",
			path:  "/products/1",
			store: &fakeStore{products: []product.Product{shoe}},
			cache: newFakeCache(shoe), wantStatus: http.StatusOK, wantCache: "HIT", wantStoreCalls: 0, wantCached: true,
		},
		{
			name:  "cache miss reads the store and fills the cache",
			path:  "/products/1",
			store: &fakeStore{products: []product.Product{shoe}},
			cache: newFakeCache(), wantStatus: http.StatusOK, wantCache: "MISS", wantStoreCalls: 1, wantCached: true,
		},
		{
			name:  "unknown product is 404",
			path:  "/products/99999",
			store: &fakeStore{products: []product.Product{shoe}},
			cache: newFakeCache(), wantStatus: http.StatusNotFound, wantCache: "MISS", wantStoreCalls: 1,
		},
		{
			name:  "non-numeric id is 400",
			path:  "/products/abc",
			store: &fakeStore{}, cache: newFakeCache(), wantStatus: http.StatusBadRequest,
		},
		{
			name:  "zero id is 400",
			path:  "/products/0",
			store: &fakeStore{}, cache: newFakeCache(), wantStatus: http.StatusBadRequest,
		},
		{
			name:  "store failure is 500",
			path:  "/products/1",
			store: &fakeStore{err: errDown},
			cache: newFakeCache(), wantStatus: http.StatusInternalServerError, wantCache: "MISS", wantStoreCalls: 1,
		},
		{
			name:       "redis down falls back to the store",
			path:       "/products/1",
			store:      &fakeStore{products: []product.Product{shoe}},
			cache:      &fakeCache{items: map[int64]product.Product{}, getErr: errDown, setErr: errDown},
			wantStatus: http.StatusOK, wantCache: "MISS", wantStoreCalls: 1,
		},
		{
			name:  "postgres down still serves cached products",
			path:  "/products/1",
			store: &fakeStore{err: errDown},
			cache: newFakeCache(shoe), wantStatus: http.StatusOK, wantCache: "HIT", wantStoreCalls: 0, wantCached: true,
		},
	}

	t.Run("failed cache read skips the cache write", func(t *testing.T) {
		cache := &fakeCache{items: map[int64]product.Product{}, getErr: errDown}
		rec := serve(t, &fakeStore{products: []product.Product{shoe}}, cache, "/products/1")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
		if cache.sets != 0 {
			t.Errorf("cache writes = %d, want 0 while Redis is failing", cache.sets)
		}
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, tt.store, tt.cache, tt.path)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if got := rec.Header().Get("X-Cache"); got != tt.wantCache {
				t.Errorf("X-Cache = %q, want %q", got, tt.wantCache)
			}
			if tt.store.calls != tt.wantStoreCalls {
				t.Errorf("store calls = %d, want %d", tt.store.calls, tt.wantStoreCalls)
			}
			if _, ok := tt.cache.items[1]; ok != tt.wantCached {
				t.Errorf("product cached = %v, want %v", ok, tt.wantCached)
			}
			if tt.wantStatus == http.StatusOK {
				var got product.Product
				if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if got != shoe {
					t.Errorf("body = %+v, want %+v", got, shoe)
				}
			}
		})
	}
}

func TestListProducts(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		store      *fakeStore
		wantStatus int
		wantCount  int
	}{
		{name: "default limit", query: "", store: &fakeStore{products: []product.Product{shoe, shirt}}, wantStatus: http.StatusOK, wantCount: 2},
		{name: "explicit limit", query: "?limit=1", store: &fakeStore{products: []product.Product{shoe, shirt}}, wantStatus: http.StatusOK, wantCount: 1},
		{name: "empty catalog is an empty list", query: "", store: &fakeStore{}, wantStatus: http.StatusOK, wantCount: 0},
		{name: "limit 0 is 400", query: "?limit=0", store: &fakeStore{}, wantStatus: http.StatusBadRequest},
		{name: "limit above max is 400", query: "?limit=101", store: &fakeStore{}, wantStatus: http.StatusBadRequest},
		{name: "non-numeric limit is 400", query: "?limit=all", store: &fakeStore{}, wantStatus: http.StatusBadRequest},
		{name: "store failure is 500", query: "", store: &fakeStore{err: errDown}, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, tt.store, newFakeCache(), "/products"+tt.query)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var body struct {
				Products []product.Product `json:"products"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.Products == nil {
				t.Error("products is null, want a JSON array")
			}
			if len(body.Products) != tt.wantCount {
				t.Errorf("got %d products, want %d", len(body.Products), tt.wantCount)
			}
		})
	}
}

func TestProbes(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		store      *fakeStore
		cache      *fakeCache
		wantStatus int
		wantBody   map[string]string
	}{
		{
			name: "liveness ignores dependencies", path: "/healthz",
			store: &fakeStore{pingErr: errDown}, cache: &fakeCache{pingErr: errDown},
			wantStatus: http.StatusOK, wantBody: map[string]string{"status": "ok"},
		},
		{
			name: "ready when everything is up", path: "/readyz",
			store: &fakeStore{}, cache: newFakeCache(),
			wantStatus: http.StatusOK, wantBody: map[string]string{"postgres": "ok", "redis": "ok"},
		},
		{
			name: "not ready when postgres is down", path: "/readyz",
			store: &fakeStore{pingErr: errDown}, cache: newFakeCache(),
			wantStatus: http.StatusServiceUnavailable, wantBody: map[string]string{"postgres": "down", "redis": "ok"},
		},
		{
			name: "still ready but degraded when redis is down", path: "/readyz",
			store: &fakeStore{}, cache: &fakeCache{pingErr: errDown},
			wantStatus: http.StatusOK, wantBody: map[string]string{"postgres": "ok", "redis": "degraded"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serve(t, tt.store, tt.cache, tt.path)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var got map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			for k, want := range tt.wantBody {
				if got[k] != want {
					t.Errorf("%s = %q, want %q", k, got[k], want)
				}
			}
		})
	}
}
