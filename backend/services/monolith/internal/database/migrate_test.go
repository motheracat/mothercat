package database

import (
	"strings"
	"testing"
)

func TestAdminTaskStatusMigrationIsRegisteredAndAddsQueriedColumns(t *testing.T) {
	assertMigrationRegisteredAfter(t, "013_admin_task_status", "migrations/013_admin_task_status.sql",
		"012_applicant_identity", []string{
			"ADD COLUMN admin_status",
			"ADD COLUMN admin_status_updated_by",
			"ADD COLUMN admin_status_updated_at",
		})
}

func TestSiwesEmailOutboxMigrationIsRegisteredAfterApplicationSchema(t *testing.T) {
	assertMigrationRegisteredAfter(t, "014_siwes_email_outbox", "migrations/014_siwes_email_outbox.sql",
		"013_admin_task_status", []string{
			"application_id uuid NOT NULL",
			"status varchar(24) NOT NULL",
			"available_at timestamptz NOT NULL",
			"processed_at timestamptz",
			"UNIQUE (application_id, status)",
		})
}

func TestSystemMonitorAccessMigrationIsRegisteredAfterExistingMigrations(t *testing.T) {
	assertMigrationRegisteredAfter(t, "015_system_monitor_access", "migrations/015_system_monitor_access.sql",
		"014_siwes_email_outbox", []string{
			"CREATE SCHEMA IF NOT EXISTS system_monitor",
			"CREATE TABLE system_monitor.access_grants",
			"granted_by varchar(200) NOT NULL",
			"expires_at timestamptz NOT NULL",
			"revoked_at timestamptz",
		})
}

func TestSystemMonitorEndpointControlsMigrationIsRegisteredAfterAccessGrants(t *testing.T) {
	assertMigrationRegisteredAfter(t, "016_system_monitor_endpoint_controls",
		"migrations/016_system_monitor_endpoint_controls.sql", "015_system_monitor_access", []string{
			"CREATE TABLE system_monitor.endpoint_controls",
			"PRIMARY KEY (method, route)",
			"response_status integer NOT NULL DEFAULT 503",
			"expires_at timestamptz NOT NULL",
			"CREATE TABLE system_monitor.endpoint_control_history",
			"action IN ('paused', 'resumed', 'expired')",
			"actor_type IN ('staff', 'system')",
		})
}

func assertMigrationRegisteredAfter(t *testing.T, version, path, previousVersion string, expected []string) {
	t.Helper()
	previousIndex := -1
	index := -1
	for i, item := range allMigrations {
		if item.version == previousVersion {
			previousIndex = i
		}
		if item.version == version && item.path == path {
			index = i
		}
	}
	if index < 0 || previousIndex < 0 || index <= previousIndex {
		t.Fatalf("migration %q must be registered after %q", version, previousVersion)
	}
	script, err := migrations.ReadFile(path)
	if err != nil {
		t.Fatalf("read embedded migration %q: %v", version, err)
	}
	for _, value := range expected {
		if !strings.Contains(string(script), value) {
			t.Errorf("migration %q does not contain %q", version, value)
		}
	}
}
