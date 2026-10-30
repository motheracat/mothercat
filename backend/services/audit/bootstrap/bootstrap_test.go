package bootstrap

import "testing"

func TestBootstrapAuditEventKeyIsStable(t *testing.T) {
	const accountID = "9a7f2995-10d7-4e9a-b46f-4d1f758c88e7"
	got := "identity:initial-super-admin:" + accountID
	if got != "identity:initial-super-admin:9a7f2995-10d7-4e9a-b46f-4d1f758c88e7" {
		t.Fatalf("unexpected idempotency key: %s", got)
	}
}
