package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/application"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/config"
	identityhttp "github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/http"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/observability"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/internal/repository"
)

func main() {
	logger := observability.NewLogger().With("service", "identity")
	if err := run(logger); err != nil {
		logger.Error("identity service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	pool, err := repository.NewPool(context.Background(), cfg)
	if err != nil {
		return fmt.Errorf("initialize PostgreSQL connection pool: %w", err)
	}
	defer pool.Close()

	authRepo := repository.NewAccountRepository(pool)
	authService := application.NewService(authRepo)
	baseHandler := identityhttp.NewHandler(pool, logger, cfg.HealthCheckTimeout)
	authHandler := identityhttp.NewAuthHandler(authService, logger)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/v1/") {
			authHandler.ServeHTTP(w, r)
			return
		}
		baseHandler.ServeHTTP(w, r)
	})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	logger.Info("identity service listening", "address", cfg.HTTPAddr)
	select {
	case <-shutdownCtx.Done():
		logger.Info("shutdown signal received")
		ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			_ = server.Close()
			return fmt.Errorf("graceful HTTP shutdown: %w", err)
		}
		return nil
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP server: %w", err)
	}
}
