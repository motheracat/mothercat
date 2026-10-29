package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const migrationLockKey int64 = 0x4d4f5448434154
const maximumMigrationBytes = 10 << 20

var migrationNamePattern = regexp.MustCompile(`^([0-9]+)_[a-z0-9_]+\.sql$`)

type migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum string
}

func loadMigrations(directory string) ([]migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		matches := migrationNamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			return nil, fmt.Errorf("invalid migration filename %q; expected NNN_description.sql", entry.Name())
		}
		version, err := strconv.ParseInt(matches[1], 10, 64)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect migration %q: %w", entry.Name(), err)
		}
		if info.Size() > maximumMigrationBytes {
			return nil, fmt.Errorf("migration %q exceeds the %d-byte limit", entry.Name(), maximumMigrationBytes)
		}
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("migration %q is empty", entry.Name())
		}
		digest := sha256.Sum256(contents)
		migrations = append(migrations, migration{Version: version, Name: entry.Name(), SQL: string(contents), Checksum: hex.EncodeToString(digest[:])})
	}
	if len(migrations) == 0 {
		return nil, errors.New("no numbered SQL migrations found")
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	for i, item := range migrations {
		if item.Version != int64(i+1) {
			return nil, fmt.Errorf("migration sequence gap at version %d", i+1)
		}
	}
	return migrations, nil
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool, migrations []migration, logger *slog.Logger) error {
	if pool == nil {
		return errors.New("migration database pool is nil")
	}
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()
	if _, err := connection.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS identity"); err != nil {
		return fmt.Errorf("create Identity schema: %w", err)
	}
	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS identity.schema_migrations (
		version BIGINT PRIMARY KEY,
		name TEXT NOT NULL,
		checksum CHAR(64) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("initialize migration history: %w", err)
	}

	for _, item := range migrations {
		var checksum string
		err := connection.QueryRow(ctx, "SELECT checksum FROM identity.schema_migrations WHERE version = $1", item.Version).Scan(&checksum)
		if err == nil {
			if strings.TrimSpace(checksum) != item.Checksum {
				return fmt.Errorf("migration %d checksum differs from the applied version", item.Version)
			}
			logger.InfoContext(ctx, "migration already applied", "version", item.Version, "name", item.Name)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read migration history for version %d: %w", item.Version, err)
		}
		tx, err := connection.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", item.Version, err)
		}
		for _, statement := range strings.Split(item.SQL, ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := tx.Exec(ctx, statement); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("execute migration %d (%s): %w", item.Version, item.Name, err)
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO identity.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)", item.Version, item.Name, item.Checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", item.Version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", item.Version, err)
		}
		logger.InfoContext(ctx, "migration applied", "version", item.Version, "name", item.Name)
	}
	return nil
}
