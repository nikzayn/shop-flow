// Command catalog serves the ShopFlow product catalog.
//
// Configuration (environment variables):
//
//	DATABASE_URL  required  postgres://user:pass@host:5432/db
//	REDIS_URL     required  redis://host:6379/0
//	CACHE_TTL     optional  Go duration, default 5m
//	PORT          optional  default 8080
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nikzayn/shop-flow/services/catalog/internal/api"
	"github.com/nikzayn/shop-flow/services/catalog/internal/cache"
	"github.com/nikzayn/shop-flow/services/catalog/internal/store"
)

// shutdownTimeout must stay below the pod's terminationGracePeriodSeconds (30s by default).
const shutdownTimeout = 10 * time.Second

type config struct {
	port        string
	databaseURL string
	redisURL    string
	cacheTTL    time.Duration
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("catalog exited", "err", err)
		os.Exit(1)
	}
}

func loadConfig() (config, error) {
	cfg := config{
		port:        envOr("PORT", "8080"),
		databaseURL: os.Getenv("DATABASE_URL"),
		redisURL:    os.Getenv("REDIS_URL"),
	}
	if cfg.databaseURL == "" {
		return cfg, errors.New("DATABASE_URL is required")
	}
	if cfg.redisURL == "" {
		return cfg, errors.New("REDIS_URL is required")
	}
	ttl, err := time.ParseDuration(envOr("CACHE_TTL", "5m"))
	if err != nil {
		return cfg, fmt.Errorf("invalid CACHE_TTL: %w", err)
	}
	cfg.cacheTTL = ttl
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.NewPostgres(ctx, cfg.databaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	rc, err := cache.NewRedis(cfg.redisURL, cfg.cacheTTL)
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	srv := &http.Server{
		Addr:              ":" + cfg.port,
		Handler:           api.New(db, rc, logger).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("catalog listening", "addr", srv.Addr, "cache_ttl", cfg.cacheTTL.String())
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}

	// Stop accepting new connections and let in-flight requests finish.
	logger.Info("shutting down", "timeout", shutdownTimeout.String())
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("shutdown complete")
	return nil
}
