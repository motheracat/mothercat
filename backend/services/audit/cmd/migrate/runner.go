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

const migrationLockKey int64 = 0x4155444954

var migrationPattern = regexp.MustCompile(`^([0-9]+)_[a-z0-9_]+\.sql$`)

type migration struct {
	version  int64
	name     string
	sql      string
	checksum string
}

func loadMigrations(directory string) ([]migration, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("read Audit migrations directory: %w", err)
	}
	var migrations []migration
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := migrationPattern.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("invalid Audit migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(match[1], 10, 64)
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid Audit migration version in %q", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("inspect Audit migration %q: %w", entry.Name(), err)
		}
		if info.Size() > 10<<20 {
			return nil, fmt.Errorf("Audit migration %q exceeds the size limit", entry.Name())
		}
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read Audit migration %q: %w", entry.Name(), err)
		}
		if strings.TrimSpace(string(contents)) == "" {
			return nil, fmt.Errorf("Audit migration %q is empty", entry.Name())
		}
		sum := sha256.Sum256(contents)
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(contents), checksum: hex.EncodeToString(sum[:])})
	}
	if len(migrations) == 0 {
		return nil, errors.New("no numbered Audit migrations found")
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	for i, item := range migrations {
		if item.version != int64(i+1) {
			return nil, fmt.Errorf("Audit migration sequence gap at version %d", i+1)
		}
	}
	return migrations, nil
}

func applyMigrations(ctx context.Context, pool *pgxpool.Pool, migrations []migration, logger *slog.Logger) error {
	connection, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire Audit migration connection: %w", err)
	}
	defer connection.Release()
	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("acquire Audit migration lock: %w", err)
	}
	defer func() {
		_, _ = connection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	if _, err := connection.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS audit"); err != nil {
		return fmt.Errorf("create Audit schema: %w", err)
	}
	if _, err := connection.Exec(ctx, `CREATE TABLE IF NOT EXISTS audit.schema_migrations (
		version BIGINT PRIMARY KEY, name TEXT NOT NULL, checksum CHAR(64) NOT NULL,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("initialize Audit migration history: %w", err)
	}
	for _, item := range migrations {
		var checksum string
		err := connection.QueryRow(ctx, "SELECT checksum FROM audit.schema_migrations WHERE version = $1", item.version).Scan(&checksum)
		if err == nil {
			if strings.TrimSpace(checksum) != item.checksum {
				return fmt.Errorf("Audit migration %d checksum differs from applied version", item.version)
			}
			logger.InfoContext(ctx, "Audit migration already applied", "version", item.version, "name", item.name)
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("read Audit migration history: %w", err)
		}
		tx, err := connection.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin Audit migration %d: %w", item.version, err)
		}
		for _, statement := range strings.Split(item.sql, ";") {
			statement = strings.TrimSpace(statement)
			if statement == "" {
				continue
			}
			if _, err := tx.Exec(ctx, statement); err != nil {
				_ = tx.Rollback(ctx)
				return fmt.Errorf("execute Audit migration %d (%s): %w", item.version, item.name, err)
			}
		}
		if _, err := tx.Exec(ctx, "INSERT INTO audit.schema_migrations (version, name, checksum) VALUES ($1, $2, $3)", item.version, item.name, item.checksum); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record Audit migration %d: %w", item.version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit Audit migration %d: %w", item.version, err)
		}
		logger.InfoContext(ctx, "Audit migration applied", "version", item.version, "name", item.name)
	}
	return nil
}
