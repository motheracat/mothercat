package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	accesshttp "github.com/mothercat-enterprise/mothercat/backend/services/access/internal/http"
	"github.com/mothercat-enterprise/mothercat/backend/services/access/internal/repository"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", "access")
	if err := run(logger); err != nil {
		logger.Error("Access service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	appEnv := strings.ToLower(strings.TrimSpace(env("APP_ENV", "development")))
	if appEnv != "development" && appEnv != "test" && appEnv != "staging" && appEnv != "production" {
		return errors.New("APP_ENV must be development, test, staging, or production")
	}
	databaseURL, err := configuredDatabaseURL(appEnv)
	if err != nil {
		return err
	}
	serviceToken := strings.TrimSpace(os.Getenv("ACCESS_SERVICE_TOKEN"))
	if len(serviceToken) < 32 || serviceToken != os.Getenv("ACCESS_SERVICE_TOKEN") {
		return errors.New("ACCESS_SERVICE_TOKEN must contain at least 32 characters")
	}
	address, err := listenAddress()
	if err != nil {
		return err
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return errors.New("invalid Access database configuration")
	}
	poolConfig.MaxConns = 10
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.ConnConfig.ConnectTimeout = 10 * time.Second
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return fmt.Errorf("create Access database pool: %w", err)
	}
	defer pool.Close()

	store := repository.NewAuthorizationRepository(pool)
	server := &http.Server{
		Addr: address, Handler: accesshttp.NewHandler(store, serviceToken, logger),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	logger.Info("Access service listening", "address", address)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("graceful Access HTTP shutdown: %w", err)
		}
		return nil
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("Access HTTP server: %w", err)
	}
}

func configuredDatabaseURL(appEnv string) (string, error) {
	raw := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil || parsed.Path == "" {
		return "", errors.New("DATABASE_URL must be a PostgreSQL URL with host, user, and database")
	}
	if password, ok := parsed.User.Password(); !ok || password == "" {
		return "", errors.New("DATABASE_URL must include a password")
	}
	query := parsed.Query()
	modes := query["sslmode"]
	if len(modes) == 0 {
		mode := strings.TrimSpace(os.Getenv("DATABASE_SSLMODE"))
		if mode == "" {
			return "", errors.New("set DATABASE_SSLMODE when DATABASE_URL omits sslmode")
		}
		query.Set("sslmode", mode)
		parsed.RawQuery = query.Encode()
		modes = query["sslmode"]
	}
	if len(modes) != 1 {
		return "", errors.New("DATABASE_URL must contain exactly one sslmode")
	}
	switch modes[0] {
	case "require", "verify-ca", "verify-full":
	case "disable", "allow", "prefer":
		if appEnv == "production" {
			return "", errors.New("production Access database connections require TLS")
		}
	default:
		return "", errors.New("DATABASE_URL has an invalid sslmode")
	}
	if appEnv == "production" && modes[0] == "require" {
		allowUnverified, parseErr := strconv.ParseBool(strings.TrimSpace(os.Getenv("DATABASE_ALLOW_UNVERIFIED_TLS")))
		if parseErr != nil || !allowUnverified {
			return "", errors.New("production sslmode=require requires DATABASE_ALLOW_UNVERIFIED_TLS=true")
		}
	}
	return parsed.String(), nil
}

func listenAddress() (string, error) {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = strings.TrimSpace(env("HTTP_ADDR", ":8080"))
		if _, _, err := net.SplitHostPort(port); err != nil {
			return "", errors.New("HTTP_ADDR must be a valid host:port address")
		}
		return port, nil
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("PORT must be between 1 and 65535")
	}
	return ":" + port, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
