package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockKey int64 = 0x4f5247414e495a

var migrationFilename = regexp.MustCompile(`^([0-9]+)_[a-z0-9_]+\.sql$`)

type migration struct {
	version  int64
	name     string
	sql      string
	checksum string
}

func main() {
	directory := flag.String("dir", "migrations", "directory containing numbered Organization SQL migrations")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(*directory, logger); err != nil {
		logger.Error("Organization migration failed", "error", err)
		os.Exit(1)
	}
}

func run(directory string, logger *slog.Logger) error {
	migrations, err := loadMigrations(directory)
	if err != nil {
		return err
	}
	databaseURL, err := migrationDatabaseURL()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("create Organization migration pool")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("connect to Organization migration database: %w", err)
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire Organization migration connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire Organization migration lock: %w", err)
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()
	if _, err := connection.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS organization"); err != nil {
		return fmt.Errorf("create Organization schema: %w", err)
	}
	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS organization.schema_migrations (
		version BIGINT PRIMARY KEY,
		name TEXT NOT NULL,
		checksum CHAR(64) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("initialize Organization migration history: %w", err)
	}
	for _, item := range migrations {
		var appliedChecksum string
		err := connection.QueryRow(ctx, "SELECT checksum FROM organization.schema_migrations WHERE version = $1", item.version).Scan(&appliedChecksum)
		if err == nil {
			if strings.TrimSpace(appliedChecksum) != item.checksum {
				return fmt.Errorf("Organization migration %d checksum differs from the applied version", item.version)
			}
			logger.InfoContext(ctx, "Organization migration already applied", "version", item.version, "name", item.name)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read Organization migration history: %w", err)
		}
		tx, err := connection.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin Organization migration %d: %w", item.version, err)
		}
		for _, statement := range strings.Split(item.sql, ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := tx.Exec(ctx, statement); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("execute Organization migration %s: %w", item.name, err)
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO organization.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)", item.version, item.name, item.checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record Organization migration %d: %w", item.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit Organization migration %d: %w", item.version, err)
		}
		logger.InfoContext(ctx, "Organization migration applied", "version", item.version, "name", item.name)
	}
	return nil
}

func loadMigrations(directory string) ([]migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read Organization migrations: %w", err)
	}
	result := make([]migration, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		matches := migrationFilename.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("invalid Organization migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid Organization migration version in %q", entry.Name())
		}
		content, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read Organization migration %q: %w", entry.Name(), err)
		}
		if len(content) == 0 || len(content) > 10<<20 {
			return nil, fmt.Errorf("Organization migration %q is empty or too large", entry.Name())
		}
		checksum := sha256.Sum256(content)
		result = append(result, migration{version: version, name: entry.Name(), sql: string(content), checksum: hex.EncodeToString(checksum[:])})
	}
	if len(result) == 0 {
		return nil, errors.New("no numbered Organization migrations found")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	for index, item := range result {
		if item.version != int64(index+1) {
			return nil, fmt.Errorf("Organization migration sequence gap at version %d", index+1)
		}
	}
	return result, nil
}

func migrationDatabaseURL() (string, error) {
	appEnv := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	raw := strings.TrimSpace(os.Getenv("MIGRATION_DATABASE_URL"))
	if raw == "" {
		if appEnv == "production" {
			return "", errors.New("production migrations require a separate MIGRATION_DATABASE_URL")
		}
		raw = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") ||
		parsed.Hostname() == "" || parsed.User == nil || parsed.Path == "" {
		return "", errors.New("migration database URL must be a PostgreSQL URL with host, user, and database")
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
