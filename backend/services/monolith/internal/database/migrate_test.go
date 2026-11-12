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

	func TestSystemMonitorAccessMigrationIsRegisteredAfterExistingMigrations(t *testing.T) {
		const version = "015_system_monitor_access"
		const path = "migrations/015_system_monitor_access.sql"

		previousIndex := -1
		index := -1
		for i, item := range allMigrations {
			if item.version == "014_siwes_email_outbox" {
				previousIndex = i
			}
			if item.version == version && item.path == path {
				index = i
			}
		}
		if index < 0 || previousIndex < 0 || index <= previousIndex {
			t.Fatalf("migration %q must be registered after 014_siwes_email_outbox", version)
		}

		script, err := migrations.ReadFile(path)
		if err != nil {
			t.Fatalf("read embedded migration %q: %v", version, err)
		}
		for _, expected := range []string{
			"CREATE SCHEMA IF NOT EXISTS system_monitor",
			"CREATE TABLE system_monitor.access_grants",
			"granted_by varchar(200) NOT NULL",
			"expires_at timestamptz NOT NULL",
			"revoked_at timestamptz",
			"WHERE revoked_at IS NULL",
		} {
			if !strings.Contains(string(script), expected) {
				t.Errorf("migration %q does not contain %q", version, expected)
			}
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
