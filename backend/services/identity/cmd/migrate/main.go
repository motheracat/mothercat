package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	migrationDir := flag.String("dir", "migrations", "directory containing numbered Identity SQL migrations")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(*migrationDir, logger); err != nil {
		logger.Error("identity migration failed", "error", err)
		os.Exit(1)
	}
}

func run(migrationDir string, logger *slog.Logger) error {
	migrations, err := loadMigrations(migrationDir)
	if err != nil {
		return err
	}
	databaseURL, err := migrationDatabaseURL()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(connectCtx, databaseURL)
	if err != nil {
		return fmt.Errorf("create migration database pool: %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(connectCtx); err != nil {
		return fmt.Errorf("connect to migration database: %w", err)
	}
	return applyMigrations(ctx, pool, migrations, logger)
}

func migrationDatabaseURL() (string, error) {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	configured := strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URL"))
	if configured == "" {
		if appEnv == "production" {
			return "", errors.New("production migrations require a separate MIGRATION_DATABASE_URL")
		}
		configured = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if configured != "" {
		parsed, err := url.Parse(configured)
		if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.Path == "" || parsed.User == nil {
			return "", errors.New("migration database URL must be a valid PostgreSQL URL with a database and username")
		}
		if password, ok := parsed.User.Password(); !ok || password == "" {
			return "", errors.New("migration database URL must include a password")
		}
		query := parsed.Query()
		modes := query["sslmode"]
		if len(modes) == 0 {
			mode := strings.TrimSpace(os.Getenv("DATABASE_SSLMODE"))
			if mode == "" {
				return "", errors.New("set DATABASE_SSLMODE when the migration URL omits sslmode")
			}
			query.Set("sslmode", mode)
			parsed.RawQuery = query.Encode()
			modes = query["sslmode"]
		}
		if len(modes) != 1 || !validSSLMode(modes[0]) {
			return "", errors.New("migration database URL must contain one valid sslmode")
		}
		if appEnv == "production" && modes[0] != "verify-full" {
			return "", errors.New("production migrations require sslmode=verify-full")
		}
		if appEnv == "staging" && modes[0] != "require" && modes[0] != "verify-ca" && modes[0] != "verify-full" {
			return "", errors.New("staging migrations require TLS")
		}
		return parsed.String(), nil
	}

	database := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
	username := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
	password := os.Getenv("POSTGRES_PASSWORD")
	if database == "" || username == "" || password == "" {
		return "", errors.New("set MIGRATION_DATABASE_URL or PostgreSQL connection settings")
	}
	host := strings.TrimSpace(env("POSTGRES_HOST", "localhost"))
	port := strings.TrimSpace(env("POSTGRES_PORT", "5432"))
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return "", errors.New("POSTGRES_PORT must be a valid TCP port")
	}
	sslDefault := "disable"
	if appEnv == "production" {
		sslDefault = "verify-full"
	} else if appEnv == "staging" {
		sslDefault = "require"
	}
	sslMode := strings.TrimSpace(env("POSTGRES_SSLMODE", sslDefault))
	if !validSSLMode(sslMode) || (appEnv == "production" && sslMode != "verify-full") || (appEnv == "staging" && sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full") {
		return "", errors.New("PostgreSQL TLS mode is not allowed for this environment")
	}
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(username, password), Host: net.JoinHostPort(host, port), Path: "/" + database}
	query := url.Values{}
	query.Set("sslmode", sslMode)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func validSSLMode(mode string) bool {
	switch mode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
		return true
	default:
		return false
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
