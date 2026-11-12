package database

import (
	"strings"
	"testing"
)

func TestAdminTaskStatusMigrationIsRegisteredAndAddsQueriedColumns(t *testing.T) {
	const version = "013_admin_task_status"
	const path = "migrations/013_admin_task_status.sql"

	previousIndex := -1
	index := -1
	for i, item := range allMigrations {
		if item.version == "012_applicant_identity" {
			previousIndex = i
		}
		if item.version == version && item.path == path {
			index = i
		}
	}
	if index < 0 {
		t.Fatalf("migration %q at %q is not registered", version, path)
	}
	if previousIndex < 0 || index <= previousIndex {
		t.Fatalf("migration %q must be registered after migration 012_applicant_identity", version)
	}

	script, err := migrations.ReadFile(path)
	if err != nil {
		t.Fatalf("read embedded migration %q: %v", version, err)
	}
	for _, column := range []string{
		"ADD COLUMN admin_status",
		"ADD COLUMN admin_status_updated_by",
		"ADD COLUMN admin_status_updated_at",
	} {
		if !strings.Contains(string(script), column) {
			t.Errorf("migration %q does not contain %q", version, column)
		}
	}
}

func TestSiwesEmailOutboxMigrationIsRegisteredAfterApplicationSchema(t *testing.T) {
	const version = "014_siwes_email_outbox"
	const path = "migrations/014_siwes_email_outbox.sql"

	previousIndex := -1
	index := -1
	for i, item := range allMigrations {
		if item.version == "013_admin_task_status" {
			previousIndex = i
		}
		if item.version == version && item.path == path {
			index = i
		}
	}
	if index < 0 {
		t.Fatalf("migration %q at %q is not registered", version, path)
	}
	if previousIndex < 0 || index <= previousIndex {
		t.Fatalf("migration %q must be registered after migration 013_admin_task_status", version)
	}

	script, err := migrations.ReadFile(path)
	if err != nil {
		t.Fatalf("read embedded migration %q: %v", version, err)
	}
	for _, column := range []string{
		"application_id uuid NOT NULL",
		"status varchar(24) NOT NULL",
		"available_at timestamptz NOT NULL",
		"processed_at timestamptz",
		"UNIQUE (application_id, status)",
	} {
		if !strings.Contains(string(script), column) {
			t.Errorf("migration %q does not contain %q", version, column)
		}
	}
}
