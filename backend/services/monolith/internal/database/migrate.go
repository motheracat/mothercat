package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
)

//go:embed migrations/*.sql
var migrations embed.FS

type migration struct {
	version string
	path    string
}

var allMigrations = []migration{
	{version: "001_initial", path: "migrations/001_initial.sql"},
	{version: "002_account_suspension", path: "migrations/002_account_suspension.sql"},
}

func Migrate(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return errors.New("PostgreSQL database is unavailable")
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create schema migration table: %w", err)
	}
	for _, item := range allMigrations {
		if err := applyMigration(ctx, db, item); err != nil {
			return err
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, item migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", item.version, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(761039122)`); err != nil {
		return fmt.Errorf("lock schema migration %s: %w", item.version, err)
	}
	var applied bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM public.schema_migrations WHERE version = $1)`, item.version,
	).Scan(&applied); err != nil {
		return fmt.Errorf("check schema migration %s: %w", item.version, err)
	}
	if applied {
		return tx.Commit()
	}
	script, err := migrations.ReadFile(item.path)
	if err != nil {
		return fmt.Errorf("read schema migration %s: %w", item.version, err)
	}
	if _, err := tx.ExecContext(ctx, string(script)); err != nil {
		return fmt.Errorf("apply schema migration %s: %w", item.version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO public.schema_migrations (version) VALUES ($1)`, item.version,
	); err != nil {
		return fmt.Errorf("record schema migration %s: %w", item.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit schema migration %s: %w", item.version, err)
	}
	return nil
}
