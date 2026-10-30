package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	directory := flag.String("dir", "migrations", "directory containing Access SQL migrations")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(*directory, logger); err != nil {
		logger.Error("Access migration failed", "error", err)
		os.Exit(1)
	}
}

func run(directory string, logger *slog.Logger) error {
	databaseURL, err := migrationDatabaseURL()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("create Access migration database pool")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to Access migration database: %w", err)
	}
	migrations, err := loadMigrations(directory)
	if err != nil {
		return err
	}
	return applyMigrations(ctx, pool, migrations, logger)
}

func migrationDatabaseURL() (string, error) {
	appEnv := strings.TrimSpace(os.Getenv("APP_ENV"))
	configured := strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URL"))
	if configured == "" {
		if appEnv == "production" {
			return "", errors.New("production migrations require a separate MIGRATION_DATABASE_URL")
		}
		configured = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if configured == "" {
		return "", errors.New("set MIGRATION_DATABASE_URL or DATABASE_URL")
	}
	parsed, err := url.Parse(configured)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", errors.New("migration database URL must be a PostgreSQL URL")
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
	if len(modes) != 1 {
		return "", errors.New("migration database URL must contain exactly one sslmode")
	}
	switch modes[0] {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return "", errors.New("migration database URL has an invalid sslmode")
	}
	if appEnv == "production" && modes[0] != "verify-full" {
		allowUnverified, parseErr := strconv.ParseBool(strings.TrimSpace(os.Getenv("DATABASE_ALLOW_UNVERIFIED_TLS")))
		if modes[0] != "require" || parseErr != nil || !allowUnverified {
			return "", errors.New("production migrations require sslmode=verify-full, or sslmode=require with DATABASE_ALLOW_UNVERIFIED_TLS=true")
		}
	}
	return parsed.String(), nil
}
