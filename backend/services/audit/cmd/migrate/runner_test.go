package main

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrationDatabaseURLAddsConfiguredSSLMode(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_SSLMODE", "require")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "true")
	t.Setenv("MIGRATION_DATABASE_URL", "postgres://admin:secret@db.example.test/mothercat")

	got, err := migrationDatabaseURL()
	if err != nil {
		t.Fatalf("migrationDatabaseURL() error = %v", err)
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}
	if modes := parsed.Query()["sslmode"]; len(modes) != 1 || modes[0] != "require" {
		t.Fatalf("sslmode = %v, want [require]", modes)
	}
}

func TestLoadMigrationsLoadsAuditMigration(t *testing.T) {
	migrations, err := loadMigrations("../../migrations")
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	if len(migrations) != 1 || migrations[0].version != 1 ||
		migrations[0].name != "001_initial_admin_audit.sql" || len(migrations[0].checksum) != 64 {
		t.Fatalf("unexpected migrations: %+v", migrations)
	}
}

func TestLoadMigrationsRejectsVersionGaps(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"001_first.sql", "003_third.sql"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatalf("write migration fixture: %v", err)
		}
	}
	if _, err := loadMigrations(directory); err == nil {
		t.Fatal("loadMigrations() accepted a migration version gap")
	}
}
