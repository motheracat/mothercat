package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
