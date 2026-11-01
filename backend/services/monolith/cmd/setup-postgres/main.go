package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/mothercat-enterprise/mothercat/backend/services/identity/monolith/internal/database"
)

func main() {
	if err := run(); err != nil {
		slog.Error("PostgreSQL migration failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := strings.TrimSpace(os.Getenv("MIGRATIONS_DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("MIGRATIONS_DATABASE_URL is required for controlled schema migrations")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil {
		return errors.New("MIGRATIONS_DATABASE_URL must be a valid PostgreSQL connection URL")
	}
	sslMode := parsed.Query().Get("sslmode")
	if sslMode != "require" && sslMode != "verify-ca" && sslMode != "verify-full" {
		return errors.New("MIGRATIONS_DATABASE_URL must require TLS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("configure PostgreSQL migration connection")
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("close PostgreSQL migration connection", "error", err)
		}
	}()
	if err := db.PingContext(ctx); err != nil {
		return errors.New("connect to PostgreSQL for migrations")
	}
	if err := database.Migrate(ctx, db); err != nil {
		return err
	}
	slog.Info("PostgreSQL schemas and baseline Access permissions are ready")
	return nil
}
