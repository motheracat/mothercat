package main

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigrationsOrdersAndChecksums(t *testing.T) {
	directory := t.TempDir()
	for name, contents := range map[string]string{
		"002_second.sql": "SELECT 2;\n",
		"001_first.sql":  "SELECT 1;\n",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("write migration %s: %v", name, err)
		}
	}
	migrations, err := loadMigrations(directory)
	if err != nil {
		t.Fatalf("loadMigrations returned error: %v", err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("unexpected migration order: %#v", migrations)
	}
	if migrations[0].Checksum == "" || migrations[0].Checksum == migrations[1].Checksum {
		t.Fatalf("unexpected checksums: %#v", migrations)
	}
}

func TestLoadMigrationsRejectsVersionGaps(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"001_first.sql", "003_third.sql"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("SELECT 1;\n"), 0o600); err != nil {
			t.Fatalf("write migration %s: %v", name, err)
		}
	}
	if _, err := loadMigrations(directory); err == nil {
		t.Fatal("loadMigrations accepted a migration version gap")
	}
}

func TestMigrationDatabaseURLAddsRenderStagingTLS(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://identity:secret@db.example.test:5432/identity")
	t.Setenv("DATABASE_SSLMODE", "require")

	connectionString, err := migrationDatabaseURL()
	if err != nil {
		t.Fatalf("migrationDatabaseURL returned error: %v", err)
	}
	parsed, err := url.Parse(connectionString)
	if err != nil {
		t.Fatalf("parse migration URL: %v", err)
	}
	if got := parsed.Query().Get("sslmode"); got != "require" {
		t.Fatalf("sslmode = %q, want require", got)
	}
}

func TestMigrationDatabaseURLRejectsPlaintextStagingTLS(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	t.Setenv("MIGRATION_DATABASE_URL", "postgresql://identity:secret@db.example.test:5432/identity?sslmode=disable")
	t.Setenv("DATABASE_SSLMODE", "require")

	if _, err := migrationDatabaseURL(); err == nil {
		t.Fatal("migrationDatabaseURL accepted plaintext staging connection")
	}
}

func TestMigrationDatabaseURLAllowsRenderPrivateNetworkTLSInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("MIGRATION_DATABASE_URL", "postgres://identity:unit-test-password@db.example.test:5432/identity?sslmode=require")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "true")

	if _, err := migrationDatabaseURL(); err != nil {
		t.Fatalf("migrationDatabaseURL returned error: %v", err)
	}
}

func TestMigrationDatabaseURLRequiresOptInForUnverifiedProductionTLS(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("MIGRATION_DATABASE_URL", "postgres://identity:unit-test-password@db.example.test:5432/identity?sslmode=require")
	t.Setenv("DATABASE_ALLOW_UNVERIFIED_TLS", "")

	if _, err := migrationDatabaseURL(); err == nil {
		t.Fatal("migrationDatabaseURL accepted unverified production TLS without opt-in")
	}
}
